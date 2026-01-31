package websocket

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
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
	proxyURL := d.getProxyURL(client, u)
	if proxyURL != nil {
		return d.dialWithProxy(ctx, client, u, proxyURL, requestHeader)
	}

	// Check if custom dial is configured.
	if d.hasCustomDial(client) {
		return d.dialWithTransport(ctx, client, u, requestHeader)
	}

	// Standard HTTP/1.1 upgrade via http.Client.
	return d.dialHTTP1(ctx, client, u, requestHeader)
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
func (d *Dialer) getProxyURL(client *http.Client, u *url.URL) *url.URL {
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil || transport.Proxy == nil {
		return nil
	}

	req := &http.Request{URL: u}
	proxyURL, err := transport.Proxy(req)
	if err != nil || proxyURL == nil {
		return nil
	}
	return proxyURL
}

// hasCustomDial checks if the transport has custom dial functions.
func (d *Dialer) hasCustomDial(client *http.Client) bool {
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil {
		return false
	}
	return transport.DialContext != nil || transport.DialTLSContext != nil
}

// dialHTTP1 establishes a WebSocket connection over HTTP/1.1 per RFC 6455.
// Uses http.Client for simple cases without proxy or custom dial.
func (d *Dialer) dialHTTP1(ctx context.Context, client *http.Client, u *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	// Generate 16-byte random challenge key per RFC 6455, section 4.1.
	challengeKey, err := generateChallengeKey()
	if err != nil {
		return nil, nil, err
	}

	// Build handshake request per RFC 6455, section 4.1.
	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Header: make(http.Header),
		Host:   u.Host,
	}
	req = req.WithContext(ctx)

	// Copy request headers.
	for k, vs := range requestHeader {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	// Set required headers per RFC 6455, section 4.1.
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", challengeKey)
	req.Header.Set("Sec-WebSocket-Version", websocketVersion)

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

	// Validate and create connection.
	return d.finishHTTP1Handshake(resp, challengeKey)
}

// dialWithTransport establishes a WebSocket connection using transport's dial functions.
func (d *Dialer) dialWithTransport(ctx context.Context, client *http.Client, u *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	transport := client.Transport.(*http.Transport)

	hostPort := hostPortFromURL(u)

	var deadline time.Time
	if d.HandshakeTimeout > 0 {
		deadline = time.Now().Add(d.HandshakeTimeout)
	}

	// Dial using transport's dial function.
	netConn, err := d.dialNet(ctx, transport, u.Scheme == "https", hostPort, u.Hostname())
	if err != nil {
		return nil, nil, err
	}

	if !deadline.IsZero() {
		if err := netConn.SetDeadline(deadline); err != nil {
			netConn.Close()
			return nil, nil, err
		}
	}

	conn, resp, err := d.doHandshake(ctx, netConn, u, requestHeader, transport.TLSClientConfig)
	if err != nil {
		netConn.Close()
		return nil, resp, err
	}

	if !deadline.IsZero() {
		if err := netConn.SetDeadline(time.Time{}); err != nil {
			conn.Close()
			return nil, resp, err
		}
	}

	return conn, resp, nil
}

// dialWithProxy establishes a WebSocket connection through an HTTP proxy.
func (d *Dialer) dialWithProxy(ctx context.Context, client *http.Client, u *url.URL, proxyURL *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	transport, _ := client.Transport.(*http.Transport)

	var deadline time.Time
	if d.HandshakeTimeout > 0 {
		deadline = time.Now().Add(d.HandshakeTimeout)
	}

	// Connect to proxy.
	proxyConn, err := d.dialProxy(ctx, transport, proxyURL, u)
	if err != nil {
		return nil, nil, err
	}

	if !deadline.IsZero() {
		if err := proxyConn.SetDeadline(deadline); err != nil {
			proxyConn.Close()
			return nil, nil, err
		}
	}

	var tlsConfig *tls.Config
	if transport != nil {
		tlsConfig = transport.TLSClientConfig
	}

	conn, resp, err := d.doHandshake(ctx, proxyConn, u, requestHeader, tlsConfig)
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
func (d *Dialer) dialProxy(ctx context.Context, transport *http.Transport, proxyURL *url.URL, targetURL *url.URL) (net.Conn, error) {
	proxyHost := proxyURL.Host
	if proxyURL.Port() == "" {
		proxyHost = net.JoinHostPort(proxyURL.Hostname(), "80")
	}

	targetHostPort := hostPortFromURL(targetURL)

	// Dial to proxy server.
	var proxyConn net.Conn
	var err error

	if transport != nil && transport.DialContext != nil {
		proxyConn, err = transport.DialContext(ctx, "tcp", proxyHost)
	} else {
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
		tlsConfig := &tls.Config{}
		if transport != nil && transport.TLSClientConfig != nil {
			tlsConfig = transport.TLSClientConfig.Clone()
		}
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = targetURL.Hostname()
		}

		tlsConn := tls.Client(proxyConn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			proxyConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	return proxyConn, nil
}

// dialNet dials using the transport's dial functions.
func (d *Dialer) dialNet(ctx context.Context, transport *http.Transport, isTLS bool, hostPort, serverName string) (net.Conn, error) {
	if isTLS {
		if transport.DialTLSContext != nil {
			return transport.DialTLSContext(ctx, "tcp", hostPort)
		}

		// Use DialContext + manual TLS.
		var netConn net.Conn
		var err error
		if transport.DialContext != nil {
			netConn, err = transport.DialContext(ctx, "tcp", hostPort)
		} else {
			var dialer net.Dialer
			netConn, err = dialer.DialContext(ctx, "tcp", hostPort)
		}
		if err != nil {
			return nil, err
		}

		tlsConfig := &tls.Config{}
		if transport.TLSClientConfig != nil {
			tlsConfig = transport.TLSClientConfig.Clone()
		}
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = serverName
		}

		tlsConn := tls.Client(netConn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			netConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}

	if transport.DialContext != nil {
		return transport.DialContext(ctx, "tcp", hostPort)
	}

	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", hostPort)
}

// doHandshake performs the client-side opening handshake per RFC 6455, section 4.1.
func (d *Dialer) doHandshake(_ context.Context, netConn net.Conn, u *url.URL, requestHeader http.Header, _ *tls.Config) (*Conn, *http.Response, error) {
	// Generate 16-byte random challenge key per RFC 6455, section 4.1.
	challengeKey, err := generateChallengeKey()
	if err != nil {
		return nil, nil, err
	}

	// Build handshake request per RFC 6455, section 4.1.
	req := &http.Request{
		Method:     http.MethodGet,
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Host:       u.Host,
	}

	for k, vs := range requestHeader {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	// Set required headers per RFC 6455, section 4.1.
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", challengeKey)
	req.Header.Set("Sec-WebSocket-Version", websocketVersion)

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

	// Validate server response per RFC 6455, section 4.1.
	if resp.StatusCode != http.StatusSwitchingProtocols {
		defer resp.Body.Close()
		return nil, resp, ErrBadHandshake
	}

	// Validate Upgrade header per RFC 6455, section 4.2.2.
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		return nil, resp, ErrBadHandshake
	}

	// Validate Connection header per RFC 6455, section 4.2.2.
	if !strings.EqualFold(resp.Header.Get("Connection"), "upgrade") {
		return nil, resp, ErrBadHandshake
	}

	// Validate Sec-WebSocket-Accept per RFC 6455, section 4.2.2, item 5.4.
	expectedAccept := computeAcceptKey(challengeKey)
	if resp.Header.Get("Sec-WebSocket-Accept") != expectedAccept {
		return nil, resp, ErrBadHandshake
	}

	// Validate subprotocol per RFC 6455, section 4.2.2.
	// Server must return a subprotocol that was requested by the client.
	subprotocol := resp.Header.Get("Sec-WebSocket-Protocol")
	if subprotocol != "" && len(d.Subprotocols) > 0 {
		found := false
		for _, p := range d.Subprotocols {
			if p == subprotocol {
				found = true
				break
			}
		}
		if !found {
			return nil, resp, ErrBadHandshake
		}
	}

	// Check for permessage-deflate extension per RFC 7692.
	var compress bool
	for _, ext := range parseExtensions(resp.Header) {
		if ext.name == "permessage-deflate" {
			compress = true
			break
		}
	}

	conn := newConnWithPool(netConn, false, d.ReadBufferSize, d.WriteBufferSize, d.WriteBufferPool)
	conn.subprotocol = subprotocol
	conn.compressionEnabled = compress

	return conn, resp, nil
}

// finishHTTP1Handshake validates the server response per RFC 6455, section 4.2.2
// and creates the WebSocket connection.
func (d *Dialer) finishHTTP1Handshake(resp *http.Response, challengeKey string) (*Conn, *http.Response, error) {
	// Validate status code per RFC 6455, section 4.2.2.
	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		return nil, resp, ErrBadHandshake
	}

	// Validate Upgrade header per RFC 6455, section 4.2.2.
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		resp.Body.Close()
		return nil, resp, ErrBadHandshake
	}

	// Validate Connection header per RFC 6455, section 4.2.2.
	if !strings.EqualFold(resp.Header.Get("Connection"), "upgrade") {
		resp.Body.Close()
		return nil, resp, ErrBadHandshake
	}

	// Validate Sec-WebSocket-Accept per RFC 6455, section 4.2.2, item 5.4.
	expectedAccept := computeAcceptKey(challengeKey)
	if resp.Header.Get("Sec-WebSocket-Accept") != expectedAccept {
		resp.Body.Close()
		return nil, resp, ErrBadHandshake
	}

	// Validate subprotocol per RFC 6455, section 4.2.2.
	// Server must return a subprotocol that was requested by the client.
	subprotocol := resp.Header.Get("Sec-WebSocket-Protocol")
	if subprotocol != "" && len(d.Subprotocols) > 0 {
		found := false
		for _, p := range d.Subprotocols {
			if p == subprotocol {
				found = true
				break
			}
		}
		if !found {
			resp.Body.Close()
			return nil, resp, ErrBadHandshake
		}
	}

	// Check for permessage-deflate extension per RFC 7692.
	var compress bool
	for _, ext := range parseExtensions(resp.Header) {
		if ext.name == "permessage-deflate" {
			compress = true
			break
		}
	}

	// For 101 responses, resp.Body is an io.ReadWriteCloser.
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

	// Validate subprotocol per RFC 6455, section 4.2.2.
	// Server must return a subprotocol that was requested by the client.
	subprotocol := resp.Header.Get("Sec-WebSocket-Protocol")
	if subprotocol != "" && len(d.Subprotocols) > 0 {
		found := false
		for _, p := range d.Subprotocols {
			if p == subprotocol {
				found = true
				break
			}
		}
		if !found {
			resp.Body.Close()
			return nil, resp, ErrBadHandshake
		}
	}

	// Check for permessage-deflate extension per RFC 7692.
	var compress bool
	for _, ext := range parseExtensions(resp.Header) {
		if ext.name == "permessage-deflate" {
			compress = true
			break
		}
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
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
