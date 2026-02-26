package websocket

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

// DefaultDialer is a dialer with all fields set to the default values.
var DefaultDialer = &Dialer{}

// Dialer contains options for connecting to WebSocket server.
type Dialer struct {
	// HTTPClient specifies the HTTP client to use for WebSocket connections.
	// If nil, http.DefaultClient is used.
	//
	// Configuration is extracted from HTTPClient.Transport (*http.Transport):
	//   - Proxy: proxy function for HTTP CONNECT tunneling
	//   - TLSClientConfig: TLS configuration for wss:// connections
	//   - DialContext: custom dial function for TCP connections
	//
	// For HTTP/2 WebSocket (RFC 8441), use an http.Client with http2.Transport.
	HTTPClient *http.Client

	// NetDialContext specifies a custom dial function for creating TCP connections.
	// Takes precedence over HTTPClient.Transport.(*http.Transport).DialContext.
	// If nil, falls back to the transport's DialContext or net.Dialer.
	NetDialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	// Proxy specifies a function to return a proxy for a given request.
	// Takes precedence over HTTPClient.Transport.(*http.Transport).Proxy.
	// If nil, falls back to the transport's Proxy function.
	Proxy func(*http.Request) (*url.URL, error)

	// TLSClientConfig specifies the TLS configuration to use for wss:// connections.
	// Takes precedence over HTTPClient.Transport.(*http.Transport).TLSClientConfig.
	// If nil, falls back to the transport's TLSClientConfig.
	TLSClientConfig *tls.Config

	// HandshakeTimeout specifies the duration for the handshake to complete.
	// If zero, no timeout is applied.
	HandshakeTimeout time.Duration

	// ReadBufferSize and WriteBufferSize specify I/O buffer sizes in bytes.
	ReadBufferSize  int
	WriteBufferSize int

	// WriteBufferPool is a pool of buffers for write operations.
	WriteBufferPool BufferPool

	// Subprotocols specifies the client's requested subprotocols.
	Subprotocols []string

	// EnableCompression specifies if the client should attempt to negotiate
	// per message compression (RFC 7692).
	EnableCompression bool

	// Jar specifies the cookie jar.
	// If nil, cookies are not sent in requests and ignored in responses.
	Jar http.CookieJar
}

// Dial creates a new client connection to the WebSocket server.
func (d *Dialer) Dial(urlStr string, requestHeader http.Header) (*Conn, *http.Response, error) {
	return d.DialContext(context.Background(), urlStr, requestHeader)
}

// DialContext creates a new client connection with the provided context.
// This implements the client-side opening handshake per RFC 6455, section 4.1,
// and RFC 8441 for HTTP/2 WebSocket bootstrapping.
func (d *Dialer) DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (*Conn, *http.Response, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, nil, err
	}

	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return nil, nil, errors.New("websocket: bad scheme")
	}

	if u.Host == "" {
		return nil, nil, errors.New("websocket: empty host")
	}

	client := d.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	// Check if transport is HTTP/2.
	if d.isHTTP2(client) {
		return d.dialHTTP2(ctx, client, u, requestHeader)
	}

	// Check if proxy is configured - need special handling for WebSocket.
	proxyURL := d.getProxyURL(u)
	if proxyURL != nil {
		return d.dialWithProxy(ctx, u, proxyURL, requestHeader)
	}

	// Direct dial with raw net.Conn, preserving the connection for
	// LocalAddr, RemoteAddr, UnderlyingConn, and deadline access.
	return d.dialDirect(ctx, u, requestHeader)
}

// isHTTP2 checks if the client's transport is HTTP/2.
func (d *Dialer) isHTTP2(client *http.Client) bool {
	if client.Transport == nil {
		return false
	}
	_, ok := client.Transport.(*http2.Transport)
	return ok
}

// getProxyURL returns the proxy URL for the given target URL, or nil if no proxy.
// Resolution chain: d.Proxy -> transport.Proxy.
func (d *Dialer) getProxyURL(u *url.URL) *url.URL {
	req := &http.Request{URL: u}

	if d.Proxy != nil {
		proxyURL, err := d.Proxy(req)
		if err != nil || proxyURL == nil {
			return nil
		}
		return proxyURL
	}

	if d.HTTPClient != nil {
		transport, ok := d.HTTPClient.Transport.(*http.Transport)
		if ok && transport != nil && transport.Proxy != nil {
			proxyURL, err := transport.Proxy(req)
			if err != nil || proxyURL == nil {
				return nil
			}
			return proxyURL
		}
	}

	return nil
}

// tlsConfig returns the TLS configuration for the given server name.
// Resolution chain: d.TLSClientConfig -> transport.TLSClientConfig -> empty config.
func (d *Dialer) tlsConfig(serverName string) *tls.Config {
	if d.TLSClientConfig != nil {
		cfg := d.TLSClientConfig.Clone()
		if cfg.ServerName == "" {
			cfg.ServerName = serverName
		}
		return cfg
	}

	if d.HTTPClient != nil {
		if transport, ok := d.HTTPClient.Transport.(*http.Transport); ok && transport != nil && transport.TLSClientConfig != nil {
			cfg := transport.TLSClientConfig.Clone()
			if cfg.ServerName == "" {
				cfg.ServerName = serverName
			}
			return cfg
		}
	}

	return &tls.Config{ServerName: serverName}
}

// dial establishes a TCP or TLS connection using the resolution chain:
// TLS fast-path (transport.DialTLSContext), then TCP dial (d.NetDialContext ->
// transport.DialContext -> net.Dialer), then TLS handshake if needed.
func (d *Dialer) dial(ctx context.Context, isTLS bool, hostPort, serverName string) (net.Conn, error) {
	var transport *http.Transport
	if d.HTTPClient != nil {
		transport, _ = d.HTTPClient.Transport.(*http.Transport)
	}

	// TLS fast-path: use transport.DialTLSContext if available.
	if isTLS && transport != nil && transport.DialTLSContext != nil {
		return transport.DialTLSContext(ctx, "tcp", hostPort)
	}

	// TCP dial: d.NetDialContext -> transport.DialContext -> net.Dialer.
	var netConn net.Conn
	var err error

	switch {
	case d.NetDialContext != nil:
		netConn, err = d.NetDialContext(ctx, "tcp", hostPort)
	case transport != nil && transport.DialContext != nil:
		netConn, err = transport.DialContext(ctx, "tcp", hostPort)
	default:
		var dialer net.Dialer
		netConn, err = dialer.DialContext(ctx, "tcp", hostPort)
	}
	if err != nil {
		return nil, err
	}

	if !isTLS {
		return netConn, nil
	}

	// TLS handshake.
	tlsConf := d.tlsConfig(serverName)
	tlsConn := tls.Client(netConn, tlsConf)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		netConn.Close()
		return nil, err
	}
	return tlsConn, nil
}

// dialDirect establishes a WebSocket connection by dialing TCP directly,
// preserving the net.Conn for address and deadline access.
func (d *Dialer) dialDirect(ctx context.Context, u *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	hostPort := hostPortFromURL(u)
	isTLS := u.Scheme == "https"

	netConn, err := d.dial(ctx, isTLS, hostPort, u.Hostname())
	if err != nil {
		return nil, nil, err
	}

	if d.HandshakeTimeout > 0 {
		deadline := time.Now().Add(d.HandshakeTimeout)
		if err := netConn.SetDeadline(deadline); err != nil {
			netConn.Close()
			return nil, nil, err
		}
	}

	conn, resp, err := d.doHandshake(ctx, netConn, u, requestHeader)
	if err != nil {
		netConn.Close()
		return nil, resp, err
	}

	if d.HandshakeTimeout > 0 {
		if err := netConn.SetDeadline(time.Time{}); err != nil {
			conn.Close()
			return nil, resp, err
		}
	}

	return conn, resp, nil
}

// dialWithProxy establishes a WebSocket connection through an HTTP proxy.
func (d *Dialer) dialWithProxy(ctx context.Context, u *url.URL, proxyURL *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	var deadline time.Time
	if d.HandshakeTimeout > 0 {
		deadline = time.Now().Add(d.HandshakeTimeout)
	}

	// Connect to proxy.
	proxyConn, err := d.dialProxy(ctx, proxyURL, u)
	if err != nil {
		return nil, nil, err
	}

	if !deadline.IsZero() {
		if err := proxyConn.SetDeadline(deadline); err != nil {
			proxyConn.Close()
			return nil, nil, err
		}
	}

	conn, resp, err := d.doHandshake(ctx, proxyConn, u, requestHeader)
	if err != nil {
		proxyConn.Close()
		return nil, resp, err
	}

	if !deadline.IsZero() {
		if err := proxyConn.SetDeadline(time.Time{}); err != nil {
			conn.Close()
			return nil, resp, err
		}
	}

	return conn, resp, nil
}

// dialProxy connects to the proxy and establishes a CONNECT tunnel per RFC 7231, section 4.3.6.
// This allows WebSocket traffic to be tunneled through HTTP proxies.
func (d *Dialer) dialProxy(ctx context.Context, proxyURL *url.URL, targetURL *url.URL) (net.Conn, error) {
	proxyHost := proxyURL.Host
	if proxyURL.Port() == "" {
		proxyHost = net.JoinHostPort(proxyURL.Hostname(), "80")
	}

	targetHostPort := hostPortFromURL(targetURL)

	// Dial to proxy server using the resolution chain:
	// d.NetDialContext -> transport.DialContext -> net.Dialer.
	var proxyConn net.Conn
	var err error

	var transport *http.Transport
	if d.HTTPClient != nil {
		transport, _ = d.HTTPClient.Transport.(*http.Transport)
	}

	switch {
	case d.NetDialContext != nil:
		proxyConn, err = d.NetDialContext(ctx, "tcp", proxyHost)
	case transport != nil && transport.DialContext != nil:
		proxyConn, err = transport.DialContext(ctx, "tcp", proxyHost)
	default:
		var dialer net.Dialer
		proxyConn, err = dialer.DialContext(ctx, "tcp", proxyHost)
	}
	if err != nil {
		return nil, err
	}

	// Send HTTP CONNECT request.
	connectReq := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: targetHostPort},
		Host:   targetHostPort,
		Header: make(http.Header),
	}

	// Add proxy authorization if present.
	if proxyURL.User != nil {
		username := proxyURL.User.Username()
		password, _ := proxyURL.User.Password()
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		connectReq.Header.Set("Proxy-Authorization", "Basic "+auth)
	}

	if err := connectReq.Write(proxyConn); err != nil {
		proxyConn.Close()
		return nil, err
	}

	// Read proxy response.
	br := bufio.NewReader(proxyConn)
	resp, err := http.ReadResponse(br, connectReq)
	if err != nil {
		proxyConn.Close()
		return nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		proxyConn.Close()
		return nil, errors.New("websocket: proxy CONNECT failed: " + resp.Status)
	}

	// For wss://, upgrade to TLS.
	if targetURL.Scheme == "https" {
		tlsConf := d.tlsConfig(targetURL.Hostname())

		tlsConn := tls.Client(proxyConn, tlsConf)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			proxyConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	return proxyConn, nil
}

// doHandshake performs the client-side opening handshake per RFC 6455, section 4.1.
// Used for connections established via raw net.Conn (direct dial and proxy paths).
func (d *Dialer) doHandshake(_ context.Context, netConn net.Conn, u *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	challengeKey, err := generateChallengeKey()
	if err != nil {
		return nil, nil, err
	}

	req := &http.Request{
		Method:     http.MethodGet,
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Host:       u.Host,
	}

	d.buildHandshakeHeaders(req, requestHeader, challengeKey)

	if d.Jar != nil {
		for _, cookie := range d.Jar.Cookies(u) {
			req.AddCookie(cookie)
		}
	}

	if err := req.Write(netConn); err != nil {
		return nil, nil, err
	}

	br := bufio.NewReader(netConn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, nil, err
	}

	if d.Jar != nil {
		if rc := resp.Cookies(); len(rc) > 0 {
			d.Jar.SetCookies(u, rc)
		}
	}

	subprotocol, compress, err := d.validateHTTP1Response(resp, challengeKey)
	if err != nil {
		defer resp.Body.Close()
		return nil, resp, err
	}

	// Prevent callers from closing the underlying connection via resp.Body.
	// The WebSocket Conn now owns the stream; resp is only useful for reading headers.
	resp.Body = http.NoBody

	conn := newConnWithPool(netConn, false, d.ReadBufferSize, d.WriteBufferSize, d.WriteBufferPool)
	conn.subprotocol = subprotocol
	conn.compressionEnabled = compress

	// Reuse the handshake bufio.Reader if it buffered data beyond the HTTP response.
	// Without this, early WebSocket frames from the server would be silently lost.
	if br.Buffered() > 0 {
		conn.br = br
	}

	return conn, resp, nil
}

// buildHandshakeHeaders sets the required WebSocket handshake headers on the request
// per RFC 6455, section 4.1.
func (d *Dialer) buildHandshakeHeaders(req *http.Request, requestHeader http.Header, challengeKey string) {
	for k, vs := range requestHeader {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", challengeKey)
	req.Header.Set("Sec-WebSocket-Version", websocketVersion)

	if len(d.Subprotocols) > 0 {
		req.Header.Set("Sec-WebSocket-Protocol", strings.Join(d.Subprotocols, ", "))
	}

	if d.EnableCompression {
		req.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate; client_max_window_bits")
	}
}

// validateHTTP1Response validates the server handshake response per RFC 6455, section 4.2.2.
// Returns the negotiated subprotocol and compression status.
func (d *Dialer) validateHTTP1Response(resp *http.Response, challengeKey string) (subprotocol string, compress bool, err error) {
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return "", false, ErrBadHandshake
	}

	if !headerContainsToken(resp.Header, "Upgrade", "websocket") {
		return "", false, ErrBadHandshake
	}

	if !headerContainsToken(resp.Header, "Connection", "upgrade") {
		return "", false, ErrBadHandshake
	}

	expectedAccept := computeAcceptKey(challengeKey)
	if resp.Header.Get("Sec-WebSocket-Accept") != expectedAccept {
		return "", false, ErrBadHandshake
	}

	// Validate subprotocol per RFC 6455, section 4.1: if the server sends a
	// Sec-WebSocket-Protocol value that was not present in the client's handshake,
	// the client must fail the connection. This includes any value when the client
	// did not request subprotocols at all.
	subprotocol = resp.Header.Get("Sec-WebSocket-Protocol")
	if subprotocol != "" {
		if len(d.Subprotocols) == 0 || !slices.Contains(d.Subprotocols, subprotocol) {
			return "", false, ErrBadHandshake
		}
	}

	// Validate extensions per RFC 6455, section 4.1: if the server includes
	// an extension that was not present in the client's handshake, the client
	// must fail the connection.
	for _, ext := range parseExtensions(resp.Header) {
		if d.EnableCompression && ext.name == "permessage-deflate" {
			compress = true
			continue
		}
		return "", false, ErrBadHandshake
	}

	return subprotocol, compress, nil
}

// dialHTTP2 establishes a WebSocket connection over HTTP/2 per RFC 8441.
// RFC 8441 defines bootstrapping WebSockets with HTTP/2 using extended CONNECT.
func (d *Dialer) dialHTTP2(ctx context.Context, client *http.Client, u *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	// Build the request with extended CONNECT method per RFC 8441, section 4.
	// The :protocol pseudo-header is set to "websocket".
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    u,
		Host:   u.Host,
		Proto:  "websocket", // :protocol pseudo-header value
		Header: make(http.Header),
	}
	req = req.WithContext(ctx)

	// Copy request headers.
	for k, vs := range requestHeader {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	if len(d.Subprotocols) > 0 {
		req.Header.Set("Sec-WebSocket-Protocol", strings.Join(d.Subprotocols, ", "))
	}

	// Request permessage-deflate extension per RFC 7692.
	if d.EnableCompression {
		req.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate; client_max_window_bits")
	}

	if d.Jar != nil {
		for _, cookie := range d.Jar.Cookies(u) {
			req.AddCookie(cookie)
		}
	}

	// Send the request.
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	if d.Jar != nil {
		if rc := resp.Cookies(); len(rc) > 0 {
			d.Jar.SetCookies(u, rc)
		}
	}

	// RFC 8441, section 4: Response should be 200 OK for successful upgrade.
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, resp, ErrBadHandshake
	}

	// Validate subprotocol per RFC 6455, section 4.1: if the server sends a
	// Sec-WebSocket-Protocol value that was not present in the client's handshake,
	// the client must fail the connection. This includes any value when the client
	// did not request subprotocols at all.
	subprotocol := resp.Header.Get("Sec-WebSocket-Protocol")
	if subprotocol != "" {
		if len(d.Subprotocols) == 0 || !slices.Contains(d.Subprotocols, subprotocol) {
			resp.Body.Close()
			return nil, resp, ErrBadHandshake
		}
	}

	// Validate extensions per RFC 6455, section 4.1: if the server includes
	// an extension that was not present in the client's handshake, the client
	// must fail the connection.
	var compress bool
	for _, ext := range parseExtensions(resp.Header) {
		if d.EnableCompression && ext.name == "permessage-deflate" {
			compress = true
			continue
		}
		resp.Body.Close()
		return nil, resp, ErrBadHandshake
	}

	// Create a connection wrapper around the response body.
	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		resp.Body.Close()
		return nil, resp, errors.New("websocket: response body is not ReadWriteCloser")
	}

	conn := newConnFromRWC(rwc, nil, false, d.ReadBufferSize, d.WriteBufferSize, d.WriteBufferPool)
	conn.subprotocol = subprotocol
	conn.compressionEnabled = compress

	return conn, resp, nil
}

// hostPortFromURL returns host:port from URL, adding default port if needed.
func hostPortFromURL(u *url.URL) string {
	if u.Port() != "" {
		return u.Host
	}
	switch u.Scheme {
	case "https":
		return net.JoinHostPort(u.Hostname(), "443")
	default:
		return net.JoinHostPort(u.Hostname(), "80")
	}
}

// generateChallengeKey generates a 16-byte random key encoded in base64
// per RFC 6455, section 4.1.
func generateChallengeKey() (string, error) {
	key := make([]byte, 16)
	if _, err := io.ReadFull(randReader, key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
