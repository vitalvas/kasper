package websocket

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

// roundTripperFunc adapts a function to http.RoundTripper for testing.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestDialerDialURLParsing(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{
			name:    "Invalid URL",
			url:     "://invalid",
			wantErr: "missing protocol scheme",
		},
		{
			name:    "Bad scheme",
			url:     "http://example.com",
			wantErr: "bad scheme",
		},
		{
			name:    "Empty host",
			url:     "ws:///path",
			wantErr: "empty host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Dialer{}
			_, _, err := d.Dial(tt.url, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDialerDialContext(t *testing.T) {
	t.Run("Context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		d := &Dialer{}
		_, _, err := d.DialContext(ctx, "ws://example.com", nil)
		require.Error(t, err)
	})
}

func TestDialerDial(t *testing.T) {
	t.Run("Custom HTTPClient Transport DialContext", func(t *testing.T) {
		called := false
		transport := &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				called = true
				return nil, net.ErrClosed
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		_, _, _ = d.Dial("ws://example.com", nil)
		assert.True(t, called)
	})

	t.Run("Custom DialTLSContext", func(t *testing.T) {
		called := false
		transport := &http.Transport{
			DialTLSContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				called = true
				return nil, net.ErrClosed
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		_, _, _ = d.Dial("wss://example.com", nil)
		assert.True(t, called)
	})
}

func TestGenerateChallengeKey(t *testing.T) {
	key1, err := generateChallengeKey()
	require.NoError(t, err)

	key2, err := generateChallengeKey()
	require.NoError(t, err)

	assert.Len(t, key1, 24)
	assert.Len(t, key2, 24)
	assert.NotEqual(t, key1, key2)
}

func TestDefaultDialer(t *testing.T) {
	assert.NotNil(t, DefaultDialer)
}

func TestDialerHandshakeTimeout(t *testing.T) {
	t.Run("Timeout on default path", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		go func() {
			conn, _ := listener.Accept()
			if conn != nil {
				time.Sleep(200 * time.Millisecond)
				conn.Close()
			}
		}()

		// Use default dialer (no custom transport) to hit dialDirect path.
		d := &Dialer{
			HandshakeTimeout: 50 * time.Millisecond,
		}

		addr := listener.Addr().String()
		_, _, err = d.Dial("ws://"+addr, nil)
		require.Error(t, err)
	})

	t.Run("Timeout on connection", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		go func() {
			conn, _ := listener.Accept()
			if conn != nil {
				time.Sleep(200 * time.Millisecond)
				conn.Close()
			}
		}()

		// Use custom dial via transport.
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, addr)
			},
		}

		d := &Dialer{
			HTTPClient:       &http.Client{Transport: transport},
			HandshakeTimeout: 50 * time.Millisecond,
		}

		addr := listener.Addr().String()
		_, _, err = d.Dial("ws://"+addr, nil)
		require.Error(t, err)
	})
}

func TestDialerWithServer(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Successful connection and echo", func(t *testing.T) {
		d := &Dialer{}
		conn, resp, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.NotNil(t, resp)
		defer conn.Close()

		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

		err = conn.WriteMessage(TextMessage, []byte("hello"))
		require.NoError(t, err)

		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("hello"), msg)
	})

	t.Run("With custom headers", func(t *testing.T) {
		d := &Dialer{}
		headers := http.Header{}
		headers.Set("X-Custom-Header", "test-value")

		conn, _, err := d.Dial(wsURL, headers)
		require.NoError(t, err)
		defer conn.Close()
	})

	t.Run("With custom DialContext", func(t *testing.T) {
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, addr)
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		conn, resp, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()

		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

		err = conn.WriteMessage(TextMessage, []byte("custom-dial"))
		require.NoError(t, err)

		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("custom-dial"), msg)
	})
}

func TestDialerWithSubprotocols(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin:  func(_ *http.Request) bool { return true },
		Subprotocols: []string{"graphql-ws", "graphql-transport-ws"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Subprotocol negotiation", func(t *testing.T) {
		d := &Dialer{
			Subprotocols: []string{"graphql-transport-ws"},
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		assert.Equal(t, "graphql-transport-ws", conn.Subprotocol())
	})
}

func TestDialerBadHandshakeResponse(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
	}{
		{
			name: "Non-101 status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "Wrong Upgrade header",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Upgrade", "http/2.0")
				w.Header().Set("Connection", "upgrade")
				w.WriteHeader(http.StatusSwitchingProtocols)
			},
			wantStatus: 0,
		},
		{
			name: "Wrong Connection header",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Upgrade", "websocket")
				w.Header().Set("Connection", "close")
				w.WriteHeader(http.StatusSwitchingProtocols)
			},
			wantStatus: 0,
		},
		{
			name: "Wrong Sec-WebSocket-Accept",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Upgrade", "websocket")
				w.Header().Set("Connection", "upgrade")
				w.Header().Set("Sec-WebSocket-Accept", "wrong-accept-key")
				w.WriteHeader(http.StatusSwitchingProtocols)
			},
			wantStatus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			d := &Dialer{}

			_, resp, err := d.Dial(wsURL, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrBadHandshake)

			if tt.wantStatus > 0 {
				assert.NotNil(t, resp)
				assert.Equal(t, tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

func TestDialerWithCompression(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin:       func(_ *http.Request) bool { return true },
		EnableCompression: true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Compression negotiation", func(t *testing.T) {
		d := &Dialer{
			EnableCompression: true,
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		assert.True(t, conn.compressionEnabled)
	})
}

func TestDialerContextTimeout(t *testing.T) {
	t.Run("Context timeout during dial", func(t *testing.T) {
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_, _, err := d.DialContext(ctx, "ws://example.com", nil)
		assert.Error(t, err)
	})
}

func TestDialerWithTLSServer(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer server.Close()

	wsURL := "wss" + strings.TrimPrefix(server.URL, "https")

	t.Run("Connect to TLS server via http.Client", func(t *testing.T) {
		d := &Dialer{
			HTTPClient: server.Client(),
		}

		conn, resp, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		require.NotNil(t, resp)
		defer conn.Close()

		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

		err = conn.WriteMessage(TextMessage, []byte("tls-hello"))
		require.NoError(t, err)

		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("tls-hello"), msg)
	})

	t.Run("Connect to TLS server with custom transport", func(t *testing.T) {
		// Get TLS config from test server's client
		serverTransport := server.Client().Transport.(*http.Transport)

		transport := &http.Transport{
			TLSClientConfig: serverTransport.TLSClientConfig,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, addr)
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		conn, resp, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()

		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	})
}

func TestDialerIsHTTP2(t *testing.T) {
	tests := []struct {
		name      string
		transport http.RoundTripper
		expected  bool
	}{
		{"Nil transport is not HTTP/2", nil, false},
		{"http.Transport is not HTTP/2", &http.Transport{}, false},
		{"http2.Transport is HTTP/2", &http2.Transport{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Dialer{}
			client := &http.Client{Transport: tt.transport}
			assert.Equal(t, tt.expected, d.isHTTP2(client))
		})
	}
}

func TestDialerNilHTTPClient(t *testing.T) {
	t.Run("Uses default client when HTTPClient is nil", func(t *testing.T) {
		upgrader := &Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			conn.Close()
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		d := &Dialer{
			HTTPClient: nil, // Explicitly nil
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()
	})
}

func TestDialerGetProxyURL(t *testing.T) {
	t.Run("No proxy when HTTPClient is nil", func(t *testing.T) {
		d := &Dialer{}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(u))
	})

	t.Run("No proxy when Proxy function is nil", func(t *testing.T) {
		d := &Dialer{
			HTTPClient: &http.Client{Transport: &http.Transport{}},
		}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(u))
	})

	t.Run("Returns proxy URL from transport", func(t *testing.T) {
		proxyURL, _ := url.Parse("http://proxy.example.com:8080")
		d := &Dialer{
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					Proxy: func(_ *http.Request) (*url.URL, error) {
						return proxyURL, nil
					},
				},
			},
		}
		u, _ := url.Parse("http://example.com")
		assert.Equal(t, proxyURL, d.getProxyURL(u))
	})

	t.Run("d.Proxy takes precedence over transport", func(t *testing.T) {
		directProxyURL, _ := url.Parse("http://direct-proxy.example.com:8080")
		transportProxyURL, _ := url.Parse("http://transport-proxy.example.com:8080")
		d := &Dialer{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return directProxyURL, nil
			},
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					Proxy: func(_ *http.Request) (*url.URL, error) {
						return transportProxyURL, nil
					},
				},
			},
		}
		u, _ := url.Parse("http://example.com")
		assert.Equal(t, directProxyURL, d.getProxyURL(u))
	})
}

func TestHostPortFromURL(t *testing.T) {
	tests := []struct {
		name     string
		urlStr   string
		expected string
	}{
		{"HTTP with port", "http://example.com:8080/path", "example.com:8080"},
		{"HTTPS with port", "https://example.com:8443/path", "example.com:8443"},
		{"HTTP default port", "http://example.com/path", "example.com:80"},
		{"HTTPS default port", "https://example.com/path", "example.com:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse(tt.urlStr)
			assert.Equal(t, tt.expected, hostPortFromURL(u))
		})
	}
}

func TestDialerDefaultPort(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		useTLS   bool
		expected string
	}{
		{"WS default port 80", "ws://example.com/path", false, "example.com:80"},
		{"WSS default port 443", "wss://example.com/path", true, "example.com:443"},
		{"Custom port preserved", "ws://example.com:8080/path", false, "example.com:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dialedAddr string
			dialFn := func(_ context.Context, _, addr string) (net.Conn, error) {
				dialedAddr = addr
				return nil, net.ErrClosed
			}

			var transport *http.Transport
			if tt.useTLS {
				transport = &http.Transport{DialTLSContext: dialFn}
			} else {
				transport = &http.Transport{DialContext: dialFn}
			}

			d := &Dialer{HTTPClient: &http.Client{Transport: transport}}
			_, _, _ = d.Dial(tt.url, nil)
			assert.Equal(t, tt.expected, dialedAddr)
		})
	}
}

// newTestCONNECTProxy returns an HTTP CONNECT proxy test server.
func newTestCONNECTProxy(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "expected CONNECT", http.StatusMethodNotAllowed)
			return
		}

		targetConn, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}

		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

		go func() {
			_, _ = io.Copy(targetConn, clientConn)
		}()
		_, _ = io.Copy(clientConn, targetConn)

		clientConn.Close()
		targetConn.Close()
	}))
}

func TestDialerWithProxy(t *testing.T) {
	// Start a WebSocket server.
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer wsServer.Close()

	proxyServer := newTestCONNECTProxy(t)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	t.Run("Connect through proxy", func(t *testing.T) {
		transport := &http.Transport{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		err = conn.WriteMessage(TextMessage, []byte("through-proxy"))
		require.NoError(t, err)

		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("through-proxy"), msg)
	})
}

func TestDialerProxyError(t *testing.T) {
	// Proxy that returns an error.
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "proxy error", http.StatusForbidden)
	}))
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)

	transport := &http.Transport{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
	}

	d := &Dialer{
		HTTPClient: &http.Client{Transport: transport},
	}

	_, _, err := d.Dial("ws://example.com/ws", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "proxy CONNECT failed")
}

func TestConnUnderlyingConn(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Server-side connection should have underlying net.Conn.
		assert.NotNil(t, conn.UnderlyingConn())
		assert.NotNil(t, conn.LocalAddr())
		assert.NotNil(t, conn.RemoteAddr())

		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	d := &Dialer{}
	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Default dialer now uses dialDirect which preserves net.Conn.
	assert.NotNil(t, conn.UnderlyingConn())
	assert.NotNil(t, conn.LocalAddr())
	assert.NotNil(t, conn.RemoteAddr())

	err = conn.WriteMessage(TextMessage, []byte("test"))
	require.NoError(t, err)
}

func TestConnCloseWithBufferPool(t *testing.T) {
	pool := &testBufferPool{}

	upgrader := &Upgrader{
		CheckOrigin:     func(_ *http.Request) bool { return true },
		WriteBufferPool: pool,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	d := &Dialer{}
	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	conn.Close()
}

func TestDialerDoHandshakeWithTransport(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin:  func(_ *http.Request) bool { return true },
		Subprotocols: []string{"test-proto"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("With subprotocol", func(t *testing.T) {
		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, addr)
			},
		}

		d := &Dialer{
			HTTPClient:   &http.Client{Transport: transport},
			Subprotocols: []string{"test-proto"},
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		assert.Equal(t, "test-proto", conn.Subprotocol())
	})

	t.Run("With compression", func(t *testing.T) {
		compressionUpgrader := &Upgrader{
			CheckOrigin:       func(_ *http.Request) bool { return true },
			EnableCompression: true,
		}

		compressionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := compressionUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			conn.Close()
		}))
		defer compressionServer.Close()

		wsURL := "ws" + strings.TrimPrefix(compressionServer.URL, "http")

		transport := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, addr)
			},
		}

		d := &Dialer{
			HTTPClient:        &http.Client{Transport: transport},
			EnableCompression: true,
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		assert.True(t, conn.compressionEnabled)
	})
}

func TestDialerDialHTTP2(t *testing.T) {
	t.Run("Sends extended CONNECT request", func(t *testing.T) {
		server, client := net.Pipe()

		d := &Dialer{
			Subprotocols:      []string{"chat"},
			EnableCompression: true,
		}

		h := make(http.Header)
		h.Set("Sec-WebSocket-Protocol", "chat")

		httpClient := &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodConnect, req.Method)
				assert.Equal(t, "websocket", req.Proto)
				assert.Equal(t, "example.com", req.Host)
				assert.Equal(t, "chat", req.Header.Get("Sec-WebSocket-Protocol"))
				assert.Contains(t, req.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate")
				assert.Equal(t, "custom-value", req.Header.Get("X-Custom"))

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     h,
					Body:       server,
				}, nil
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		headers := make(http.Header)
		headers.Set("X-Custom", "custom-value")

		conn, resp, err := d.dialHTTP2(context.Background(), httpClient, u, headers)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "chat", conn.Subprotocol())
	})

	t.Run("Transport error", func(t *testing.T) {
		d := &Dialer{}
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return nil, net.ErrClosed
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		_, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
		require.Error(t, err)
	})

	t.Run("Non-200 response", func(t *testing.T) {
		d := &Dialer{}
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		_, resp, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("Wrong subprotocol", func(t *testing.T) {
		server, client := net.Pipe()
		defer client.Close()

		h := make(http.Header)
		h.Set("Sec-WebSocket-Protocol", "wrong-proto")

		d := &Dialer{Subprotocols: []string{"expected-proto"}}
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     h,
					Body:       server,
				}, nil
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		_, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Body not ReadWriteCloser", func(t *testing.T) {
		d := &Dialer{}
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		_, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not ReadWriteCloser")
	})

	t.Run("With compression", func(t *testing.T) {
		server, client := net.Pipe()

		h := make(http.Header)
		h.Set("Sec-WebSocket-Extensions", "permessage-deflate")

		d := &Dialer{EnableCompression: true}
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     h,
					Body:       server,
				}, nil
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		conn, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()
		defer client.Close()

		assert.True(t, conn.compressionEnabled)
	})
}

func TestDialerBuildHandshakeHeaders(t *testing.T) {
	t.Run("Sets required headers", func(t *testing.T) {
		d := &Dialer{
			Subprotocols:      []string{"chat", "superchat"},
			EnableCompression: true,
		}

		req := &http.Request{Header: make(http.Header)}
		customHeaders := http.Header{}
		customHeaders.Set("X-Custom", "value")

		d.buildHandshakeHeaders(req, customHeaders, "test-key")

		assert.Equal(t, "websocket", req.Header.Get("Upgrade"))
		assert.Equal(t, "Upgrade", req.Header.Get("Connection"))
		assert.Equal(t, "test-key", req.Header.Get("Sec-WebSocket-Key"))
		assert.Equal(t, websocketVersion, req.Header.Get("Sec-WebSocket-Version"))
		assert.Equal(t, "chat, superchat", req.Header.Get("Sec-WebSocket-Protocol"))
		assert.Contains(t, req.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate")
		assert.Equal(t, "value", req.Header.Get("X-Custom"))
	})

	t.Run("No optional headers when not configured", func(t *testing.T) {
		d := &Dialer{}

		req := &http.Request{Header: make(http.Header)}
		d.buildHandshakeHeaders(req, nil, "key")

		assert.Equal(t, "websocket", req.Header.Get("Upgrade"))
		assert.Empty(t, req.Header.Get("Sec-WebSocket-Protocol"))
		assert.Empty(t, req.Header.Get("Sec-WebSocket-Extensions"))
	})
}

func TestDialerValidateHTTP1Response(t *testing.T) {
	challengeKey := "dGhlIHNhbXBsZSBub25jZQ=="
	acceptKey := computeAcceptKey(challengeKey)

	t.Run("Valid response", func(t *testing.T) {
		d := &Dialer{Subprotocols: []string{"chat"}, EnableCompression: true}

		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)
		resp.Header.Set("Sec-WebSocket-Protocol", "chat")
		resp.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")

		subproto, compress, err := d.validateHTTP1Response(resp, challengeKey)
		require.NoError(t, err)
		assert.Equal(t, "chat", subproto)
		assert.True(t, compress)
	})

	t.Run("Wrong status code", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
		}
		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong upgrade header", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "http/2.0")
		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong connection header", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "close")
		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong accept key", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", "wrong-key")
		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong subprotocol", func(t *testing.T) {
		d := &Dialer{Subprotocols: []string{"expected"}}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)
		resp.Header.Set("Sec-WebSocket-Protocol", "wrong")
		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("No compression when not offered", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)

		_, compress, err := d.validateHTTP1Response(resp, challengeKey)
		require.NoError(t, err)
		assert.False(t, compress)
	})
}

func TestDialerRejectsUnrequestedSubprotocol(t *testing.T) {
	challengeKey := "dGhlIHNhbXBsZSBub25jZQ=="
	acceptKey := computeAcceptKey(challengeKey)

	t.Run("HTTP1 rejects unrequested subprotocol", func(t *testing.T) {
		d := &Dialer{} // No subprotocols requested.
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)
		resp.Header.Set("Sec-WebSocket-Protocol", "chat")

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("HTTP1 accepts when no subprotocol in response", func(t *testing.T) {
		d := &Dialer{} // No subprotocols requested.
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		require.NoError(t, err)
	})
}

func TestDialerRejectsUnrequestedCompression(t *testing.T) {
	challengeKey := "dGhlIHNhbXBsZSBub25jZQ=="
	acceptKey := computeAcceptKey(challengeKey)

	t.Run("HTTP1 rejects unrequested compression", func(t *testing.T) {
		d := &Dialer{} // EnableCompression is false.
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)
		resp.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("HTTP1 accepts compression when requested", func(t *testing.T) {
		d := &Dialer{EnableCompression: true}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)
		resp.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate")

		_, compress, err := d.validateHTTP1Response(resp, challengeKey)
		require.NoError(t, err)
		assert.True(t, compress)
	})

	t.Run("HTTP1 rejects unknown extension with compression enabled", func(t *testing.T) {
		d := &Dialer{EnableCompression: true}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)
		resp.Header.Set("Sec-WebSocket-Extensions", "custom-ext")

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("HTTP1 rejects mixed known and unknown extensions", func(t *testing.T) {
		d := &Dialer{EnableCompression: true}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)
		resp.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate, custom-ext")

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})
}

func TestDialerBadSubprotocolInDoHandshake(t *testing.T) {
	// Server that returns wrong subprotocol.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Sec-WebSocket-Key")
		accept := computeAcceptKey(key)

		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "upgrade")
		w.Header().Set("Sec-WebSocket-Accept", accept)
		w.Header().Set("Sec-WebSocket-Protocol", "wrong-proto")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, addr)
		},
	}

	d := &Dialer{
		HTTPClient:   &http.Client{Transport: transport},
		Subprotocols: []string{"expected-proto"},
	}

	_, _, err := d.Dial(wsURL, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBadHandshake)
}

func TestDialerNetDialContext(t *testing.T) {
	t.Run("NetDialContext takes precedence over transport", func(t *testing.T) {
		netDialCalled := false
		transportDialCalled := false

		d := &Dialer{
			NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				netDialCalled = true
				return nil, net.ErrClosed
			},
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
						transportDialCalled = true
						return nil, net.ErrClosed
					},
				},
			},
		}

		_, _, _ = d.Dial("ws://example.com", nil)
		assert.True(t, netDialCalled)
		assert.False(t, transportDialCalled)
	})

	t.Run("NetDialContext used for wss connections", func(t *testing.T) {
		called := false
		d := &Dialer{
			NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				called = true
				return nil, net.ErrClosed
			},
		}

		_, _, _ = d.Dial("wss://example.com", nil)
		assert.True(t, called)
	})
}

func TestDialerTLSClientConfig(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer server.Close()

	wsURL := "wss" + strings.TrimPrefix(server.URL, "https")

	t.Run("d.TLSClientConfig takes precedence over transport", func(t *testing.T) {
		serverTransport := server.Client().Transport.(*http.Transport)

		d := &Dialer{
			TLSClientConfig: serverTransport.TLSClientConfig,
		}

		conn, resp, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		require.NotNil(t, conn)
		defer conn.Close()

		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

		err = conn.WriteMessage(TextMessage, []byte("tls-config"))
		require.NoError(t, err)

		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("tls-config"), msg)
	})
}

func TestDialerProxyField(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer wsServer.Close()

	proxyServer := newTestCONNECTProxy(t)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	t.Run("d.Proxy used for proxy detection", func(t *testing.T) {
		d := &Dialer{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		err = conn.WriteMessage(TextMessage, []byte("proxy-field"))
		require.NoError(t, err)

		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("proxy-field"), msg)
	})
}

func TestDialerDefaultPathPreservesNetConn(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	d := &Dialer{}
	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.UnderlyingConn(), "UnderlyingConn should be non-nil")
	assert.NotNil(t, conn.LocalAddr(), "LocalAddr should be non-nil")
	assert.NotNil(t, conn.RemoteAddr(), "RemoteAddr should be non-nil")

	// Deadlines should work (not return ErrDeadlineNotSupported).
	err = conn.SetReadDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)

	err = conn.SetWriteDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)

	// Clear deadlines.
	err = conn.SetReadDeadline(time.Time{})
	assert.NoError(t, err)

	err = conn.SetWriteDeadline(time.Time{})
	assert.NoError(t, err)
}

func TestDialerRespBodySafeClose(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	d := &Dialer{}
	conn, resp, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Closing resp.Body should be safe and not kill the WebSocket connection.
	err = resp.Body.Close()
	require.NoError(t, err)

	// Connection should still work after resp.Body.Close().
	err = conn.WriteMessage(TextMessage, []byte("after-body-close"))
	require.NoError(t, err)

	msgType, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, TextMessage, msgType)
	assert.Equal(t, []byte("after-body-close"), msg)
}

func TestDialerTLSConfig(t *testing.T) {
	t.Run("Uses d.TLSClientConfig first", func(t *testing.T) {
		customConfig := &tls.Config{ServerName: "custom.example.com"}
		d := &Dialer{
			TLSClientConfig: customConfig,
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{ServerName: "transport.example.com"},
				},
			},
		}

		cfg := d.tlsConfig("fallback.example.com")
		assert.Equal(t, "custom.example.com", cfg.ServerName)
	})

	t.Run("Falls back to transport.TLSClientConfig", func(t *testing.T) {
		d := &Dialer{
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{ServerName: "transport.example.com"},
				},
			},
		}

		cfg := d.tlsConfig("fallback.example.com")
		assert.Equal(t, "transport.example.com", cfg.ServerName)
	})

	t.Run("Falls back to empty config with server name", func(t *testing.T) {
		d := &Dialer{}
		cfg := d.tlsConfig("fallback.example.com")
		assert.Equal(t, "fallback.example.com", cfg.ServerName)
	})

	t.Run("Sets ServerName when empty in config", func(t *testing.T) {
		d := &Dialer{
			TLSClientConfig: &tls.Config{},
		}

		cfg := d.tlsConfig("auto.example.com")
		assert.Equal(t, "auto.example.com", cfg.ServerName)
	})

	t.Run("Does not override existing ServerName", func(t *testing.T) {
		d := &Dialer{
			TLSClientConfig: &tls.Config{ServerName: "explicit.example.com"},
		}

		cfg := d.tlsConfig("auto.example.com")
		assert.Equal(t, "explicit.example.com", cfg.ServerName)
	})

	t.Run("Clones config to avoid mutation", func(t *testing.T) {
		original := &tls.Config{ServerName: "original.example.com"}
		d := &Dialer{
			TLSClientConfig: original,
		}

		cfg := d.tlsConfig("other.example.com")
		assert.Equal(t, "original.example.com", cfg.ServerName)
		assert.Equal(t, "original.example.com", original.ServerName)
		assert.NotSame(t, original, cfg)
	})
}

func TestDoHandshakePreservesBufferedData(t *testing.T) {
	t.Run("Server sends message immediately after upgrade", func(t *testing.T) {
		upgrader := &Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			// Send a message immediately after upgrade, before reading anything.
			_ = conn.WriteMessage(TextMessage, []byte("server-push"))
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		d := &Dialer{}
		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		// The client must receive the server-pushed message even if it
		// was buffered during the HTTP response read.
		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, "server-push", string(msg))
	})
}

func TestValidateHTTP1ResponseMultiValueHeaders(t *testing.T) {
	challengeKey := "dGhlIHNhbXBsZSBub25jZQ=="
	acceptKey := computeAcceptKey(challengeKey)

	t.Run("Connection header with multiple tokens", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "Upgrade, Keep-Alive")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		require.NoError(t, err)
	})

	t.Run("Upgrade header with multiple tokens", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket, h2c")
		resp.Header.Set("Connection", "upgrade")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		require.NoError(t, err)
	})

	t.Run("Connection header without upgrade token rejected", func(t *testing.T) {
		d := &Dialer{}
		resp := &http.Response{
			StatusCode: http.StatusSwitchingProtocols,
			Header:     make(http.Header),
		}
		resp.Header.Set("Upgrade", "websocket")
		resp.Header.Set("Connection", "Keep-Alive")
		resp.Header.Set("Sec-WebSocket-Accept", acceptKey)

		_, _, err := d.validateHTTP1Response(resp, challengeKey)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})
}

func TestDialDirectHandshakeTimeoutSuccess(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	d := &Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	err = conn.WriteMessage(TextMessage, []byte("timeout-ok"))
	require.NoError(t, err)

	msgType, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, TextMessage, msgType)
	assert.Equal(t, []byte("timeout-ok"), msg)
}

func TestDialWithProxyHandshakeTimeout(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer wsServer.Close()

	proxyServer := newTestCONNECTProxy(t)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	t.Run("Successful with timeout", func(t *testing.T) {
		d := &Dialer{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
			HandshakeTimeout: 5 * time.Second,
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		err = conn.WriteMessage(TextMessage, []byte("proxy-timeout"))
		require.NoError(t, err)

		msgType, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, TextMessage, msgType)
		assert.Equal(t, []byte("proxy-timeout"), msg)
	})

	t.Run("Handshake failure through proxy", func(t *testing.T) {
		badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer badServer.Close()

		badWsURL := "ws" + strings.TrimPrefix(badServer.URL, "http")

		d := &Dialer{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
		}

		_, _, err := d.Dial(badWsURL, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})
}

func TestDialProxyAuth(t *testing.T) {
	var receivedAuth string

	// Proxy that records Proxy-Authorization header.
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Proxy-Authorization")

		if r.Method != http.MethodConnect {
			http.Error(w, "expected CONNECT", http.StatusMethodNotAllowed)
			return
		}

		targetConn, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}

		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

		go func() {
			_, _ = io.Copy(targetConn, clientConn)
		}()
		_, _ = io.Copy(clientConn, targetConn)

		clientConn.Close()
		targetConn.Close()
	}))
	defer proxyServer.Close()

	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer wsServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	proxyURL.User = url.UserPassword("user", "pass")
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
	}

	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	conn.Close()

	assert.Equal(t, "Basic dXNlcjpwYXNz", receivedAuth)
}

func TestDialProxyNetDialContext(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer wsServer.Close()

	proxyServer := newTestCONNECTProxy(t)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	netDialCalled := false
	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
		NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			netDialCalled = true
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, addr)
		},
	}

	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	conn.Close()

	assert.True(t, netDialCalled)
}

func TestDialProxyWSSConnection(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	wsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(msgType, msg)
	}))
	defer wsServer.Close()

	proxyServer := newTestCONNECTProxy(t)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	wsURL := "wss" + strings.TrimPrefix(wsServer.URL, "https")

	serverTransport := wsServer.Client().Transport.(*http.Transport)

	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
		TLSClientConfig: serverTransport.TLSClientConfig,
	}

	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteMessage(TextMessage, []byte("wss-proxy"))
	require.NoError(t, err)

	msgType, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, TextMessage, msgType)
	assert.Equal(t, []byte("wss-proxy"), msg)
}

func TestDialProxyDefaultPort(t *testing.T) {
	var dialedAddr string
	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			proxyURL, _ := url.Parse("http://proxy.example.com")
			return proxyURL, nil
		},
		NetDialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
			dialedAddr = addr
			return nil, net.ErrClosed
		},
	}

	_, _, err := d.Dial("ws://target.example.com", nil)
	require.Error(t, err)
	assert.Equal(t, "proxy.example.com:80", dialedAddr)
}

func TestDialProxyDialFailure(t *testing.T) {
	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			proxyURL, _ := url.Parse("http://proxy.example.com:9999")
			return proxyURL, nil
		},
		NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return nil, net.ErrClosed
		},
	}

	_, _, err := d.Dial("ws://target.example.com", nil)
	require.Error(t, err)
}

func TestDoHandshakeCookieJar(t *testing.T) {
	var receivedCookie string

	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCookie = r.Header.Get("Cookie")

		// Pass Set-Cookie via responseHeader since Upgrade writes raw headers.
		respHeaders := http.Header{}
		respHeaders.Set("Set-Cookie", "resp-cookie=resp-value")

		conn, err := upgrader.Upgrade(w, r, respHeaders)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	jar := &testCookieJar{
		cookies: map[string][]*http.Cookie{
			serverURL.Host: {
				{Name: "test-cookie", Value: "test-value"},
			},
		},
	}

	d := &Dialer{Jar: jar}
	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	conn.Close()

	assert.Contains(t, receivedCookie, "test-cookie=test-value")
	assert.NotEmpty(t, jar.setCookies)
}

type testCookieJar struct {
	cookies    map[string][]*http.Cookie
	setCookies []*http.Cookie
}

func (j *testCookieJar) SetCookies(_ *url.URL, cookies []*http.Cookie) {
	j.setCookies = append(j.setCookies, cookies...)
}

func (j *testCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.cookies[u.Host]
}

func TestDialHTTP2CookieJar(t *testing.T) {
	server, client := net.Pipe()

	u, _ := url.Parse("https://example.com/ws")

	jar := &testCookieJar{
		cookies: map[string][]*http.Cookie{
			u.Host: {
				{Name: "h2-cookie", Value: "h2-value"},
			},
		},
	}

	h := make(http.Header)
	h.Set("Set-Cookie", "resp-h2=val; Path=/")

	var capturedCookie string
	d := &Dialer{Jar: jar}
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			capturedCookie = req.Header.Get("Cookie")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     h,
				Body:       server,
			}, nil
		}),
	}

	conn, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
	require.NoError(t, err)
	defer conn.Close()
	defer client.Close()

	assert.Contains(t, capturedCookie, "h2-cookie=h2-value")
	assert.NotEmpty(t, jar.setCookies)
}

func TestDialHTTP2UnrequestedSubprotocol(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	h := make(http.Header)
	h.Set("Sec-WebSocket-Protocol", "chat")

	d := &Dialer{} // No Subprotocols.
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     h,
				Body:       server,
			}, nil
		}),
	}

	u, _ := url.Parse("https://example.com/ws")
	_, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBadHandshake)
}

func TestDialHTTP2UnrequestedExtension(t *testing.T) {
	t.Run("Rejects extension when compression disabled", func(t *testing.T) {
		server, client := net.Pipe()
		defer client.Close()

		h := make(http.Header)
		h.Set("Sec-WebSocket-Extensions", "permessage-deflate")

		d := &Dialer{} // EnableCompression is false.
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     h,
					Body:       server,
				}, nil
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		_, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Rejects unknown extension with compression enabled", func(t *testing.T) {
		server, client := net.Pipe()
		defer client.Close()

		h := make(http.Header)
		h.Set("Sec-WebSocket-Extensions", "custom-ext")

		d := &Dialer{EnableCompression: true}
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     h,
					Body:       server,
				}, nil
			}),
		}

		u, _ := url.Parse("https://example.com/ws")
		_, _, err := d.dialHTTP2(context.Background(), httpClient, u, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})
}

func TestGetProxyURLErrors(t *testing.T) {
	t.Run("d.Proxy returns error", func(t *testing.T) {
		d := &Dialer{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return nil, errors.New("proxy error")
			},
		}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(u))
	})

	t.Run("d.Proxy returns nil URL", func(t *testing.T) {
		d := &Dialer{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return nil, nil
			},
		}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(u))
	})

	t.Run("transport.Proxy returns error", func(t *testing.T) {
		d := &Dialer{
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					Proxy: func(_ *http.Request) (*url.URL, error) {
						return nil, errors.New("transport proxy error")
					},
				},
			},
		}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(u))
	})

	t.Run("transport.Proxy returns nil URL", func(t *testing.T) {
		d := &Dialer{
			HTTPClient: &http.Client{
				Transport: &http.Transport{
					Proxy: func(_ *http.Request) (*url.URL, error) {
						return nil, nil
					},
				},
			},
		}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(u))
	})
}

func TestDialTLSHandshakeFailure(t *testing.T) {
	// Server that accepts TCP but is not TLS.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			time.Sleep(100 * time.Millisecond)
			conn.Close()
		}
	}()

	d := &Dialer{}
	_, _, err = d.Dial("wss://"+listener.Addr().String(), nil)
	require.Error(t, err)
}

func TestDialContextHTTP2Path(t *testing.T) {
	// This test verifies that DialContext dispatches to dialHTTP2 for HTTP/2 transport.
	// The connection will fail because http2.Transport can't connect, but the dispatch is hit.
	d := &Dialer{
		HTTPClient: &http.Client{
			Transport: &http2.Transport{},
		},
	}

	_, _, err := d.DialContext(context.Background(), "wss://127.0.0.1:1", nil)
	require.Error(t, err)
}

func TestDialProxyTransportDialContext(t *testing.T) {
	upgrader := &Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer wsServer.Close()

	proxyServer := newTestCONNECTProxy(t)
	defer proxyServer.Close()

	proxyURL, _ := url.Parse(proxyServer.URL)
	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	transportDialCalled := false
	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					transportDialCalled = true
					var dialer net.Dialer
					return dialer.DialContext(ctx, network, addr)
				},
			},
		},
	}

	conn, _, err := d.Dial(wsURL, nil)
	require.NoError(t, err)
	conn.Close()

	assert.True(t, transportDialCalled)
}

func TestDialWithProxyTransportDialContextForProxy(t *testing.T) {
	// Verify that transport.DialContext is used when NetDialContext is not set
	// and proxy connection uses the transport dial chain.
	transportDialCalled := false

	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			proxyURL, _ := url.Parse("http://proxy.example.com:9999")
			return proxyURL, nil
		},
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					transportDialCalled = true
					return nil, net.ErrClosed
				},
			},
		},
	}

	_, _, err := d.Dial("ws://example.com", nil)
	require.Error(t, err)
	assert.True(t, transportDialCalled)
}

// errMockSetDeadline is a sentinel error for testing SetDeadline failures.
var errMockSetDeadline = errors.New("mock: SetDeadline error")

// deadlineMockConn wraps net.Conn with configurable SetDeadline failure for testing.
type deadlineMockConn struct {
	net.Conn
	setDeadlineFailAt int // fail on Nth SetDeadline call (1-based); 0 = always pass through
	setDeadlineCalls  int
}

func (c *deadlineMockConn) SetDeadline(t time.Time) error {
	c.setDeadlineCalls++
	if c.setDeadlineFailAt > 0 && c.setDeadlineCalls >= c.setDeadlineFailAt {
		return errMockSetDeadline
	}
	return c.Conn.SetDeadline(t)
}

func TestDialDirectSetDeadlineErrors(t *testing.T) {
	t.Run("set deadline error before handshake", func(t *testing.T) {
		s, c := net.Pipe()
		defer s.Close()
		go func() { _, _ = io.Copy(io.Discard, s) }()

		d := &Dialer{
			HandshakeTimeout: time.Second,
			NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return &deadlineMockConn{Conn: c, setDeadlineFailAt: 1}, nil
			},
		}

		_, _, err := d.Dial("ws://example.com", nil)
		require.ErrorIs(t, err, errMockSetDeadline)
	})

	t.Run("clear deadline error after handshake", func(t *testing.T) {
		upgrader := &Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			conn.Close()
		}))
		defer server.Close()

		d := &Dialer{
			HandshakeTimeout: 5 * time.Second,
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				conn, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return &deadlineMockConn{Conn: conn, setDeadlineFailAt: 2}, nil
			},
		}

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		_, _, err := d.Dial(wsURL, nil)
		require.ErrorIs(t, err, errMockSetDeadline)
	})
}

func TestDialWithProxySetDeadlineErrors(t *testing.T) {
	t.Run("set deadline error before handshake", func(t *testing.T) {
		wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer wsServer.Close()

		proxyServer := newTestCONNECTProxy(t)
		defer proxyServer.Close()
		proxyURL, _ := url.Parse(proxyServer.URL)

		d := &Dialer{
			HandshakeTimeout: 5 * time.Second,
			Proxy:            func(_ *http.Request) (*url.URL, error) { return proxyURL, nil },
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				conn, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return &deadlineMockConn{Conn: conn, setDeadlineFailAt: 1}, nil
			},
		}

		wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")
		_, _, err := d.Dial(wsURL, nil)
		require.ErrorIs(t, err, errMockSetDeadline)
	})

	t.Run("clear deadline error after handshake", func(t *testing.T) {
		upgrader := &Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
		wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			conn.Close()
		}))
		defer wsServer.Close()

		proxyServer := newTestCONNECTProxy(t)
		defer proxyServer.Close()
		proxyURL, _ := url.Parse(proxyServer.URL)

		d := &Dialer{
			HandshakeTimeout: 5 * time.Second,
			Proxy:            func(_ *http.Request) (*url.URL, error) { return proxyURL, nil },
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var dialer net.Dialer
				conn, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return &deadlineMockConn{Conn: conn, setDeadlineFailAt: 2}, nil
			},
		}

		wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")
		_, _, err := d.Dial(wsURL, nil)
		require.ErrorIs(t, err, errMockSetDeadline)
	})
}

func TestDialProxyConnectWriteError(t *testing.T) {
	s, c := net.Pipe()
	s.Close()

	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return &url.URL{Host: "proxy.example.com:8080"}, nil
		},
		NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return c, nil
		},
	}

	_, _, err := d.Dial("ws://target.example.com", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestDialProxyReadResponseError(t *testing.T) {
	s, c := net.Pipe()
	go func() {
		br := bufio.NewReader(s)
		req, _ := http.ReadRequest(br)
		if req != nil {
			req.Body.Close()
		}
		s.Close()
	}()

	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return &url.URL{Host: "proxy.example.com:8080"}, nil
		},
		NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return c, nil
		},
	}

	_, _, err := d.Dial("ws://target.example.com", nil)
	require.Error(t, err)
}

func TestDialProxyTLSHandshakeError(t *testing.T) {
	s, c := net.Pipe()
	go func() {
		defer s.Close()
		br := bufio.NewReader(s)
		req, _ := http.ReadRequest(br)
		if req != nil {
			req.Body.Close()
		}
		_, _ = s.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		// Read TLS ClientHello attempt, then close to fail the handshake.
		buf := make([]byte, 4096)
		_, _ = s.Read(buf)
	}()

	d := &Dialer{
		Proxy: func(_ *http.Request) (*url.URL, error) {
			return &url.URL{Host: "proxy.example.com:8080"}, nil
		},
		NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return c, nil
		},
	}

	_, _, err := d.Dial("wss://target.example.com", nil)
	require.Error(t, err)
}

func TestDoHandshakeWriteError(t *testing.T) {
	s, c := net.Pipe()
	s.Close()

	d := &Dialer{
		NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return c, nil
		},
	}

	_, _, err := d.Dial("ws://example.com", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestGenerateChallengeKeyRandError(t *testing.T) {
	origRandReader := randReader
	testErr := errors.New("rand read error")
	randReader = &errReader{err: testErr}
	defer func() { randReader = origRandReader }()

	_, err := generateChallengeKey()
	require.ErrorIs(t, err, testErr)
}

func TestDoHandshakeRandError(t *testing.T) {
	s, c := net.Pipe()
	defer s.Close()
	go func() { _, _ = io.Copy(io.Discard, s) }()

	origRandReader := randReader
	testErr := errors.New("rand read error")
	randReader = &errReader{err: testErr}
	defer func() { randReader = origRandReader }()

	d := &Dialer{
		NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return c, nil
		},
	}

	_, _, err := d.Dial("ws://example.com", nil)
	require.ErrorIs(t, err, testErr)
}
