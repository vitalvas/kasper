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
)

// DefaultDialer is a dialer with all fields set to the default values.
var DefaultDialer = &Dialer{}

// Dialer contains options for connecting to WebSocket server.
type Dialer struct {
	// NetDial specifies the dial function for creating TCP connections.
	NetDial func(network, addr string) (net.Conn, error)

	// NetDialContext specifies the dial function for creating TCP connections with context.
	NetDialContext func(ctx context.Context, network, addr string) (net.Conn, error)

	// NetDialTLSContext specifies the dial function for creating TLS connections with context.
	NetDialTLSContext func(ctx context.Context, network, addr string) (net.Conn, error)

	// Proxy specifies a function to return a proxy for a given Request.
	Proxy func(*http.Request) (*url.URL, error)

	// TLSClientConfig specifies the TLS configuration to use with tls.Client.
	TLSClientConfig *tls.Config

	// HandshakeTimeout specifies the duration for the handshake to complete.
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
	Jar http.CookieJar
}

// Dial creates a new client connection to the WebSocket server.
func (d *Dialer) Dial(urlStr string, requestHeader http.Header) (*Conn, *http.Response, error) {
	return d.DialContext(context.Background(), urlStr, requestHeader)
}

// DialContext creates a new client connection with the provided context.
// This implements the client-side opening handshake per RFC 6455, section 4.1.
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

	hostPort := u.Host
	if u.Port() == "" {
		switch u.Scheme {
		case "http":
			hostPort = net.JoinHostPort(u.Host, "80")
		case "https":
			hostPort = net.JoinHostPort(u.Host, "443")
		}
	}

	var deadline time.Time
	if d.HandshakeTimeout > 0 {
		deadline = time.Now().Add(d.HandshakeTimeout)
	}

	netConn, err := d.dial(ctx, u, hostPort)
	if err != nil {
		return nil, nil, err
	}

	if !deadline.IsZero() {
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

	if !deadline.IsZero() {
		if err := netConn.SetDeadline(time.Time{}); err != nil {
			conn.Close()
			return nil, resp, err
		}
	}

	return conn, resp, nil
}

func (d *Dialer) dial(ctx context.Context, u *url.URL, hostPort string) (net.Conn, error) {
	// Check for proxy configuration.
	var proxyURL *url.URL
	if d.Proxy != nil {
		req := &http.Request{URL: u}
		var err error
		proxyURL, err = d.Proxy(req)
		if err != nil {
			return nil, err
		}
	}

	// If proxy is configured, connect through proxy.
	if proxyURL != nil {
		return d.dialProxy(ctx, proxyURL, u, hostPort)
	}

	if u.Scheme == "https" {
		if d.NetDialTLSContext != nil {
			return d.NetDialTLSContext(ctx, "tcp", hostPort)
		}

		tlsConn, err := d.dialTLS(ctx, hostPort, u.Hostname())
		if err != nil {
			return nil, err
		}
		return tlsConn, nil
	}

	if d.NetDialContext != nil {
		return d.NetDialContext(ctx, "tcp", hostPort)
	}

	if d.NetDial != nil {
		return d.NetDial("tcp", hostPort)
	}

	var dialer net.Dialer
	return dialer.DialContext(ctx, "tcp", hostPort)
}

func (d *Dialer) dialProxy(ctx context.Context, proxyURL *url.URL, targetURL *url.URL, hostPort string) (net.Conn, error) {
	proxyHost := proxyURL.Host
	if proxyURL.Port() == "" {
		proxyHost = net.JoinHostPort(proxyURL.Hostname(), "80")
	}

	// Connect to proxy server.
	var dialer net.Dialer
	proxyConn, err := dialer.DialContext(ctx, "tcp", proxyHost)
	if err != nil {
		return nil, err
	}

	// Send HTTP CONNECT request.
	connectReq := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: hostPort},
		Host:   hostPort,
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
		tlsConfig := d.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		} else {
			tlsConfig = tlsConfig.Clone()
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

func (d *Dialer) dialTLS(ctx context.Context, hostPort, serverName string) (net.Conn, error) {
	tlsConfig := d.TLSClientConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	} else {
		tlsConfig = tlsConfig.Clone()
	}

	if tlsConfig.ServerName == "" {
		tlsConfig.ServerName = serverName
	}

	var dialer net.Dialer
	netConn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(netConn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		netConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// doHandshake performs the client-side opening handshake per RFC 6455, section 4.1.
func (d *Dialer) doHandshake(_ context.Context, netConn net.Conn, u *url.URL, requestHeader http.Header) (*Conn, *http.Response, error) {
	// Generate 16-byte random challenge key per RFC 6455, section 4.1.
	challengeKey := generateChallengeKey()

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

	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		return nil, resp, ErrBadHandshake
	}

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

// generateChallengeKey generates a 16-byte random key encoded in base64
// per RFC 6455, section 4.1.
func generateChallengeKey() string {
	key := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}
