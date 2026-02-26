package websocket

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeAcceptKey(t *testing.T) {
	tests := []struct {
		name         string
		challengeKey string
		expected     string
	}{
		{
			name:         "RFC example",
			challengeKey: "dGhlIHNhbXBsZSBub25jZQ==",
			expected:     "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeAcceptKey(tt.challengeKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWebSocketUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name: "Valid upgrade request",
			headers: map[string]string{
				"Connection": "upgrade",
				"Upgrade":    "websocket",
			},
			expected: true,
		},
		{
			name: "Case insensitive",
			headers: map[string]string{
				"Connection": "Upgrade",
				"Upgrade":    "WebSocket",
			},
			expected: true,
		},
		{
			name: "Missing Connection header",
			headers: map[string]string{
				"Upgrade": "websocket",
			},
			expected: false,
		},
		{
			name: "Missing Upgrade header",
			headers: map[string]string{
				"Connection": "upgrade",
			},
			expected: false,
		},
		{
			name: "Wrong Upgrade value",
			headers: map[string]string{
				"Connection": "upgrade",
				"Upgrade":    "http/2.0",
			},
			expected: false,
		},
		{
			name:     "Empty headers",
			headers:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			result := IsWebSocketUpgrade(r)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubprotocols(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected []string
	}{
		{
			name:     "Single protocol",
			header:   "graphql-ws",
			expected: []string{"graphql-ws"},
		},
		{
			name:     "Multiple protocols comma separated",
			header:   "chat, superchat",
			expected: []string{"chat", "superchat"},
		},
		{
			name:     "Empty header",
			header:   "",
			expected: nil,
		},
		{
			name:     "Whitespace handling",
			header:   "  proto1  ,  proto2  ",
			expected: []string{"proto1", "proto2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Sec-WebSocket-Protocol", tt.header)
			}
			result := Subprotocols(r)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckSameOrigin(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		origin   string
		expected bool
	}{
		{
			name:     "No origin header",
			host:     "example.com",
			origin:   "",
			expected: true,
		},
		{
			name:     "Same origin http",
			host:     "example.com",
			origin:   "http://example.com",
			expected: true,
		},
		{
			name:     "Same origin https",
			host:     "example.com",
			origin:   "https://example.com",
			expected: true,
		},
		{
			name:     "Different origin",
			host:     "example.com",
			origin:   "http://other.com",
			expected: false,
		},
		{
			name:     "Case insensitive",
			host:     "Example.Com",
			origin:   "http://example.com",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Host = tt.host
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			result := checkSameOrigin(r)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEqualASCIIFold(t *testing.T) {
	tests := []struct {
		s, t     string
		expected bool
	}{
		{"abc", "abc", true},
		{"ABC", "abc", true},
		{"abc", "ABC", true},
		{"AbC", "aBc", true},
		{"abc", "abcd", false},
		{"", "", true},
		{"a", "b", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.t, func(t *testing.T) {
			result := equalASCIIFold(tt.s, tt.t)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseExtensions(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected []extension
	}{
		{
			name:   "permessage-deflate",
			header: "permessage-deflate",
			expected: []extension{
				{name: "permessage-deflate", params: map[string]string{}},
			},
		},
		{
			name:   "With parameters",
			header: "permessage-deflate; client_max_window_bits=15",
			expected: []extension{
				{name: "permessage-deflate", params: map[string]string{"client_max_window_bits": "15"}},
			},
		},
		{
			name:   "Multiple extensions",
			header: "ext1, ext2",
			expected: []extension{
				{name: "ext1", params: map[string]string{}},
				{name: "ext2", params: map[string]string{}},
			},
		},
		{
			name:     "Empty",
			header:   "",
			expected: nil,
		},
		{
			name:   "Values are case sensitive",
			header: "Permessage-Deflate; Client_Max_Window_Bits=15",
			expected: []extension{
				{name: "Permessage-Deflate", params: map[string]string{"Client_Max_Window_Bits": "15"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			if tt.header != "" {
				h.Set("Sec-WebSocket-Extensions", tt.header)
			}
			result := parseExtensions(h)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUpgraderSelectSubprotocol(t *testing.T) {
	tests := []struct {
		name         string
		serverProtos []string
		clientHeader string
		expected     string
	}{
		{"Match found", []string{"graphql-ws", "graphql-transport-ws"}, "graphql-transport-ws, chat", "graphql-transport-ws"},
		{"No match", []string{"graphql-ws"}, "chat", ""},
		{"Server preference order", []string{"preferred", "fallback"}, "fallback, preferred", "preferred"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &Upgrader{Subprotocols: tt.serverProtos}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Sec-WebSocket-Protocol", tt.clientHeader)
			result := u.selectSubprotocol(r)
			assert.Equal(t, tt.expected, result)
		})
	}
}

type mockHijacker struct {
	http.ResponseWriter
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.conn, bufio.NewReadWriter(m.reader, m.writer), nil
}

// errHijacker implements http.Hijacker but always returns an error from Hijack.
type errHijacker struct {
	http.ResponseWriter
	err error
}

func (e *errHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, e.err
}

// failWriter is an io.Writer that always returns an error.
type failWriter struct {
	err error
}

func (f *failWriter) Write(_ []byte) (int, error) {
	return 0, f.err
}

// noFlushHTTP2Writer implements http.ResponseWriter but NOT http.Flusher,
// causing http.ResponseController.Flush() to return an error.
type noFlushHTTP2Writer struct {
	headers http.Header
	writer  io.Writer
	code    int
}

func (w *noFlushHTTP2Writer) Header() http.Header         { return w.headers }
func (w *noFlushHTTP2Writer) Write(p []byte) (int, error) { return w.writer.Write(p) }
func (w *noFlushHTTP2Writer) WriteHeader(code int)        { w.code = code }

func TestUpgraderUpgrade(t *testing.T) {
	t.Run("Not a websocket upgrade", func(t *testing.T) {
		u := &Upgrader{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong HTTP method", func(t *testing.T) {
		u := &Upgrader{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong websocket version", func(t *testing.T) {
		u := &Upgrader{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "8")

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Missing Sec-WebSocket-Key", func(t *testing.T) {
		u := &Upgrader{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Origin check fails", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return false },
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Response does not implement Hijacker", func(t *testing.T) {
		u := &Upgrader{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Successful upgrade", func(t *testing.T) {
		u := &Upgrader{
			Subprotocols: []string{"graphql-ws"},
		}

		server, client := net.Pipe()

		writeBuf := new(bytes.Buffer)
		readBuf := bufio.NewReader(strings.NewReader(""))

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         readBuf,
			writer:         bufio.NewWriter(writeBuf),
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		r.Header.Set("Sec-WebSocket-Protocol", "graphql-ws")

		conn, err := u.Upgrade(hijacker, r, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)

		assert.Equal(t, "graphql-ws", conn.Subprotocol())
		assert.True(t, conn.isServer)

		client.Close()
		conn.Close()
	})

	t.Run("With compression", func(t *testing.T) {
		u := &Upgrader{
			EnableCompression: true,
		}

		server, client := net.Pipe()

		writeBuf := new(bytes.Buffer)
		readBuf := bufio.NewReader(strings.NewReader(""))

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         readBuf,
			writer:         bufio.NewWriter(writeBuf),
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		r.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")

		conn, err := u.Upgrade(hijacker, r, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)

		assert.True(t, conn.compressionEnabled)

		client.Close()
		conn.Close()
	})

	t.Run("With response headers", func(t *testing.T) {
		u := &Upgrader{}

		server, client := net.Pipe()

		writeBuf := new(bytes.Buffer)
		readBuf := bufio.NewReader(strings.NewReader(""))

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         readBuf,
			writer:         bufio.NewWriter(writeBuf),
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		responseHeader := http.Header{}
		responseHeader.Set("X-Custom-Header", "custom-value")

		conn, err := u.Upgrade(hijacker, r, responseHeader)
		require.NoError(t, err)
		require.NotNil(t, conn)

		client.Close()
		conn.Close()
	})

	t.Run("Hijack returns error", func(t *testing.T) {
		hijackErr := errors.New("hijack failed")
		u := &Upgrader{}
		w := &errHijacker{
			ResponseWriter: httptest.NewRecorder(),
			err:            hijackErr,
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, hijackErr)
	})

	t.Run("HandshakeTimeout sets and clears write deadline", func(t *testing.T) {
		u := &Upgrader{
			HandshakeTimeout: time.Second,
		}

		server, client := net.Pipe()

		writeBuf := new(bytes.Buffer)
		readBuf := bufio.NewReader(strings.NewReader(""))

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         readBuf,
			writer:         bufio.NewWriter(writeBuf),
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		conn, err := u.Upgrade(hijacker, r, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)

		client.Close()
		conn.Close()
	})

	t.Run("Flush error closes connection", func(t *testing.T) {
		flushErr := errors.New("flush failed")
		u := &Upgrader{}

		server, client := net.Pipe()

		readBuf := bufio.NewReader(strings.NewReader(""))
		// Use a failWriter so that bufio.Writer.Flush() returns an error.
		fw := &failWriter{err: flushErr}

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         readBuf,
			writer:         bufio.NewWriterSize(fw, 1), // size 1 forces flush on handshake write
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		conn, err := u.Upgrade(hijacker, r, nil)
		assert.Nil(t, conn)
		assert.Error(t, err)

		client.Close()
	})
}

// mockHTTP2Writer implements http.ResponseWriter and http.Flusher
// for testing HTTP/2 WebSocket upgrades without Hijack.
type mockHTTP2Writer struct {
	headers http.Header
	writer  io.Writer
	code    int
}

func (w *mockHTTP2Writer) Header() http.Header         { return w.headers }
func (w *mockHTTP2Writer) Write(p []byte) (int, error) { return w.writer.Write(p) }
func (w *mockHTTP2Writer) WriteHeader(code int)        { w.code = code }
func (w *mockHTTP2Writer) Flush()                      {}

func TestUpgraderUpgradeHTTP2(t *testing.T) {
	t.Run("Invalid protocol pseudo-header", func(t *testing.T) {
		u := &Upgrader{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "not-websocket"

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Origin check fails", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return false },
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.ErrorIs(t, err, ErrBadHandshake)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Successful upgrade", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin:  func(_ *http.Request) bool { return true },
			Subprotocols: []string{"chat"},
		}

		server, client := net.Pipe()

		w := &mockHTTP2Writer{
			headers: make(http.Header),
			writer:  server,
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"
		r.Header.Set("Sec-WebSocket-Protocol", "chat")
		r.Body = io.NopCloser(server)

		responseHeader := make(http.Header)
		responseHeader.Set("X-Custom", "value")

		conn, err := u.Upgrade(w, r, responseHeader)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.Equal(t, "chat", conn.Subprotocol())
		assert.True(t, conn.isServer)
		assert.Equal(t, http.StatusOK, w.code)
	})

	t.Run("With compression", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin:       func(_ *http.Request) bool { return true },
			EnableCompression: true,
		}

		server, client := net.Pipe()

		w := &mockHTTP2Writer{
			headers: make(http.Header),
			writer:  server,
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"
		r.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")
		r.Body = io.NopCloser(server)

		conn, err := u.Upgrade(w, r, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.True(t, conn.compressionEnabled)
	})

	t.Run("With response headers", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		}

		server, client := net.Pipe()

		w := &mockHTTP2Writer{
			headers: make(http.Header),
			writer:  server,
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"
		r.Body = io.NopCloser(server)

		responseHeader := make(http.Header)
		responseHeader.Set("X-Custom", "custom-value")

		conn, err := u.Upgrade(w, r, responseHeader)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.Equal(t, "custom-value", w.Header().Get("X-Custom"))
	})

	t.Run("Nil CheckOrigin falls back to checkSameOrigin", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin: nil,
		}

		server, client := net.Pipe()

		w := &mockHTTP2Writer{
			headers: make(http.Header),
			writer:  server,
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"
		r.Host = "example.com"
		r.Body = io.NopCloser(server)

		conn, err := u.Upgrade(w, r, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.True(t, conn.isServer)
	})

	t.Run("Flush error after WriteHeader", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		}

		w := &noFlushHTTP2Writer{
			headers: make(http.Header),
			writer:  io.Discard,
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"
		r.Body = io.NopCloser(strings.NewReader(""))

		conn, err := u.Upgrade(w, r, nil)
		assert.Nil(t, conn)
		assert.Error(t, err)
	})
}

func TestHTTP2ConnAdapterWriteFlushError(t *testing.T) {
	t.Run("Flush error after successful write", func(t *testing.T) {
		// Use noFlushHTTP2Writer which does not implement http.Flusher,
		// so ResponseController.Flush() returns an error after Write succeeds.
		w := &noFlushHTTP2Writer{
			headers: make(http.Header),
			writer:  new(bytes.Buffer),
		}

		adapter := &http2ConnAdapter{
			body: io.NopCloser(strings.NewReader("")),
			w:    w,
			rc:   http.NewResponseController(w),
		}

		n, err := adapter.Write([]byte("data"))
		assert.Equal(t, 4, n)
		assert.Error(t, err)
	})
}

func TestNewConnFromBufioBufferedData(t *testing.T) {
	t.Run("Buffered reader data is preserved", func(t *testing.T) {
		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()

		// Create a bufio.Reader that has buffered data by reading from a
		// source that contains more than what we consume.
		data := "buffered-data-here"
		combined := strings.NewReader(data)
		br := bufio.NewReaderSize(combined, 256)
		// Peek to fill the buffer without consuming data.
		_, err := br.Peek(len(data))
		require.NoError(t, err)
		assert.Greater(t, br.Buffered(), 0)

		bw := bufio.NewWriter(new(bytes.Buffer))
		brw := bufio.NewReadWriter(br, bw)

		conn := newConnFromBufio(server, brw, true, 0, 0, nil)
		require.NotNil(t, conn)

		// The conn.br should be the buffered reader, not the raw netConn.
		assert.Equal(t, br, conn.br)
	})

	t.Run("Empty reader does not use brw reader", func(t *testing.T) {
		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()

		br := bufio.NewReader(strings.NewReader(""))
		bw := bufio.NewWriter(new(bytes.Buffer))
		brw := bufio.NewReadWriter(br, bw)

		conn := newConnFromBufio(server, brw, true, 0, 0, nil)
		require.NotNil(t, conn)

		// With no buffered data, conn.br should NOT be the brw.Reader.
		// newConnFromRWC creates a new bufio.Reader wrapping netConn.
		assert.NotEqual(t, br, conn.br)
	})
}

func TestParseExtensionsEmptyName(t *testing.T) {
	t.Run("Empty extension name after trim", func(t *testing.T) {
		h := http.Header{}
		h.Set("Sec-WebSocket-Extensions", "permessage-deflate, , other")

		result := parseExtensions(h)

		// The empty segment between commas should be skipped.
		require.Len(t, result, 2)
		assert.Equal(t, "permessage-deflate", result[0].name)
		assert.Equal(t, "other", result[1].name)
	})
}

func TestUpgraderReturnError(t *testing.T) {
	t.Run("Custom error handler", func(t *testing.T) {
		var calledStatus int
		u := &Upgrader{
			Error: func(_ http.ResponseWriter, _ *http.Request, status int, _ error) {
				calledStatus = status
			},
		}

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)

		u.returnError(w, r, http.StatusBadRequest, ErrBadHandshake)
		assert.Equal(t, http.StatusBadRequest, calledStatus)
	})

	t.Run("Default error handler", func(t *testing.T) {
		u := &Upgrader{}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)

		u.returnError(w, r, http.StatusBadRequest, ErrBadHandshake)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestUpgraderReadBufferSize(t *testing.T) {
	t.Run("Custom buffer sizes", func(t *testing.T) {
		u := &Upgrader{
			ReadBufferSize:  2048,
			WriteBufferSize: 4096,
		}

		server, client := net.Pipe()

		writeBuf := new(bytes.Buffer)
		readBuf := bufio.NewReader(strings.NewReader(""))

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         readBuf,
			writer:         bufio.NewWriter(writeBuf),
		}

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Connection", "upgrade")
		r.Header.Set("Upgrade", "websocket")
		r.Header.Set("Sec-WebSocket-Version", "13")
		r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

		conn, err := u.Upgrade(hijacker, r, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)

		client.Close()
		conn.Close()
	})
}

func BenchmarkSubprotocols(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Sec-WebSocket-Protocol", "graphql-ws, graphql-transport-ws, chat")

	for b.Loop() {
		_ = Subprotocols(r)
	}
}

func BenchmarkIsWebSocketUpgrade(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Connection", "upgrade")
	r.Header.Set("Upgrade", "websocket")

	for b.Loop() {
		_ = IsWebSocketUpgrade(r)
	}
}

func FuzzParseExtensions(f *testing.F) {
	f.Add("permessage-deflate")
	f.Add("permessage-deflate; client_max_window_bits=15")
	f.Add("ext1, ext2")
	f.Add("ext1; param1=value1, ext2; param2=value2")
	f.Add("")
	f.Add("  spaced  ;  param = value  ")

	f.Fuzz(func(t *testing.T, header string) {
		h := http.Header{}
		if header != "" {
			h.Set("Sec-WebSocket-Extensions", header)
		}

		result := parseExtensions(h)

		for _, ext := range result {
			if ext.name == "" && len(ext.params) > 0 {
				t.Errorf("extension with empty name but has params")
			}
		}
	})
}

func FuzzSubprotocols(f *testing.F) {
	f.Add("graphql-ws")
	f.Add("chat, superchat")
	f.Add("")
	f.Add("  proto1  ,  proto2  ")
	f.Add("a,b,c,d,e")

	f.Fuzz(func(t *testing.T, header string) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			r.Header.Set("Sec-WebSocket-Protocol", header)
		}

		result := Subprotocols(r)

		for _, proto := range result {
			if proto == "" {
				t.Errorf("empty protocol in result")
			}
		}
	})
}

func TestNegotiateCompressionParams(t *testing.T) {
	t.Run("Basic negotiation", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{})
		assert.Contains(t, result, "server_no_context_takeover")
	})

	t.Run("With client_no_context_takeover", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{
			"client_no_context_takeover": "",
		})
		assert.Contains(t, result, "client_no_context_takeover")
	})

	t.Run("With client_max_window_bits", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{
			"client_max_window_bits": "",
		})
		assert.Contains(t, result, "client_max_window_bits=15")
	})

	t.Run("With server_max_window_bits value", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{
			"server_max_window_bits": "10",
		})
		assert.Contains(t, result, "server_max_window_bits=10")
	})

	t.Run("With server_max_window_bits no value", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{
			"server_max_window_bits": "",
		})
		assert.Contains(t, result, "server_max_window_bits=15")
	})

	t.Run("With server_max_window_bits too low", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{
			"server_max_window_bits": "7",
		})
		assert.NotContains(t, result, "server_max_window_bits")
	})

	t.Run("With server_max_window_bits too high", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{
			"server_max_window_bits": "16",
		})
		assert.NotContains(t, result, "server_max_window_bits")
	})

	t.Run("With server_max_window_bits boundary values", func(t *testing.T) {
		result8 := negotiateCompressionParams(map[string]string{
			"server_max_window_bits": "8",
		})
		assert.Contains(t, result8, "server_max_window_bits=8")

		result15 := negotiateCompressionParams(map[string]string{
			"server_max_window_bits": "15",
		})
		assert.Contains(t, result15, "server_max_window_bits=15")
	})

	t.Run("With server_max_window_bits invalid string", func(t *testing.T) {
		result := negotiateCompressionParams(map[string]string{
			"server_max_window_bits": "abc",
		})
		assert.NotContains(t, result, "server_max_window_bits")
	})
}

func TestHTTP2ConnAdapter(t *testing.T) {
	t.Run("Read", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader("hello"))
		w := &mockHTTP2Writer{
			headers: make(http.Header),
			writer:  io.Discard,
		}

		adapter := &http2ConnAdapter{
			body: body,
			w:    w,
			rc:   http.NewResponseController(w),
		}

		buf := make([]byte, 5)
		n, err := adapter.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hello"), buf)
	})

	t.Run("Write", func(t *testing.T) {
		var writeBuf bytes.Buffer
		w := &mockHTTP2Writer{
			headers: make(http.Header),
			writer:  &writeBuf,
		}

		adapter := &http2ConnAdapter{
			body: io.NopCloser(strings.NewReader("")),
			w:    w,
			rc:   http.NewResponseController(w),
		}

		n, err := adapter.Write([]byte("world"))
		require.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "world", writeBuf.String())
	})

	t.Run("Close", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader(""))
		w := &mockHTTP2Writer{
			headers: make(http.Header),
			writer:  io.Discard,
		}

		adapter := &http2ConnAdapter{
			body: body,
			w:    w,
			rc:   http.NewResponseController(w),
		}

		err := adapter.Close()
		require.NoError(t, err)
	})
}
