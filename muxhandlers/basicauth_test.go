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

func newBasicAuthRouter(t *testing.T, cfg BasicAuthConfig) *mux.Router {
	t.Helper()

	r := mux.NewRouter()
	r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	mw, err := BasicAuthMiddleware(cfg)
	require.NoError(t, err)

	r.Use(mw)

	return r
}

func TestBasicAuth(t *testing.T) {
	t.Run("valid credentials via ValidateFunc", func(t *testing.T) {
		handlerCalled := false
		r := mux.NewRouter()
		r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BasicAuthMiddleware(BasicAuthConfig{
			ValidateFunc: func(username, password string) bool {
				return username == "admin" && password == "secret"
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)
	})

	t.Run("valid credentials via Credentials map", func(t *testing.T) {
		handlerCalled := false
		r := mux.NewRouter()
		r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BasicAuthMiddleware(BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)
	})

	t.Run("invalid password", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("admin", "wrong"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.NotEmpty(t, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("unknown username", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("unknown", "secret"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("missing Authorization header", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.NotEmpty(t, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("malformed header not Basic", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer some-token")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("malformed base64", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Basic !!!invalid-base64!!!")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("malformed credentials no colon", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		encoded := base64.StdEncoding.EncodeToString([]byte("nocolon"))
		req.Header.Set("Authorization", "Basic "+encoded)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("password with colons", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "pass:with:colons"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("admin", "pass:with:colons"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("ValidateFunc takes priority over Credentials", func(t *testing.T) {
		callbackCalled := false
		r := mux.NewRouter()
		r.HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := BasicAuthMiddleware(BasicAuthConfig{
			ValidateFunc: func(username, password string) bool {
				callbackCalled = true
				return username == "func-user" && password == "func-pass"
			},
			Credentials: map[string]string{"map-user": "map-pass"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", basicAuthHeader("func-user", "func-pass"))
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, callbackCalled)
	})

	t.Run("custom realm", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Realm:       "My App",
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, `Basic realm="My App"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("default realm", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, `Basic realm="Restricted"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("config error no auth source", func(t *testing.T) {
		_, err := BasicAuthMiddleware(BasicAuthConfig{})
		assert.ErrorIs(t, err, ErrNoAuthSource)
	})

	t.Run("empty body on 401", func(t *testing.T) {
		r := newBasicAuthRouter(t, BasicAuthConfig{
			Credentials: map[string]string{"admin": "secret"},
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		body, err := io.ReadAll(w.Body)
		require.NoError(t, err)
		assert.Empty(t, body)
	})
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
