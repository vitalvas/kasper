package muxhandlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestRequestSizeLimitMiddleware(t *testing.T) {
	t.Run("config validation", func(t *testing.T) {
		tests := []struct {
			name    string
			config  RequestSizeLimitConfig
			wantErr error
		}{
			{"zero max bytes", RequestSizeLimitConfig{MaxBytes: 0}, ErrInvalidMaxSize},
			{"negative max bytes", RequestSizeLimitConfig{MaxBytes: -1}, ErrInvalidMaxSize},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := RequestSizeLimitMiddleware(tt.config)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}

		t.Run("valid max bytes", func(t *testing.T) {
			_, err := RequestSizeLimitMiddleware(RequestSizeLimitConfig{MaxBytes: 1024})
			assert.NoError(t, err)
		})
	})

	tests := []struct {
		name            string
		maxBytes        int64
		body            string
		wantCode        int
		wantHandlerCall bool
		wantReadErr     bool
	}{
		{
			name:            "body within limit",
			maxBytes:        1024,
			body:            "hello",
			wantCode:        http.StatusOK,
			wantHandlerCall: true,
		},
		{
			name:            "body exactly at limit",
			maxBytes:        5,
			body:            "hello",
			wantCode:        http.StatusOK,
			wantHandlerCall: true,
		},
		{
			name:            "body exceeds limit",
			maxBytes:        3,
			body:            "hello world",
			wantCode:        http.StatusRequestEntityTooLarge,
			wantHandlerCall: true,
			wantReadErr:     true,
		},
		{
			name:            "empty body",
			maxBytes:        1024,
			body:            "",
			wantCode:        http.StatusOK,
			wantHandlerCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handlerCalled bool

			r := mux.NewRouter()
			r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
				handlerCalled = true
				_, err := io.ReadAll(req.Body)
				if err != nil {
					http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
					return
				}
				w.WriteHeader(http.StatusOK)
			}).Methods(http.MethodPost)

			mw, err := RequestSizeLimitMiddleware(RequestSizeLimitConfig{MaxBytes: tt.maxBytes})
			require.NoError(t, err)
			r.Use(mw)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantHandlerCall, handlerCalled)
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}

	t.Run("GET request without body passes through", func(t *testing.T) {
		var handlerCalled bool

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := RequestSizeLimitMiddleware(RequestSizeLimitConfig{MaxBytes: 1024})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.True(t, handlerCalled)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func BenchmarkRequestSizeLimitMiddleware(b *testing.B) {
	b.Run("body within limit", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			io.Copy(io.Discard, req.Body)
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodPost)

		mw, err := RequestSizeLimitMiddleware(RequestSizeLimitConfig{MaxBytes: 1024})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		body := strings.NewReader("hello")

		b.ResetTimer()
		for b.Loop() {
			body.Reset("hello")
			req := httptest.NewRequest(http.MethodPost, "/test", body)
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("body exceeds limit", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			io.Copy(io.Discard, req.Body)
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodPost)

		mw, err := RequestSizeLimitMiddleware(RequestSizeLimitConfig{MaxBytes: 3})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		body := strings.NewReader("hello world this is too long")

		b.ResetTimer()
		for b.Loop() {
			body.Reset("hello world this is too long")
			req := httptest.NewRequest(http.MethodPost, "/test", body)
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
