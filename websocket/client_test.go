package websocket

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		d := &Dialer{
			NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := d.DialContext(ctx, "ws://example.com", nil)
		require.Error(t, err)
	})
}

func TestDialerDial(t *testing.T) {
	t.Run("Custom NetDial", func(t *testing.T) {
		called := false
		d := &Dialer{
			NetDial: func(_, _ string) (net.Conn, error) {
				called = true
				return nil, net.ErrClosed
			},
		}

		_, _, _ = d.Dial("ws://example.com", nil)
		assert.True(t, called)
	})

	t.Run("Custom NetDialContext", func(t *testing.T) {
		called := false
		d := &Dialer{
			NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				called = true
				return nil, net.ErrClosed
			},
		}

		_, _, _ = d.Dial("ws://example.com", nil)
		assert.True(t, called)
	})
}

func TestDialerTLS(t *testing.T) {
	t.Run("Custom NetDialTLSContext", func(t *testing.T) {
		called := false
		d := &Dialer{
			NetDialTLSContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				called = true
				return nil, net.ErrClosed
			},
		}

		_, _, _ = d.Dial("wss://example.com", nil)
		assert.True(t, called)
	})

	t.Run("Custom TLSClientConfig", func(t *testing.T) {
		d := &Dialer{
			TLSClientConfig: &tls.Config{
				ServerName: "custom.example.com",
			},
			NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return nil, net.ErrClosed
			},
		}

		_, _, err := d.Dial("wss://example.com", nil)
		require.Error(t, err)
	})
}

func TestGenerateChallengeKey(t *testing.T) {
	key1 := generateChallengeKey()
	key2 := generateChallengeKey()

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

		d := &Dialer{
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

func TestDialerDefaultPort(t *testing.T) {
	t.Run("WS default port 80", func(t *testing.T) {
		var dialedAddr string
		d := &Dialer{
			NetDialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
				dialedAddr = addr
				return nil, net.ErrClosed
			},
		}

		_, _, _ = d.Dial("ws://example.com/path", nil)
		assert.Equal(t, "example.com:80", dialedAddr)
	})

	t.Run("WSS default port 443", func(t *testing.T) {
		var dialedAddr string
		d := &Dialer{
			NetDialTLSContext: func(_ context.Context, _, addr string) (net.Conn, error) {
				dialedAddr = addr
				return nil, net.ErrClosed
			},
		}

		_, _, _ = d.Dial("wss://example.com/path", nil)
		assert.Equal(t, "example.com:443", dialedAddr)
	})

	t.Run("Custom port preserved", func(t *testing.T) {
		var dialedAddr string
		d := &Dialer{
			NetDialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
				dialedAddr = addr
				return nil, net.ErrClosed
			},
		}

		_, _, _ = d.Dial("ws://example.com:8080/path", nil)
		assert.Equal(t, "example.com:8080", dialedAddr)
	})
}

func TestDialerTLSWithConfig(t *testing.T) {
	t.Run("TLS config with ServerName", func(t *testing.T) {
		d := &Dialer{
			TLSClientConfig: &tls.Config{
				ServerName:         "custom.example.com",
				InsecureSkipVerify: true,
			},
			NetDialTLSContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return nil, net.ErrClosed
			},
		}

		_, _, err := d.Dial("wss://example.com/path", nil)
		assert.Error(t, err)
	})

	t.Run("TLS with nil config uses default", func(t *testing.T) {
		d := &Dialer{
			TLSClientConfig: nil,
			NetDialTLSContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return nil, net.ErrClosed
			},
		}

		_, _, err := d.Dial("wss://example.com/path", nil)
		assert.Error(t, err)
	})
}

func TestDialerContextTimeout(t *testing.T) {
	t.Run("Context timeout during dial", func(t *testing.T) {
		d := &Dialer{
			NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
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

	t.Run("Connect to TLS server", func(t *testing.T) {
		d := &Dialer{
			TLSClientConfig: server.Client().Transport.(*http.Transport).TLSClientConfig,
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
}

func TestDialerSetDeadlineError(t *testing.T) {
	t.Run("Deadline after successful dial", func(t *testing.T) {
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

		d := &Dialer{
			HandshakeTimeout: 5 * time.Second,
		}

		conn, _, err := d.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()
	})
}
