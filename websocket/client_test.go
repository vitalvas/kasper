package websocket

import (
	"context"
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

		// Use custom dial to trigger dialWithTransport path
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
	t.Run("Non-101 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		d := &Dialer{}

		_, resp, err := d.Dial(wsURL, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
		assert.NotNil(t, resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("Wrong Upgrade header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Upgrade", "http/2.0")
			w.Header().Set("Connection", "upgrade")
			w.WriteHeader(http.StatusSwitchingProtocols)
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		d := &Dialer{}

		_, _, err := d.Dial(wsURL, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong Connection header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Upgrade", "websocket")
			w.Header().Set("Connection", "close")
			w.WriteHeader(http.StatusSwitchingProtocols)
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		d := &Dialer{}

		_, _, err := d.Dial(wsURL, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})

	t.Run("Wrong Sec-WebSocket-Accept", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Upgrade", "websocket")
			w.Header().Set("Connection", "upgrade")
			w.Header().Set("Sec-WebSocket-Accept", "wrong-accept-key")
			w.WriteHeader(http.StatusSwitchingProtocols)
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		d := &Dialer{}

		_, _, err := d.Dial(wsURL, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBadHandshake)
	})
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
	t.Run("Nil transport is not HTTP/2", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{}
		assert.False(t, d.isHTTP2(client))
	})

	t.Run("http.Transport is not HTTP/2", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{Transport: &http.Transport{}}
		assert.False(t, d.isHTTP2(client))
	})

	t.Run("http2.Transport is HTTP/2", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{Transport: &http2.Transport{}}
		assert.True(t, d.isHTTP2(client))
	})
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
	t.Run("No proxy when transport is nil", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(client, u))
	})

	t.Run("No proxy when Proxy function is nil", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{Transport: &http.Transport{}}
		u, _ := url.Parse("http://example.com")
		assert.Nil(t, d.getProxyURL(client, u))
	})

	t.Run("Returns proxy URL when configured", func(t *testing.T) {
		proxyURL, _ := url.Parse("http://proxy.example.com:8080")
		d := &Dialer{}
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: func(_ *http.Request) (*url.URL, error) {
					return proxyURL, nil
				},
			},
		}
		u, _ := url.Parse("http://example.com")
		assert.Equal(t, proxyURL, d.getProxyURL(client, u))
	})
}

func TestDialerHasCustomDial(t *testing.T) {
	t.Run("No custom dial when transport is nil", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{}
		assert.False(t, d.hasCustomDial(client))
	})

	t.Run("No custom dial with default transport", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{Transport: &http.Transport{}}
		assert.False(t, d.hasCustomDial(client))
	})

	t.Run("Has custom dial with DialContext", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return nil, nil
				},
			},
		}
		assert.True(t, d.hasCustomDial(client))
	})

	t.Run("Has custom dial with DialTLSContext", func(t *testing.T) {
		d := &Dialer{}
		client := &http.Client{
			Transport: &http.Transport{
				DialTLSContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return nil, nil
				},
			},
		}
		assert.True(t, d.hasCustomDial(client))
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
	t.Run("WS default port 80", func(t *testing.T) {
		var dialedAddr string
		transport := &http.Transport{
			DialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
				dialedAddr = addr
				return nil, net.ErrClosed
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		_, _, _ = d.Dial("ws://example.com/path", nil)
		assert.Equal(t, "example.com:80", dialedAddr)
	})

	t.Run("WSS default port 443", func(t *testing.T) {
		var dialedAddr string
		transport := &http.Transport{
			DialTLSContext: func(_ context.Context, _, addr string) (net.Conn, error) {
				dialedAddr = addr
				return nil, net.ErrClosed
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		_, _, _ = d.Dial("wss://example.com/path", nil)
		assert.Equal(t, "example.com:443", dialedAddr)
	})

	t.Run("Custom port preserved", func(t *testing.T) {
		var dialedAddr string
		transport := &http.Transport{
			DialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
				dialedAddr = addr
				return nil, net.ErrClosed
			},
		}

		d := &Dialer{
			HTTPClient: &http.Client{Transport: transport},
		}

		_, _, _ = d.Dial("ws://example.com:8080/path", nil)
		assert.Equal(t, "example.com:8080", dialedAddr)
	})
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

	// Start a proxy server.
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "expected CONNECT", http.StatusMethodNotAllowed)
			return
		}

		// Connect to target.
		targetConn, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		// Hijack the connection.
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

		// Send 200 OK.
		_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

		// Relay data.
		go func() {
			_, _ = io.Copy(targetConn, clientConn)
		}()
		_, _ = io.Copy(clientConn, targetConn)

		clientConn.Close()
		targetConn.Close()
	}))
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

	// Client with default http.Client has nil UnderlyingConn.
	// This is expected since we use http.Client.Do() path.
	conn.WriteMessage(TextMessage, []byte("test"))
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
