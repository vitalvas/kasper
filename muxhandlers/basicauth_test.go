package muxhandlers

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func basicAuthHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func TestBasicAuth(t *testing.T) {
	t.Run("config error no auth source", func(t *testing.T) {
		_, err := BasicAuthMiddleware(BasicAuthConfig{})
		assert.ErrorIs(t, err, ErrNoAuthSource)
	})

	tests := []struct {
		name        string
		config      BasicAuthConfig
		authHeader  string
		wantCode    int
		wantWWWAuth string
	}{
		{
			name:       "valid credentials via ValidateFunc",
			config:     BasicAuthConfig{ValidateFunc: func(u, p string) bool { return u == "admin" && p == "secret" }},
			authHeader: basicAuthHeader("admin", "secret"),
			wantCode:   http.StatusOK,
		},
		{
			name:       "valid credentials via Credentials map",
			config:     BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			authHeader: basicAuthHeader("admin", "secret"),
			wantCode:   http.StatusOK,
		},
		{
			name:       "invalid password",
			config:     BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			authHeader: basicAuthHeader("admin", "wrong"),
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "unknown username",
			config:     BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			authHeader: basicAuthHeader("unknown", "secret"),
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:     "missing Authorization header",
			config:   BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:       "malformed header not Basic",
			config:     BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			authHeader: "Bearer some-token",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "malformed base64",
			config:     BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			authHeader: "Basic !!!invalid-base64!!!",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "malformed credentials no colon",
			config:     BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			authHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")),
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "password with colons",
			config:     BasicAuthConfig{Credentials: map[string]string{"admin": "pass:with:colons"}},
			authHeader: basicAuthHeader("admin", "pass:with:colons"),
			wantCode:   http.StatusOK,
		},
		{
			name: "ValidateFunc takes priority over Credentials",
			config: BasicAuthConfig{
				ValidateFunc: func(u, p string) bool { return u == "func-user" && p == "func-pass" },
				Credentials:  map[string]string{"map-user": "map-pass"},
			},
			authHeader: basicAuthHeader("func-user", "func-pass"),
			wantCode:   http.StatusOK,
		},
		{
			name:        "custom realm",
			config:      BasicAuthConfig{Realm: "My App", Credentials: map[string]string{"admin": "secret"}},
			wantCode:    http.StatusUnauthorized,
			wantWWWAuth: `Basic realm="My App"`,
		},
		{
			name:        "default realm",
			config:      BasicAuthConfig{Credentials: map[string]string{"admin": "secret"}},
			wantCode:    http.StatusUnauthorized,
			wantWWWAuth: `Basic realm="Restricted"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mux.NewRouter()
			r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}).Methods(http.MethodGet)

			mw, err := BasicAuthMiddleware(tt.config)
			require.NoError(t, err)
			r.Use(mw)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			if tt.wantWWWAuth != "" {
				assert.Equal(t, tt.wantWWWAuth, w.Header().Get("WWW-Authenticate"))
			}
			if tt.wantCode == http.StatusUnauthorized {
				assert.NotEmpty(t, w.Header().Get("WWW-Authenticate"))
				body, err := io.ReadAll(w.Body)
				require.NoError(t, err)
				assert.Empty(t, body)
			}
		})
	}
}

func BenchmarkBasicAuth(b *testing.B) {
	b.Run("valid credentials", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BasicAuthMiddleware(BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("invalid credentials", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BasicAuthMiddleware(BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("admin", "wrong"))

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
