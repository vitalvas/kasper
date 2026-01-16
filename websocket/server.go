package websocket

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"slices"
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
// This implements the server-side opening handshake per RFC 6455, section 4.2.2.
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*Conn, error) {
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

	conn := newConnFromBufio(netConn, brw, true, u.ReadBufferSize, u.WriteBufferSize, u.WriteBufferPool)
	conn.subprotocol = subprotocol
	conn.compressionEnabled = compress

	return conn, nil
}

func newConnFromBufio(netConn net.Conn, brw *bufio.ReadWriter, isServer bool, readBufferSize, writeBufferSize int, writeBufferPool BufferPool) *Conn {
	c := newConnWithPool(netConn, isServer, readBufferSize, writeBufferSize, writeBufferPool)
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
	return equalASCIIFold(origin, "http://"+r.Host) || equalASCIIFold(origin, "https://"+r.Host)
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
		for _, p := range strings.Split(s, ",") {
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

// headerContainsToken checks if a header contains a specific token (case-insensitive).
// Tokens may be comma-separated (e.g., "Connection: keep-alive, Upgrade").
func headerContainsToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, t := range strings.Split(v, ",") {
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
func parseExtensions(header http.Header) []extension {
	var extensions []extension
	for _, h := range header.Values("Sec-WebSocket-Extensions") {
		for _, ext := range strings.Split(h, ",") {
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
				if idx := strings.Index(param, "="); idx >= 0 {
					e.params[strings.TrimSpace(param[:idx])] = strings.TrimSpace(param[idx+1:])
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

	// Build the response string.
	if len(params) == 0 {
		return ""
	}
	return "; " + strings.Join(params, "; ")
}
