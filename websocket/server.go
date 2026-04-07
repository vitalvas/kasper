package websocket

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// WebSocket protocol constants per RFC 6455.
const (
	// websocketGUID is the globally unique identifier for WebSocket handshake
	// per RFC 6455, section 4.2.2, item 5.4.
	websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	// websocketVersion is the WebSocket protocol version per RFC 6455, section 4.2.1, item 6.
	websocketVersion = "13"
)

// Upgrader specifies parameters for upgrading an HTTP connection to a WebSocket connection.
type Upgrader struct {
	// HandshakeTimeout specifies the duration for the handshake to complete.
	HandshakeTimeout time.Duration

	// ReadBufferSize and WriteBufferSize specify I/O buffer sizes in bytes.
	ReadBufferSize  int
	WriteBufferSize int

	// WriteBufferPool is a pool of buffers for write operations.
	WriteBufferPool BufferPool

	// Subprotocols specifies the server's supported protocols in order of preference.
	Subprotocols []string

	// Error specifies the function for generating HTTP error responses.
	Error func(w http.ResponseWriter, r *http.Request, status int, reason error)

	// CheckOrigin returns true if the request Origin header is acceptable.
	CheckOrigin func(r *http.Request) bool

	// EnableCompression specifies if the server should attempt to negotiate
	// per message compression (RFC 7692).
	EnableCompression bool

	// MessageTypePolicy is applied to every accepted connection.
	// The zero value MessageTypePolicyAny imposes no restriction.
	MessageTypePolicy MessageTypePolicy

	// MaxFrameSize sets the maximum payload size in bytes for a single
	// WebSocket frame on every accepted connection. Zero disables the limit.
	MaxFrameSize int64

	// DisablePongReply disables the automatic pong response to client ping
	// frames. By default the server echoes a pong with the ping payload per
	// RFC 6455, section 5.5.3.
	DisablePongReply bool

	// RequireEmptyPingPayload closes the connection with CloseProtocolError
	// when a ping frame carrying a non-empty payload is received.
	RequireEmptyPingPayload bool

	// EmptyPongPayload sends an empty pong payload regardless of the ping
	// payload. By default the ping payload is echoed per RFC 6455.
	// Ignored when PingHandler is set.
	EmptyPongPayload bool

	// PingHandler is called for each received ping frame with the raw payload
	// bytes. The returned bytes are used as the pong payload; returning nil
	// sends an empty pong. If PingHandler is set, DisablePongReply and
	// EmptyPongPayload are ignored. RequireEmptyPingPayload is still enforced
	// before PingHandler is called.
	PingHandler func(payload []byte) ([]byte, error)
}

// applyConnPolicy applies all per-connection policies from the Upgrader to conn.
func (u *Upgrader) applyConnPolicy(conn *Conn) {
	conn.SetMessageTypePolicy(u.MessageTypePolicy)
	if u.MaxFrameSize > 0 {
		conn.SetMaxFrameSize(u.MaxFrameSize)
	}
	u.applyPingPolicy(conn)
}

// applyPingPolicy configures the ping handler on conn according to the
// Upgrader's ping/pong fields. Priority (highest first):
//  1. PingHandler — full control; RequireEmptyPingPayload is still pre-checked
//  2. DisablePongReply — no pong sent
//  3. RequireEmptyPingPayload / EmptyPongPayload — convenience options
//  4. No fields set — default RFC 6455 echo behaviour
func (u *Upgrader) applyPingPolicy(conn *Conn) {
	requireEmpty := u.RequireEmptyPingPayload

	if u.PingHandler != nil {
		h := u.PingHandler
		conn.SetPingHandler(func(appData string) error {
			if requireEmpty && len(appData) != 0 {
				_ = conn.CloseWithMessage(CloseProtocolError, "non-empty ping payload not allowed")
				return ErrNonEmptyPingPayload
			}
			pong, err := h([]byte(appData))
			if err != nil {
				return err
			}
			return conn.WriteControl(PongMessage, pong, time.Now().Add(5*time.Second))
		})
		return
	}

	emptyPong := u.EmptyPongPayload
	disable := u.DisablePongReply

	if !requireEmpty && !emptyPong && !disable {
		return // default echo behaviour
	}

	conn.SetPingHandler(func(appData string) error {
		if requireEmpty && len(appData) != 0 {
			_ = conn.CloseWithMessage(CloseProtocolError, "non-empty ping payload not allowed")
			return ErrNonEmptyPingPayload
		}
		if disable {
			return nil
		}
		var payload []byte
		if !emptyPong {
			payload = []byte(appData)
		}
		return conn.WriteControl(PongMessage, payload, time.Now().Add(5*time.Second))
	})
}

func (u *Upgrader) returnError(w http.ResponseWriter, r *http.Request, status int, reason error) {
	if u.Error != nil {
		u.Error(w, r, status, reason)
		return
	}
	http.Error(w, reason.Error(), status)
}

func (u *Upgrader) selectSubprotocol(r *http.Request) string {
	clientProtocols := Subprotocols(r)
	for _, serverProtocol := range u.Subprotocols {
		if slices.Contains(clientProtocols, serverProtocol) {
			return serverProtocol
		}
	}
	return ""
}

// Upgrade upgrades the HTTP server connection to the WebSocket protocol.
// This implements the server-side opening handshake per RFC 6455, section 4.2.2,
// and RFC 8441 for HTTP/2 WebSocket bootstrapping.
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*Conn, error) {
	// Check for HTTP/2 WebSocket upgrade (RFC 8441).
	if r.ProtoMajor == 2 && r.Method == http.MethodConnect {
		return u.upgradeHTTP2(w, r, responseHeader)
	}

	if !IsWebSocketUpgrade(r) {
		u.returnError(w, r, http.StatusBadRequest, ErrBadHandshake)
		return nil, ErrBadHandshake
	}

	if r.Method != http.MethodGet {
		u.returnError(w, r, http.StatusMethodNotAllowed, ErrBadHandshake)
		return nil, ErrBadHandshake
	}

	// Check WebSocket version per RFC 6455, section 4.2.1, item 6.
	if !strings.EqualFold(r.Header.Get("Sec-WebSocket-Version"), websocketVersion) {
		u.returnError(w, r, http.StatusBadRequest, errors.New("websocket: unsupported version"))
		return nil, ErrBadHandshake
	}

	checkOrigin := u.CheckOrigin
	if checkOrigin == nil {
		checkOrigin = checkSameOrigin
	}
	if !checkOrigin(r) {
		u.returnError(w, r, http.StatusForbidden, errors.New("websocket: origin not allowed"))
		return nil, ErrBadHandshake
	}

	// Extract challenge key per RFC 6455, section 4.2.1, item 5.
	challengeKey := r.Header.Get("Sec-WebSocket-Key")
	if challengeKey == "" {
		u.returnError(w, r, http.StatusBadRequest, errors.New("websocket: missing Sec-WebSocket-Key"))
		return nil, ErrBadHandshake
	}

	subprotocol := u.selectSubprotocol(r)

	// Negotiate permessage-deflate extension per RFC 7692.
	var compress bool
	var compressionParams string
	if u.EnableCompression {
		for _, ext := range parseExtensions(r.Header) {
			if ext.name == "permessage-deflate" {
				compress = true
				compressionParams = negotiateCompressionParams(ext.params)
				break
			}
		}
	}

	h, ok := w.(http.Hijacker)
	if !ok {
		u.returnError(w, r, http.StatusInternalServerError, errors.New("websocket: response does not implement http.Hijacker"))
		return nil, ErrBadHandshake
	}

	netConn, brw, err := h.Hijack()
	if err != nil {
		u.returnError(w, r, http.StatusInternalServerError, err)
		return nil, err
	}

	if u.HandshakeTimeout > 0 {
		_ = netConn.SetWriteDeadline(time.Now().Add(u.HandshakeTimeout))
	}

	// Send server handshake response per RFC 6455, section 4.2.2.
	buf := brw.Writer
	buf.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	buf.WriteString("Upgrade: websocket\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Sec-WebSocket-Accept: ")
	buf.WriteString(computeAcceptKey(challengeKey))
	buf.WriteString("\r\n")

	if subprotocol != "" {
		buf.WriteString("Sec-WebSocket-Protocol: ")
		buf.WriteString(subprotocol)
		buf.WriteString("\r\n")
	}

	// Respond with permessage-deflate extension per RFC 7692, section 7.1.
	if compress {
		buf.WriteString("Sec-WebSocket-Extensions: permessage-deflate")
		buf.WriteString(compressionParams)
		buf.WriteString("\r\n")
	}

	for k, vs := range responseHeader {
		for _, v := range vs {
			buf.WriteString(k)
			buf.WriteString(": ")
			buf.WriteString(v)
			buf.WriteString("\r\n")
		}
	}

	buf.WriteString("\r\n")

	if err := buf.Flush(); err != nil {
		netConn.Close()
		return nil, err
	}

	if u.HandshakeTimeout > 0 {
		_ = netConn.SetWriteDeadline(time.Time{})
	}

	conn := newConnFromBufio(netConn, brw, connConfig{
		isServer:        true,
		readBufferSize:  u.ReadBufferSize,
		writeBufferSize: u.WriteBufferSize,
		writeBufferPool: u.WriteBufferPool,
	})
	conn.subprotocol = subprotocol
	conn.compressionEnabled = compress
	u.applyConnPolicy(conn)

	return conn, nil
}

// http2ConnAdapter adapts an HTTP/2 stream into an io.ReadWriteCloser
// for use with WebSocket connections. It reads from the request body
// and writes to the response writer with automatic flushing.
type http2ConnAdapter struct {
	body io.ReadCloser
	w    http.ResponseWriter
	rc   *http.ResponseController
}

func (a *http2ConnAdapter) Read(p []byte) (int, error) {
	return a.body.Read(p)
}

func (a *http2ConnAdapter) Write(p []byte) (int, error) {
	n, err := a.w.Write(p)
	if err != nil {
		return n, err
	}

	if fErr := a.rc.Flush(); fErr != nil {
		return n, fErr
	}

	return n, nil
}

func (a *http2ConnAdapter) Close() error {
	return a.body.Close()
}

// upgradeHTTP2 handles WebSocket upgrade over HTTP/2 per RFC 8441.
// Uses an adapter wrapping r.Body and w instead of Hijack, since
// HTTP/2 connections do not support hijacking.
func (u *Upgrader) upgradeHTTP2(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*Conn, error) {
	if r.Proto != "websocket" {
		u.returnError(w, r, http.StatusBadRequest, errors.New("websocket: invalid :protocol for HTTP/2"))
		return nil, ErrBadHandshake
	}

	checkOrigin := u.CheckOrigin
	if checkOrigin == nil {
		checkOrigin = checkSameOrigin
	}
	if !checkOrigin(r) {
		u.returnError(w, r, http.StatusForbidden, errors.New("websocket: origin not allowed"))
		return nil, ErrBadHandshake
	}

	subprotocol := u.selectSubprotocol(r)

	// Negotiate permessage-deflate extension per RFC 7692.
	var compress bool
	var compressionParams string
	if u.EnableCompression {
		for _, ext := range parseExtensions(r.Header) {
			if ext.name == "permessage-deflate" {
				compress = true
				compressionParams = negotiateCompressionParams(ext.params)
				break
			}
		}
	}

	for k, vs := range responseHeader {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	if subprotocol != "" {
		w.Header().Set("Sec-WebSocket-Protocol", subprotocol)
	}

	if compress {
		w.Header().Set("Sec-WebSocket-Extensions", fmt.Sprintf("permessage-deflate%s", compressionParams))
	}

	// RFC 8441: Response is 200 OK, not 101 Switching Protocols.
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	if err := rc.Flush(); err != nil {
		return nil, err
	}

	rwc := &http2ConnAdapter{
		body: r.Body,
		w:    w,
		rc:   rc,
	}

	conn := newConnFromRWC(connConfig{
		rwc:             rwc,
		isServer:        true,
		readBufferSize:  u.ReadBufferSize,
		writeBufferSize: u.WriteBufferSize,
		writeBufferPool: u.WriteBufferPool,
	})
	conn.subprotocol = subprotocol
	conn.compressionEnabled = compress
	u.applyConnPolicy(conn)

	return conn, nil
}

func newConnFromBufio(netConn net.Conn, brw *bufio.ReadWriter, cfg connConfig) *Conn {
	cfg.rwc = netConn
	cfg.netConn = netConn
	c := newConnFromRWC(cfg)
	// Use the buffered reader if there's buffered data from the HTTP handshake.
	// This ensures any data read-ahead by the HTTP server is not lost.
	if brw != nil && brw.Reader.Buffered() > 0 {
		c.br = brw.Reader
	}
	return c
}

// computeAcceptKey computes the Sec-WebSocket-Accept value per RFC 6455, section 4.2.2, item 5.4.
// The accept key is the base64-encoded SHA-1 hash of the challenge key concatenated with the GUID.
func computeAcceptKey(challengeKey string) string {
	h := sha1.New()
	h.Write([]byte(challengeKey))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func checkSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return equalASCIIFold(origin, fmt.Sprintf("http://%s", r.Host)) || equalASCIIFold(origin, fmt.Sprintf("https://%s", r.Host))
}

func equalASCIIFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		sr := s[i]
		tr := t[i]
		if sr >= 'A' && sr <= 'Z' {
			sr = sr + 'a' - 'A'
		}
		if tr >= 'A' && tr <= 'Z' {
			tr = tr + 'a' - 'A'
		}
		if sr != tr {
			return false
		}
	}
	return true
}

// Subprotocols returns the subprotocols requested by the client in the
// Sec-WebSocket-Protocol header per RFC 6455, section 11.3.4.
func Subprotocols(r *http.Request) []string {
	h := r.Header.Values("Sec-WebSocket-Protocol")
	if len(h) == 0 {
		return nil
	}
	var protocols []string
	for _, s := range h {
		for p := range strings.SplitSeq(s, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				protocols = append(protocols, p)
			}
		}
	}
	return protocols
}

// IsWebSocketUpgrade returns true if the client sent a WebSocket upgrade request
// per RFC 6455, section 4.2.1, items 1 and 2.
func IsWebSocketUpgrade(r *http.Request) bool {
	return headerContainsToken(r.Header, "Connection", "upgrade") &&
		headerContainsToken(r.Header, "Upgrade", "websocket")
}

// headerContainsToken checks if a header contains a specific token using case-insensitive
// comparison. Used for Connection and Upgrade headers where token values are
// case-insensitive per RFC 7230, sections 6.1 and 6.7.
// Tokens may be comma-separated (e.g., "Connection: keep-alive, Upgrade").
func headerContainsToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for t := range strings.SplitSeq(v, ",") {
			if equalASCIIFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

// extension represents a WebSocket extension per RFC 6455, section 9.1.
type extension struct {
	name   string
	params map[string]string
}

// parseExtensions parses Sec-WebSocket-Extensions header per RFC 6455, section 9.1.
// Header field names (keys) are case-insensitive per RFC 7230, section 3.2, which
// Go's net/http handles via canonical form. Header field values, including extension
// names and parameter names, are case-sensitive and preserved as-is.
func parseExtensions(header http.Header) []extension {
	var extensions []extension
	for _, h := range header.Values("Sec-WebSocket-Extensions") {
		for ext := range strings.SplitSeq(h, ",") {
			ext = strings.TrimSpace(ext)
			if ext == "" {
				continue
			}
			parts := strings.Split(ext, ";")
			e := extension{
				name:   strings.TrimSpace(parts[0]),
				params: make(map[string]string),
			}
			for _, param := range parts[1:] {
				param = strings.TrimSpace(param)
				if key, val, ok := strings.Cut(param, "="); ok {
					e.params[strings.TrimSpace(key)] = strings.TrimSpace(val)
				} else {
					e.params[param] = ""
				}
			}
			extensions = append(extensions, e)
		}
	}
	return extensions
}

// negotiateCompressionParams negotiates permessage-deflate parameters per RFC 7692.
// Returns the response parameters string to include in Sec-WebSocket-Extensions.
func negotiateCompressionParams(clientParams map[string]string) string {
	var params []string

	// Server always uses no_context_takeover for stateless compression.
	params = append(params, "server_no_context_takeover")

	// If client requested client_no_context_takeover, acknowledge it.
	if _, ok := clientParams["client_no_context_takeover"]; ok {
		params = append(params, "client_no_context_takeover")
	}

	// If client offered client_max_window_bits, acknowledge with our value.
	// We use the default (15) but acknowledge the parameter.
	if _, ok := clientParams["client_max_window_bits"]; ok {
		params = append(params, "client_max_window_bits=15")
	}

	// Handle server_max_window_bits per RFC 7692, section 7.1.2.1.
	// Valid values are 8-15. If no value is specified, use the default (15).
	if v, ok := clientParams["server_max_window_bits"]; ok {
		if v == "" {
			params = append(params, "server_max_window_bits=15")
		} else {
			bits, err := strconv.Atoi(v)
			if err == nil && bits >= 8 && bits <= 15 {
				params = append(params, fmt.Sprintf("server_max_window_bits=%s", v))
			}
		}
	}

	if len(params) == 0 {
		return ""
	}
	return fmt.Sprintf("; %s", strings.Join(params, "; "))
}
