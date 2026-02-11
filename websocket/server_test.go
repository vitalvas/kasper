package websocket

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	t.Run("Match found", func(t *testing.T) {
		u := &Upgrader{
			Subprotocols: []string{"graphql-ws", "graphql-transport-ws"},
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Sec-WebSocket-Protocol", "graphql-transport-ws, chat")

		result := u.selectSubprotocol(r)
		assert.Equal(t, "graphql-transport-ws", result)
	})

	t.Run("No match", func(t *testing.T) {
		u := &Upgrader{
			Subprotocols: []string{"graphql-ws"},
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Sec-WebSocket-Protocol", "chat")

		result := u.selectSubprotocol(r)
		assert.Equal(t, "", result)
	})

	t.Run("Server preference order", func(t *testing.T) {
		u := &Upgrader{
			Subprotocols: []string{"preferred", "fallback"},
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Sec-WebSocket-Protocol", "fallback, preferred")

		result := u.selectSubprotocol(r)
		assert.Equal(t, "preferred", result)
	})
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
		defer client.Close()

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
		defer conn.Close()

		assert.Equal(t, "graphql-ws", conn.Subprotocol())
		assert.True(t, conn.isServer)
	})

	t.Run("With compression", func(t *testing.T) {
		u := &Upgrader{
			EnableCompression: true,
		}

		server, client := net.Pipe()
		defer client.Close()

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
		defer conn.Close()

		assert.True(t, conn.compressionEnabled)
	})

	t.Run("With response headers", func(t *testing.T) {
		u := &Upgrader{}

		server, client := net.Pipe()
		defer client.Close()

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
		defer conn.Close()
	})
}

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

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         bufio.NewReader(strings.NewReader("")),
			writer:         bufio.NewWriter(new(bytes.Buffer)),
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"
		r.Header.Set("Sec-WebSocket-Protocol", "chat")

		responseHeader := make(http.Header)
		responseHeader.Set("X-Custom", "value")

		conn, err := u.Upgrade(hijacker, r, responseHeader)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.Equal(t, "chat", conn.Subprotocol())
		assert.True(t, conn.isServer)

		recorder := hijacker.ResponseWriter.(*httptest.ResponseRecorder)
		assert.Equal(t, http.StatusOK, recorder.Code)
	})

	t.Run("With compression", func(t *testing.T) {
		u := &Upgrader{
			CheckOrigin:       func(_ *http.Request) bool { return true },
			EnableCompression: true,
		}

		server, client := net.Pipe()

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         bufio.NewReader(strings.NewReader("")),
			writer:         bufio.NewWriter(new(bytes.Buffer)),
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"
		r.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")

		conn, err := u.Upgrade(hijacker, r, nil)
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

		hijacker := &mockHijacker{
			ResponseWriter: httptest.NewRecorder(),
			conn:           server,
			reader:         bufio.NewReader(strings.NewReader("")),
			writer:         bufio.NewWriter(new(bytes.Buffer)),
		}

		r := httptest.NewRequest(http.MethodConnect, "/ws", nil)
		r.ProtoMajor = 2
		r.Proto = "websocket"

		responseHeader := make(http.Header)
		responseHeader.Set("X-Custom", "custom-value")

		conn, err := u.Upgrade(hijacker, r, responseHeader)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.Equal(t, "custom-value", hijacker.Header().Get("X-Custom"))
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
		defer client.Close()

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
		defer conn.Close()
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
