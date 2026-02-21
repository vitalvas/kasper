package muxhandlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestRecoveryMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		logFunc       func(r *http.Request, err any)
		wantCode      int
		wantPanic     bool
		wantLogCalled bool
	}{
		{
			name: "no panic passes through",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "panic returns 500",
			handler: func(_ http.ResponseWriter, _ *http.Request) {
				panic("something went wrong")
			},
			wantCode:  http.StatusInternalServerError,
			wantPanic: true,
		},
		{
			name: "panic with LogFunc calls logger",
			handler: func(_ http.ResponseWriter, _ *http.Request) {
				panic("log this")
			},
			logFunc:       func(_ *http.Request, _ any) {},
			wantCode:      http.StatusInternalServerError,
			wantPanic:     true,
			wantLogCalled: true,
		},
		{
			name: "panic with integer value",
			handler: func(_ http.ResponseWriter, _ *http.Request) {
				panic(42)
			},
			wantCode:  http.StatusInternalServerError,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logCalled bool
			var loggedErr any

			cfg := RecoveryConfig{}
			if tt.logFunc != nil {
				cfg.LogFunc = func(r *http.Request, err any) {
					logCalled = true
					loggedErr = err
					tt.logFunc(r, err)
				}
			}

			r := mux.NewRouter()
			r.HandleFunc("/test", tt.handler).Methods(http.MethodGet)
			r.Use(RecoveryMiddleware(cfg))

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)

			if tt.wantLogCalled {
				assert.True(t, logCalled)
				assert.NotNil(t, loggedErr)
			}

			if tt.wantPanic {
				body, err := io.ReadAll(w.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), http.StatusText(http.StatusInternalServerError))
			}
		})
	}

	t.Run("LogFunc receives correct panic value", func(t *testing.T) {
		var loggedValue any

		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			panic("expected-value")
		}).Methods(http.MethodGet)
		r.Use(RecoveryMiddleware(RecoveryConfig{
			LogFunc: func(_ *http.Request, err any) {
				loggedValue = err
			},
		}))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "expected-value", loggedValue)
	})
}

func BenchmarkRecoveryMiddleware(b *testing.B) {
	b.Run("no panic", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(RecoveryMiddleware(RecoveryConfig{}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("panic recovery", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			panic("bench")
		}).Methods(http.MethodGet)
		r.Use(RecoveryMiddleware(RecoveryConfig{}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
