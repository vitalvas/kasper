package muxhandlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestTimeoutMiddleware(t *testing.T) {
	t.Run("config validation", func(t *testing.T) {
		tests := []struct {
			name    string
			config  TimeoutConfig
			wantErr error
		}{
			{"zero duration", TimeoutConfig{Duration: 0}, ErrInvalidTimeout},
			{"negative duration", TimeoutConfig{Duration: -1 * time.Second}, ErrInvalidTimeout},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := TimeoutMiddleware(tt.config)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}

		t.Run("valid duration", func(t *testing.T) {
			_, err := TimeoutMiddleware(TimeoutConfig{Duration: time.Second})
			assert.NoError(t, err)
		})
	})

	t.Run("handler completes before timeout", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		}).Methods(http.MethodGet)

		mw, err := TimeoutMiddleware(TimeoutConfig{Duration: 2 * time.Second})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		body, err := io.ReadAll(w.Body)
		require.NoError(t, err)
		assert.Equal(t, "ok", string(body))
	})

	t.Run("handler exceeds timeout", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			select {
			case <-time.After(5 * time.Second):
				w.WriteHeader(http.StatusOK)
			case <-req.Context().Done():
				return
			}
		}).Methods(http.MethodGet)

		mw, err := TimeoutMiddleware(TimeoutConfig{Duration: 50 * time.Millisecond})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("custom timeout message", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			select {
			case <-time.After(5 * time.Second):
				w.WriteHeader(http.StatusOK)
			case <-req.Context().Done():
				return
			}
		}).Methods(http.MethodGet)

		mw, err := TimeoutMiddleware(TimeoutConfig{
			Duration: 50 * time.Millisecond,
			Message:  "request timed out",
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		body, err := io.ReadAll(w.Body)
		require.NoError(t, err)
		assert.Equal(t, "request timed out", string(body))
	})

	t.Run("empty message uses stdlib default", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			select {
			case <-time.After(5 * time.Second):
				w.WriteHeader(http.StatusOK)
			case <-req.Context().Done():
				return
			}
		}).Methods(http.MethodGet)

		mw, err := TimeoutMiddleware(TimeoutConfig{Duration: 50 * time.Millisecond})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func BenchmarkTimeoutMiddleware(b *testing.B) {
	b.Run("completes before timeout", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := TimeoutMiddleware(TimeoutConfig{Duration: 5 * time.Second})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
