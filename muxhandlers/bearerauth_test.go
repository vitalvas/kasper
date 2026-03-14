package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestBearerAuth(t *testing.T) {
	t.Run("config error no validator", func(t *testing.T) {
		_, err := BearerAuthMiddleware(BearerAuthConfig{})
		assert.ErrorIs(t, err, ErrNoTokenValidator)
	})

	tests := []struct {
		name        string
		config      BearerAuthConfig
		authHeader  string
		wantCode    int
		wantWWWAuth string
	}{
		{
			name: "valid token",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, token string) bool {
					return token == "valid-token"
				},
			},
			authHeader: "Bearer valid-token",
			wantCode:   http.StatusOK,
		},
		{
			name: "invalid token",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, token string) bool {
					return token == "valid-token"
				},
			},
			authHeader: "Bearer wrong-token",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name: "missing Authorization header",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, _ string) bool {
					return true
				},
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "Basic auth header instead of Bearer",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, _ string) bool {
					return true
				},
			},
			authHeader: "Basic dXNlcjpwYXNz",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name: "Bearer prefix only without token",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, _ string) bool {
					return true
				},
			},
			authHeader: "Bearer ",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name: "bearer lowercase prefix",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, token string) bool {
					return token == "valid-token"
				},
			},
			authHeader: "bearer valid-token",
			wantCode:   http.StatusOK,
		},
		{
			name: "BEARER uppercase prefix",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, token string) bool {
					return token == "valid-token"
				},
			},
			authHeader: "BEARER valid-token",
			wantCode:   http.StatusOK,
		},
		{
			name: "custom realm",
			config: BearerAuthConfig{
				Realm: "My API",
				ValidateFunc: func(_ *http.Request, _ string) bool {
					return false
				},
			},
			authHeader:  "Bearer some-token",
			wantCode:    http.StatusUnauthorized,
			wantWWWAuth: `Bearer realm="My API"`,
		},
		{
			name: "default realm",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, _ string) bool {
					return false
				},
			},
			authHeader:  "Bearer some-token",
			wantCode:    http.StatusUnauthorized,
			wantWWWAuth: `Bearer realm="Restricted"`,
		},
		{
			name: "token with special characters",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, token string) bool {
					return token == "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123"
				},
			},
			authHeader: "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123",
			wantCode:   http.StatusOK,
		},
		{
			name: "empty Authorization header",
			config: BearerAuthConfig{
				ValidateFunc: func(_ *http.Request, _ string) bool {
					return true
				},
			},
			authHeader: "",
			wantCode:   http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw, err := BearerAuthMiddleware(tt.config)
			require.NoError(t, err)

			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)

			wwwAuth := w.Header().Get("WWW-Authenticate")
			if tt.wantWWWAuth != "" {
				assert.Equal(t, tt.wantWWWAuth, wwwAuth)
			}
			if tt.wantCode == http.StatusUnauthorized {
				assert.NotEmpty(t, wwwAuth)
				assert.Empty(t, w.Body.Bytes())
			}
		})
	}
}

func TestBearerAuthValidateFuncReceivesRequest(t *testing.T) {
	t.Run("ValidateFunc receives the original request", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BearerAuthMiddleware(BearerAuthConfig{
			ValidateFunc: func(r *http.Request, token string) bool {
				return token == "valid" && r.URL.Path == "/users/42"
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		req.Header.Set("Authorization", "Bearer valid")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func BenchmarkBearerAuth(b *testing.B) {
	b.Run("valid token", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BearerAuthMiddleware(BearerAuthConfig{
			ValidateFunc: func(_ *http.Request, token string) bool {
				return token == "valid-token"
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer valid-token")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("invalid token", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BearerAuthMiddleware(BearerAuthConfig{
			ValidateFunc: func(_ *http.Request, token string) bool {
				return token == "valid-token"
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
