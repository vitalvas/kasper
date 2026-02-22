package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestServerMiddleware(t *testing.T) {
	t.Run("default os hostname", func(t *testing.T) {
		expected, err := os.Hostname()
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ServerMiddleware(ServerConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, expected, w.Header().Get("X-Server-Hostname"))
	})

	t.Run("custom hostname", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ServerMiddleware(ServerConfig{
			Hostname: "web-01",
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "web-01", w.Header().Get("X-Server-Hostname"))
	})

	t.Run("hostname from environment variable", func(t *testing.T) {
		t.Setenv("TEST_POD_NAME", "pod-abc-123")

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ServerMiddleware(ServerConfig{
			HostnameEnv: []string{"TEST_POD_NAME"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "pod-abc-123", w.Header().Get("X-Server-Hostname"))
	})

	t.Run("env list first non-empty wins", func(t *testing.T) {
		t.Setenv("TEST_UNSET_VAR", "")
		t.Setenv("TEST_POD_NAME_2", "pod-xyz-789")

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ServerMiddleware(ServerConfig{
			HostnameEnv: []string{"TEST_UNSET_VAR", "TEST_POD_NAME_2"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "pod-xyz-789", w.Header().Get("X-Server-Hostname"))
	})

	t.Run("all empty envs fall back to os hostname", func(t *testing.T) {
		t.Setenv("TEST_EMPTY_A", "")
		t.Setenv("TEST_EMPTY_B", "")

		expected, err := os.Hostname()
		require.NoError(t, err)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ServerMiddleware(ServerConfig{
			HostnameEnv: []string{"TEST_EMPTY_A", "TEST_EMPTY_B"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, expected, w.Header().Get("X-Server-Hostname"))
	})

	t.Run("Hostname field takes priority over env", func(t *testing.T) {
		t.Setenv("TEST_POD_NAME_PRIO", "from-env")

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := ServerMiddleware(ServerConfig{
			Hostname:    "from-field",
			HostnameEnv: []string{"TEST_POD_NAME_PRIO"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, "from-field", w.Header().Get("X-Server-Hostname"))
	})

	t.Run("header set on every response", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet, http.MethodPost)

		mw, err := ServerMiddleware(ServerConfig{
			Hostname: "web-01",
		})
		require.NoError(t, err)
		r.Use(mw)

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, "web-01", w.Header().Get("X-Server-Hostname"))
		}
	})
}

func BenchmarkServerMiddleware(b *testing.B) {
	r := mux.NewRouter()
	r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	mw, err := ServerMiddleware(ServerConfig{
		Hostname: "bench-host",
	})
	if err != nil {
		b.Fatal(err)
	}
	r.Use(mw)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}
