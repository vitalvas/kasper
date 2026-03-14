package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestIPAllowMiddleware(t *testing.T) {
	t.Run("allowed single IP", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"192.168.1.1"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("denied single IP", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"192.168.1.1"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.5:12345"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("allowed CIDR range", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"10.0.0.0/8"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.50.100.200:8080"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("denied outside CIDR range", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"10.0.0.0/8"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "172.16.0.1:8080"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("multiple allowed entries", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"192.168.1.1", "10.0.0.0/8", "::1"},
		})
		require.NoError(t, err)
		r.Use(mw)

		tests := []struct {
			name       string
			remoteAddr string
			wantCode   int
		}{
			{"exact IP match", "192.168.1.1:1234", http.StatusOK},
			{"CIDR match", "10.5.5.5:1234", http.StatusOK},
			{"IPv6 loopback", "[::1]:1234", http.StatusOK},
			{"not in list", "172.16.0.1:1234", http.StatusForbidden},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.RemoteAddr = tt.remoteAddr
				r.ServeHTTP(w, req)

				assert.Equal(t, tt.wantCode, w.Code)
			})
		}
	})

	t.Run("custom denied handler", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"192.168.1.1"},
			DeniedHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"access denied"}`))
			}),
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.5:12345"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Equal(t, `{"error":"access denied"}`, w.Body.String())
	})

	t.Run("bare IP without port", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"192.168.1.1"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("unparseable remote addr denied", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"192.168.1.1"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "not-an-ip"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("empty allowed list returns error", func(t *testing.T) {
		_, err := IPAllowMiddleware(IPAllowConfig{})
		assert.ErrorIs(t, err, ErrIPAllowEmpty)
	})

	t.Run("invalid IP returns error", func(t *testing.T) {
		_, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"not-valid"},
		})
		assert.ErrorIs(t, err, ErrIPAllowInvalidEntry)
	})

	t.Run("invalid CIDR returns error", func(t *testing.T) {
		_, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"999.999.999.999/32"},
		})
		assert.ErrorIs(t, err, ErrIPAllowInvalidEntry)
	})

	t.Run("IPv6 CIDR range", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := IPAllowMiddleware(IPAllowConfig{
			Allowed: []string{"fd00::/8"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "[fd12:3456:789a::1]:8080"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func BenchmarkIPAllowMiddleware(b *testing.B) {
	r := mux.NewRouter()
	r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	mw, err := IPAllowMiddleware(IPAllowConfig{
		Allowed: []string{"10.0.0.0/8", "192.168.0.0/16", "::1"},
	})
	if err != nil {
		b.Fatal(err)
	}
	r.Use(mw)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:8080"

	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}
