package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestMethodOverrideMiddleware(t *testing.T) {
	t.Run("config validation", func(t *testing.T) {
		tests := []struct {
			name    string
			config  MethodOverrideConfig
			wantErr error
		}{
			{
				"empty string in allowed methods",
				MethodOverrideConfig{
					AllowedMethods: []string{""},
				},
				ErrInvalidOverrideMethod,
			},
			{
				"lowercase allowed method",
				MethodOverrideConfig{
					AllowedMethods: []string{"put"},
				},
				ErrInvalidOverrideMethod,
			},
			{
				"mixed case allowed method",
				MethodOverrideConfig{
					AllowedMethods: []string{"Put"},
				},
				ErrInvalidOverrideMethod,
			},
			{
				"lowercase original method",
				MethodOverrideConfig{
					OriginalMethods: []string{"post"},
				},
				ErrInvalidOverrideMethod,
			},
			{
				"empty string in original methods",
				MethodOverrideConfig{
					OriginalMethods: []string{""},
				},
				ErrInvalidOverrideMethod,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := MethodOverrideMiddleware(tt.config)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}

		t.Run("zero value config uses defaults", func(t *testing.T) {
			_, err := MethodOverrideMiddleware(MethodOverrideConfig{})
			assert.NoError(t, err)
		})

		t.Run("valid config with custom methods", func(t *testing.T) {
			_, err := MethodOverrideMiddleware(MethodOverrideConfig{
				AllowedMethods: []string{http.MethodPut, http.MethodDelete},
			})
			assert.NoError(t, err)
		})
	})

	t.Run("default headers", func(t *testing.T) {
		tests := []struct {
			name   string
			header string
		}{
			{"X-HTTP-Method-Override", "X-HTTP-Method-Override"},
			{"X-Method-Override", "X-Method-Override"},
			{"X-HTTP-Method", "X-HTTP-Method"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var gotMethod string

				r := mux.NewRouter()
				r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
					gotMethod = req.Method
					w.WriteHeader(http.StatusOK)
				})

				mw, err := MethodOverrideMiddleware(MethodOverrideConfig{})
				require.NoError(t, err)
				r.Use(mw)

				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/test", nil)
				req.Header.Set(tt.header, "DELETE")
				r.ServeHTTP(w, req)

				assert.Equal(t, http.MethodDelete, gotMethod)
			})
		}

		t.Run("priority order first wins", func(t *testing.T) {
			var gotMethod string

			r := mux.NewRouter()
			r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
				gotMethod = req.Method
				w.WriteHeader(http.StatusOK)
			})

			mw, err := MethodOverrideMiddleware(MethodOverrideConfig{})
			require.NoError(t, err)
			r.Use(mw)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("X-HTTP-Method-Override", "PUT")
			req.Header.Set("X-HTTP-Method", "DELETE")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.MethodPut, gotMethod)
		})
	})

	t.Run("POST with override header changes method", func(t *testing.T) {
		var gotMethod string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			gotMethod = req.Method
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-HTTP-Method-Override"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("X-HTTP-Method-Override", "PUT")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.MethodPut, gotMethod)
	})

	t.Run("POST with non-allowed override leaves method unchanged", func(t *testing.T) {
		var gotMethod string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			gotMethod = req.Method
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames:    []string{"X-HTTP-Method-Override"},
			AllowedMethods: []string{http.MethodPut},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("X-HTTP-Method-Override", "DELETE")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.MethodPost, gotMethod)
	})

	t.Run("GET with override header is ignored by default", func(t *testing.T) {
		var gotMethod string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			gotMethod = req.Method
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-HTTP-Method-Override"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-HTTP-Method-Override", "DELETE")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.MethodGet, gotMethod)
	})

	t.Run("custom original methods", func(t *testing.T) {
		var gotMethod string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			gotMethod = req.Method
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames:     []string{"X-HTTP-Method-Override"},
			OriginalMethods: []string{http.MethodPost, http.MethodGet},
		})
		require.NoError(t, err)
		r.Use(mw)

		t.Run("GET override applies", func(t *testing.T) {
			gotMethod = ""

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-HTTP-Method-Override", "DELETE")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.MethodDelete, gotMethod)
		})

		t.Run("POST override still applies", func(t *testing.T) {
			gotMethod = ""

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("X-HTTP-Method-Override", "PUT")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.MethodPut, gotMethod)
		})

		t.Run("PUT not in original methods is ignored", func(t *testing.T) {
			gotMethod = ""

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/test", nil)
			req.Header.Set("X-HTTP-Method-Override", "DELETE")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.MethodPut, gotMethod)
		})
	})

	t.Run("multiple headers first non-empty wins", func(t *testing.T) {
		var gotMethod string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			gotMethod = req.Method
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-Method-First", "X-HTTP-Method-Override"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("X-HTTP-Method-Override", "DELETE")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.MethodDelete, gotMethod)
	})

	t.Run("header removed after override", func(t *testing.T) {
		var headerValue string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			headerValue = req.Header.Get("X-HTTP-Method-Override")
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-HTTP-Method-Override"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("X-HTTP-Method-Override", "PUT")
		r.ServeHTTP(w, req)

		assert.Empty(t, headerValue)
	})

	t.Run("case insensitivity of override value", func(t *testing.T) {
		var gotMethod string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			gotMethod = req.Method
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-HTTP-Method-Override"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("X-HTTP-Method-Override", "patch")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.MethodPatch, gotMethod)
	})

	t.Run("POST without override header passes through", func(t *testing.T) {
		var gotMethod string

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			gotMethod = req.Method
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-HTTP-Method-Override"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.MethodPost, gotMethod)
	})
}

func BenchmarkMethodOverrideMiddleware(b *testing.B) {
	b.Run("with override", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-HTTP-Method-Override"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		b.ResetTimer()
		for b.Loop() {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("X-HTTP-Method-Override", "PUT")
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("without override", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mw, err := MethodOverrideMiddleware(MethodOverrideConfig{
			HeaderNames: []string{"X-HTTP-Method-Override"},
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
	})
}
