package muxhandlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func newTestRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	}).Methods(http.MethodGet, http.MethodPost)

	return r
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("allows matching origin", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("blocks non-matching origin", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://evil.com")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("allows wildcard origin", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"*"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://any.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("allows origin via AllowOriginFunc", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowOriginFunc: func(origin string) bool {
				return strings.HasSuffix(origin, ".example.com")
			},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://sub.example.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "https://sub.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("skips CORS headers when no Origin header", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"*"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("reflects specific origin with multiple allowed", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://a.com", "https://b.com"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://b.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "https://b.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("sets Allow-Credentials when configured", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins:   []string{"https://example.com"},
			AllowCredentials: true,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("does not set Allow-Credentials when not configured", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("handles preflight with 204 and stops chain", func(t *testing.T) {
		r := mux.NewRouter()
		handlerCalled := false
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
		}).Methods(http.MethodGet, http.MethodPost)
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.False(t, handlerCalled)
	})

	t.Run("sets Allow-Methods from config on preflight", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowedMethods: []string{http.MethodGet, http.MethodPut, http.MethodDelete},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "PUT")
		r.ServeHTTP(w, req)

		methods := w.Header().Get("Access-Control-Allow-Methods")
		assert.Equal(t, "GET,PUT,DELETE", methods)
	})

	t.Run("auto-discovers methods on preflight when AllowedMethods empty", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		methods := w.Header().Get("Access-Control-Allow-Methods")
		assert.Contains(t, methods, http.MethodGet)
		assert.Contains(t, methods, http.MethodPost)
	})

	t.Run("sets Allow-Headers from config on preflight", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Equal(t, "Content-Type,Authorization", w.Header().Get("Access-Control-Allow-Headers"))
	})

	t.Run("reflects request headers on preflight when AllowedHeaders empty", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "X-Custom, X-Other")
		r.ServeHTTP(w, req)

		assert.Equal(t, "X-Custom, X-Other", w.Header().Get("Access-Control-Allow-Headers"))
	})

	t.Run("sets Max-Age on preflight", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			MaxAge:         3600,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Equal(t, "3600", w.Header().Get("Access-Control-Max-Age"))
	})

	t.Run("sends Max-Age 0 for negative config", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			MaxAge:         -1,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Equal(t, "0", w.Header().Get("Access-Control-Max-Age"))
	})

	t.Run("omits Max-Age when zero", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			MaxAge:         0,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Max-Age"))
	})

	t.Run("OPTIONS without Access-Control-Request-Method is not preflight", func(t *testing.T) {
		r := mux.NewRouter()
		handlerCalled := false
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
		}).Methods(http.MethodOptions)
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.True(t, handlerCalled)
		assert.NotEqual(t, http.StatusNoContent, w.Code)
	})

	t.Run("sets Expose-Headers on actual request", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			ExposeHeaders:  []string{"X-Request-Id", "X-Total-Count"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "X-Request-Id,X-Total-Count", w.Header().Get("Access-Control-Expose-Headers"))
	})

	t.Run("does not set Expose-Headers when empty", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Expose-Headers"))
	})

	t.Run("adds Vary Origin for reflected origin", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.Contains(t, w.Header().Values("Vary"), "Origin")
	})

	t.Run("does not add Vary Origin for wildcard", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"*"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.NotContains(t, w.Header().Values("Vary"), "Origin")
	})

	t.Run("adds Vary preflight headers on preflight", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		vary := w.Header().Values("Vary")
		assert.Contains(t, vary, "Access-Control-Request-Method")
		assert.Contains(t, vary, "Access-Control-Request-Headers")
	})

	// Feature 1: Wildcard + Credentials Validation

	t.Run("returns error for wildcard origin with credentials", func(t *testing.T) {
		r := newTestRouter()
		_, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: true,
		})
		assert.ErrorIs(t, err, ErrWildcardCredentials)
	})

	t.Run("allows credentials with AllowOriginFunc instead of wildcard", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowOriginFunc:  func(_ string) bool { return true },
			AllowCredentials: true,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	})

	// Feature 2: Origin Case Normalization

	t.Run("matches origin case-insensitively", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "HTTPS://EXAMPLE.COM")
		r.ServeHTTP(w, req)

		assert.Equal(t, "HTTPS://EXAMPLE.COM", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("reflects original origin casing in response header", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://EXAMPLE.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://Example.COM")
		r.ServeHTTP(w, req)

		assert.Equal(t, "https://Example.COM", w.Header().Get("Access-Control-Allow-Origin"))
	})

	// Feature 3: Subdomain Wildcard Patterns

	t.Run("matches subdomain wildcard pattern", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://*.example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://app.example.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("matches deep subdomain wildcard pattern", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://*.example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://a.b.example.com")
		r.ServeHTTP(w, req)

		assert.Equal(t, "https://a.b.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("rejects origin not matching wildcard pattern", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://*.example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://evil.com")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("returns error for multiple wildcards in pattern", func(t *testing.T) {
		r := newTestRouter()
		_, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://*.*"}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "multiple wildcards")
	})

	t.Run("mixes exact and wildcard origins", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://exact.com", "https://*.example.com"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://exact.com")
		r.ServeHTTP(w, req)
		assert.Equal(t, "https://exact.com", w.Header().Get("Access-Control-Allow-Origin"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://app.example.com")
		r.ServeHTTP(w, req)
		assert.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	// Feature 4: Configurable Preflight Status Code

	t.Run("uses custom OptionsStatusCode for preflight", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins:    []string{"https://example.com"},
			OptionsStatusCode: http.StatusOK,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("defaults to 204 when OptionsStatusCode is zero", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	// Feature 5: OptionsPassthrough

	t.Run("OptionsPassthrough forwards preflight to next handler", func(t *testing.T) {
		r := mux.NewRouter()
		handlerCalled := false
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins:     []string{"https://example.com"},
			OptionsPassthrough: true,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.True(t, handlerCalled)
		assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("OptionsPassthrough false stops chain on preflight", func(t *testing.T) {
		r := mux.NewRouter()
		handlerCalled := false
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
		}).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins:     []string{"https://example.com"},
			OptionsPassthrough: false,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.False(t, handlerCalled)
		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	// Feature 6: AllowedHeaders Wildcard

	t.Run("reflects request headers when AllowedHeaders contains wildcard", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowedHeaders: []string{"*"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "X-Custom, X-Other")
		r.ServeHTTP(w, req)

		assert.Equal(t, "X-Custom, X-Other", w.Header().Get("Access-Control-Allow-Headers"))
	})

	t.Run("AllowedHeaders wildcard with no request headers", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowedHeaders: []string{"*"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Headers"))
	})

	// Feature 7: Vary: Origin on Non-CORS Requests

	t.Run("sets Vary Origin on non-CORS request with specific origins", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://example.com"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)

		assert.Contains(t, w.Header().Values("Vary"), "Origin")
	})

	t.Run("does not set Vary Origin on non-CORS request with wildcard only", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"*"}})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)

		assert.NotContains(t, w.Header().Values("Vary"), "Origin")
	})

	t.Run("sets Vary Origin on non-CORS request with AllowOriginFunc", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowOriginFunc: func(_ string) bool { return true },
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)

		assert.Contains(t, w.Header().Values("Vary"), "Origin")
	})

	t.Run("sets Vary Origin on non-CORS request with wildcard pattern", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://*.example.com"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)

		assert.Contains(t, w.Header().Values("Vary"), "Origin")
	})

	// Private Network Access

	t.Run("sets Allow-Private-Network on preflight when enabled", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins:      []string{"https://example.com"},
			AllowPrivateNetwork: true,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Private-Network", "true")
		r.ServeHTTP(w, req)

		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Private-Network"))
		assert.Contains(t, w.Header().Values("Vary"), "Access-Control-Request-Private-Network")
	})

	t.Run("does not set Allow-Private-Network when disabled", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Private-Network", "true")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Private-Network"))
		assert.NotContains(t, w.Header().Values("Vary"), "Access-Control-Request-Private-Network")
	})

	t.Run("does not set Allow-Private-Network when request header absent", func(t *testing.T) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins:      []string{"https://example.com"},
			AllowPrivateNetwork: true,
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		r.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Private-Network"))
	})
}

func TestGetAllMethodsForRoute(t *testing.T) {
	t.Run("returns all methods for matching path", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodPost)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		methods, err := getAllMethodsForRoute(r, req)
		require.NoError(t, err)
		assert.Contains(t, methods, http.MethodGet)
		assert.Contains(t, methods, http.MethodPost)
		assert.NotContains(t, methods, http.MethodOptions)
	})

	t.Run("returns error when no methods match", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/other", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		_, err := getAllMethodsForRoute(r, req)
		assert.Error(t, err)
	})

	t.Run("includes OPTIONS only when explicitly registered", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet, http.MethodOptions)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		methods, err := getAllMethodsForRoute(r, req)
		require.NoError(t, err)
		assert.Contains(t, methods, http.MethodGet)
		assert.Contains(t, methods, http.MethodOptions)
	})

	t.Run("skips routes without methods", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		methods, err := getAllMethodsForRoute(r, req)
		require.NoError(t, err)
		assert.Contains(t, methods, http.MethodGet)
	})
}

func BenchmarkCORSMiddleware(b *testing.B) {
	b.Run("actual request", func(b *testing.B) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		req.Header.Set("Origin", "https://example.com")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("preflight", func(b *testing.B) {
		r := newTestRouter()
		mw, err := CORSMiddleware(r, CORSConfig{
			AllowedOrigins: []string{"https://example.com"},
			AllowedMethods: []string{http.MethodGet, http.MethodPost},
			AllowedHeaders: []string{"Content-Type"},
			MaxAge:         3600,
		})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)
		req := httptest.NewRequest(http.MethodOptions, "/users", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
