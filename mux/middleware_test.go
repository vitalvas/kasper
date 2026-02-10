package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterUse(t *testing.T) {
	t.Run("applies single middleware", func(t *testing.T) {
		r := NewRouter()
		var order []string
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "mw")
				next.ServeHTTP(w, req)
			})
		})
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"mw", "handler"}, order)
	})

	t.Run("applies multiple middleware in order", func(t *testing.T) {
		r := NewRouter()
		var order []string
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "first")
				next.ServeHTTP(w, req)
			})
		})
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "second")
				next.ServeHTTP(w, req)
			})
		})
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"first", "second", "handler"}, order)
	})

	t.Run("middleware applies to not-found without panic", func(t *testing.T) {
		r := NewRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "yes")
				next.ServeHTTP(w, req)
			})
		})
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("middleware can modify response", func(t *testing.T) {
		r := NewRouter()
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Middleware", "applied")
				next.ServeHTTP(w, req)
			})
		})
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "applied", w.Header().Get("X-Middleware"))
		assert.Equal(t, "ok", w.Body.String())
	})

	t.Run("middleware can short-circuit", func(t *testing.T) {
		r := NewRouter()
		r.Use(func(_ http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			})
		})
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "should not reach")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Empty(t, w.Body.String())
	})
}

func TestCORSMethodMiddleware(t *testing.T) {
	t.Run("sets Access-Control-Allow-Methods header", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		}).Methods(http.MethodGet, http.MethodPost)

		r.Use(CORSMethodMiddleware(r))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)

		allowMethods := w.Header().Get("Access-Control-Allow-Methods")
		require.NotEmpty(t, allowMethods)
		assert.Contains(t, allowMethods, http.MethodGet)
		assert.Contains(t, allowMethods, http.MethodPost)
		assert.Contains(t, allowMethods, http.MethodOptions)
	})

	t.Run("includes OPTIONS method", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		r.Use(CORSMethodMiddleware(r))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		allowMethods := w.Header().Get("Access-Control-Allow-Methods")
		assert.Contains(t, allowMethods, http.MethodOptions)
	})
}

// --- Benchmarks ---

func BenchmarkCORSMethodMiddleware(b *testing.B) {
	r := NewRouter()
	r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
		Methods(http.MethodGet, http.MethodPost, http.MethodPut)
	r.Use(CORSMethodMiddleware(r))
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkMiddlewareChain(b *testing.B) {
	r := NewRouter()
	for range 5 {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req)
			})
		})
	}
	r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func TestGetAllMethodsForRoute(t *testing.T) {
	t.Run("returns all methods for matching path", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodPost)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		methods, err := getAllMethodsForRoute(r, req)
		require.NoError(t, err)
		assert.Contains(t, methods, http.MethodGet)
		assert.Contains(t, methods, http.MethodPost)
		assert.Contains(t, methods, http.MethodOptions)
	})

	t.Run("returns error when no methods match", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/other", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		_, err := getAllMethodsForRoute(r, req)
		assert.Error(t, err)
	})

	t.Run("does not duplicate OPTIONS when already registered", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet, http.MethodOptions)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		methods, err := getAllMethodsForRoute(r, req)
		require.NoError(t, err)
		count := 0
		for _, m := range methods {
			if m == http.MethodOptions {
				count++
			}
		}
		assert.Equal(t, 1, count)
	})

	t.Run("skips routes without methods", func(t *testing.T) {
		r := NewRouter()
		// Route without Methods() call - has no method matcher
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
		// Route with Methods() call
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		methods, err := getAllMethodsForRoute(r, req)
		require.NoError(t, err)
		assert.Contains(t, methods, http.MethodGet)
		assert.Contains(t, methods, http.MethodOptions)
	})
}
