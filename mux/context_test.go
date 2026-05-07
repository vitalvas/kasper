package mux

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVars(t *testing.T) {
	t.Run("returns nil for request without vars", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.Nil(t, Vars(r))
	})

	t.Run("returns vars from request context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		vars := map[string]string{"id": "42", "name": "test"}
		r = setRouteContext(r, nil, vars)
		result := Vars(r)
		require.NotNil(t, result)
		assert.Equal(t, "42", result["id"])
		assert.Equal(t, "test", result["name"])
	})
}

func TestVarGet(t *testing.T) {
	tests := []struct {
		name      string
		setupVars map[string]string
		hasSetup  bool
		key       string
		wantVal   string
		wantOK    bool
	}{
		{
			name:   "returns false for request without vars",
			key:    "id",
			wantOK: false,
		},
		{
			name:      "returns false for missing key",
			hasSetup:  true,
			setupVars: map[string]string{"name": "test"},
			key:       "id",
			wantOK:    false,
		},
		{
			name:      "returns value and true for existing key",
			hasSetup:  true,
			setupVars: map[string]string{"id": "42", "name": "test"},
			key:       "id",
			wantVal:   "42",
			wantOK:    true,
		},
		{
			name:      "returns false for nil vars map",
			hasSetup:  true,
			setupVars: nil,
			key:       "id",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.hasSetup {
				r = setRouteContext(r, nil, tt.setupVars)
			}

			val, ok := VarGet(r, tt.key)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantVal, val)
			} else {
				assert.Empty(t, val)
			}
		})
	}
}

func TestCurrentRoute(t *testing.T) {
	t.Run("returns nil for request without route", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.Nil(t, CurrentRoute(r))
	})

	t.Run("returns route from request context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		route := &Route{}
		r = setRouteContext(r, route, nil)
		result := CurrentRoute(r)
		require.NotNil(t, result)
		assert.Equal(t, route, result)
	})
}

func TestCurrentRouter(t *testing.T) {
	t.Run("returns nil for request without router", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.Nil(t, CurrentRouter(r))
	})

	t.Run("returns router from handler context", func(t *testing.T) {
		router := NewRouter()
		var got *Router
		router.HandleFunc("/test", func(_ http.ResponseWriter, r *http.Request) {
			got = CurrentRouter(r)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, router, got)
	})

	t.Run("returns innermost subrouter", func(t *testing.T) {
		router := NewRouter()
		sub := router.PathPrefix("/api").Subrouter()
		var got *Router
		sub.HandleFunc("/users", func(_ http.ResponseWriter, r *http.Request) {
			got = CurrentRouter(r)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, sub, got)
	})
}

func TestSetURLVars(t *testing.T) {
	t.Run("sets vars on request", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		vars := map[string]string{"key": "value"}
		r = SetURLVars(r, vars)
		result := Vars(r)
		require.NotNil(t, result)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("overwrites existing vars", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = SetURLVars(r, map[string]string{"a": "1"})
		r = SetURLVars(r, map[string]string{"b": "2"})
		result := Vars(r)
		require.NotNil(t, result)
		assert.Empty(t, result["a"])
		assert.Equal(t, "2", result["b"])
	})

	t.Run("preserves existing route", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		route := &Route{}
		r = setRouteContext(r, route, map[string]string{"a": "1"})
		r = SetURLVars(r, map[string]string{"b": "2"})
		assert.Equal(t, route, CurrentRoute(r))
		assert.Equal(t, "2", Vars(r)["b"])
	})
}

func TestMatcherFunc(t *testing.T) {
	t.Run("implements matcher interface", func(t *testing.T) {
		called := false
		fn := MatcherFunc(func(_ *http.Request, _ *RouteMatch) bool {
			called = true
			return true
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		result := fn.Match(r, &RouteMatch{})
		assert.True(t, called)
		assert.True(t, result)
	})

	t.Run("returns false when matcher rejects", func(t *testing.T) {
		fn := MatcherFunc(func(_ *http.Request, _ *RouteMatch) bool {
			return false
		})
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.False(t, fn.Match(r, &RouteMatch{}))
	})
}

func TestMiddlewareFunc(t *testing.T) {
	t.Run("wraps handler", func(t *testing.T) {
		called := false
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				next.ServeHTTP(w, r)
			})
		})
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		handler := mw.Middleware(inner)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)
		assert.True(t, called)
	})
}

func TestReverse(t *testing.T) {
	t.Run("returns error when no router in context", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := Reverse(r, "x")
		assert.ErrorIs(t, err, ErrNoRouterInContext)
	})

	t.Run("returns error for unknown route", func(t *testing.T) {
		router := NewRouter()
		var got error
		router.HandleFunc("/test", func(_ http.ResponseWriter, r *http.Request) {
			_, got = Reverse(r, "missing")
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)
		assert.ErrorIs(t, got, ErrRouteNotFound)
	})

	t.Run("builds path for static named route", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/login", func(_ http.ResponseWriter, _ *http.Request) {}).Name("login")
		var url string
		router.HandleFunc("/test", func(_ http.ResponseWriter, r *http.Request) {
			url, _ = Reverse(r, "login")
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, "/login", url)
	})

	t.Run("builds path with vars", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/products/{pk}", func(_ http.ResponseWriter, _ *http.Request) {}).Name("product-detail")
		var url string
		router.HandleFunc("/test", func(_ http.ResponseWriter, r *http.Request) {
			url, _ = Reverse(r, "product-detail", "pk", "123")
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, "/products/123", url)
	})

	t.Run("returns error for missing var", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/products/{pk}", func(_ http.ResponseWriter, _ *http.Request) {}).Name("product-detail")
		var got error
		router.HandleFunc("/test", func(_ http.ResponseWriter, r *http.Request) {
			_, got = Reverse(r, "product-detail")
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)
		assert.Error(t, got)
	})

	t.Run("resolves names registered on subrouter from parent handler", func(t *testing.T) {
		router := NewRouter()
		sub := router.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).Name("user-detail")
		var url string
		router.HandleFunc("/test", func(_ http.ResponseWriter, r *http.Request) {
			url, _ = Reverse(r, "user-detail", "id", "42")
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, "/api/users/42", url)
	})
}

func TestReverseMacros(t *testing.T) {
	tests := []struct {
		name    string
		pattern string // path template
		varName string
		value   string
		want    string
	}{
		{"uuid", "/items/{id:uuid}", "id", "550e8400-e29b-41d4-a716-446655440000", "/items/550e8400-e29b-41d4-a716-446655440000"},
		{"int", "/page/{n:int}", "n", "42", "/page/42"},
		{"float", "/value/{v:float}", "v", "3.14", "/value/3.14"},
		{"float-leading-dot", "/value/{v:float}", "v", ".5", "/value/.5"},
		{"slug", "/posts/{s:slug}", "s", "my-post-title", "/posts/my-post-title"},
		{"alpha", "/names/{n:alpha}", "n", "hello", "/names/hello"},
		{"alphanum", "/tokens/{t:alphanum}", "t", "abc123", "/tokens/abc123"},
		{"date", "/events/{d:date}", "d", "2024-01-15", "/events/2024-01-15"},
		{"hex", "/colors/{h:hex}", "h", "deadBEEF", "/colors/deadBEEF"},
		{"domain", "/sites/{d:domain}", "d", "sub.example.co.uk", "/sites/sub.example.co.uk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			router.HandleFunc(tt.pattern, func(_ http.ResponseWriter, _ *http.Request) {}).Name(tt.name)
			var got string
			var gotErr error
			router.HandleFunc("/probe", func(_ http.ResponseWriter, r *http.Request) {
				got, gotErr = Reverse(r, tt.name, tt.varName, tt.value)
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			router.ServeHTTP(w, req)
			require.NoError(t, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReverseMultipleVars(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		pairs   []string
		want    string
	}{
		{
			name:    "two-plain-vars",
			pattern: "/articles/{category}/{id}",
			pairs:   []string{"category", "tech", "id", "42"},
			want:    "/articles/tech/42",
		},
		{
			name:    "pairs-order-independent",
			pattern: "/articles/{category}/{id}",
			pairs:   []string{"id", "42", "category", "tech"},
			want:    "/articles/tech/42",
		},
		{
			name:    "mixed-macros",
			pattern: "/users/{id:uuid}/posts/{n:int}",
			pairs:   []string{"id", "550e8400-e29b-41d4-a716-446655440000", "n", "7"},
			want:    "/users/550e8400-e29b-41d4-a716-446655440000/posts/7",
		},
		{
			name:    "three-vars-mixed-types",
			pattern: "/{year:int}/{month:int}/{slug:slug}",
			pairs:   []string{"year", "2026", "month", "5", "slug", "hello-world"},
			want:    "/2026/5/hello-world",
		},
		{
			name:    "var-with-static-segments",
			pattern: "/api/v1/users/{id:int}/comments/{cid:int}",
			pairs:   []string{"id", "1", "cid", "99"},
			want:    "/api/v1/users/1/comments/99",
		},
		{
			name:    "macro-and-regex-mix",
			pattern: "/files/{name:slug}/v{version:[0-9]+}",
			pairs:   []string{"name", "report", "version", "3"},
			want:    "/files/report/v3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			router.HandleFunc(tt.pattern, func(_ http.ResponseWriter, _ *http.Request) {}).Name(tt.name)
			var got string
			var gotErr error
			router.HandleFunc("/probe", func(_ http.ResponseWriter, r *http.Request) {
				got, gotErr = Reverse(r, tt.name, tt.pairs...)
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			router.ServeHTTP(w, req)
			require.NoError(t, gotErr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReverseMultipleVarsErrors(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		pairs   []string
	}{
		{
			name:    "missing-second-var",
			pattern: "/articles/{category}/{id}",
			pairs:   []string{"category", "tech"},
		},
		{
			name:    "odd-number-of-pairs",
			pattern: "/articles/{category}/{id}",
			pairs:   []string{"category", "tech", "id"},
		},
		{
			name:    "second-var-fails-macro",
			pattern: "/users/{id:uuid}/posts/{n:int}",
			pairs:   []string{"id", "550e8400-e29b-41d4-a716-446655440000", "n", "abc"},
		},
		{
			name:    "unknown-var-name",
			pattern: "/articles/{category}/{id}",
			pairs:   []string{"category", "tech", "wrong", "42"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			router.HandleFunc(tt.pattern, func(_ http.ResponseWriter, _ *http.Request) {}).Name(tt.name)
			var gotErr error
			router.HandleFunc("/probe", func(_ http.ResponseWriter, r *http.Request) {
				_, gotErr = Reverse(r, tt.name, tt.pairs...)
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			router.ServeHTTP(w, req)
			assert.Error(t, gotErr)
		})
	}
}

func TestReverseMacroValidation(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		varName string
		value   string
	}{
		{"uuid-rejects-non-uuid", "/items/{id:uuid}", "id", "not-a-uuid"},
		{"int-rejects-letters", "/page/{n:int}", "n", "abc"},
		{"int-rejects-negative", "/page/{n:int}", "n", "-1"},
		{"slug-rejects-space", "/posts/{s:slug}", "s", "with space"},
		{"alpha-rejects-digits", "/names/{n:alpha}", "n", "abc123"},
		{"date-rejects-bad-format", "/events/{d:date}", "d", "2024/01/15"},
		{"hex-rejects-non-hex", "/colors/{h:hex}", "h", "xyz"},
		{"domain-rejects-empty", "/sites/{d:domain}", "d", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			router.HandleFunc(tt.pattern, func(_ http.ResponseWriter, _ *http.Request) {}).Name(tt.name)
			var gotErr error
			router.HandleFunc("/probe", func(_ http.ResponseWriter, r *http.Request) {
				_, gotErr = Reverse(r, tt.name, tt.varName, tt.value)
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			router.ServeHTTP(w, req)
			assert.Error(t, gotErr)
		})
	}
}

func TestScheme(t *testing.T) {
	t.Run("returns http for plain request", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.URL.Scheme = ""
		r.TLS = nil
		assert.Equal(t, "http", Scheme(r))
	})

	t.Run("returns https when r.TLS is set", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.URL.Scheme = ""
		r.TLS = &tls.ConnectionState{}
		assert.Equal(t, "https", Scheme(r))
	})

	t.Run("uses r.URL.Scheme when populated", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.URL.Scheme = "https"
		r.TLS = nil
		assert.Equal(t, "https", Scheme(r))
	})

	t.Run("URL.Scheme takes precedence over TLS", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.URL.Scheme = "http"
		r.TLS = &tls.ConnectionState{}
		assert.Equal(t, "http", Scheme(r))
	})
}

func FuzzReverse(f *testing.F) {
	router := NewRouter()
	router.HandleFunc("/articles/{category}/{id:[0-9]+}", func(_ http.ResponseWriter, _ *http.Request) {}).Name("article")
	router.HandleFunc("/users/{id:uuid}", func(_ http.ResponseWriter, _ *http.Request) {}).Name("user")
	router.HandleFunc("/posts/{slug:slug}", func(_ http.ResponseWriter, _ *http.Request) {}).Name("post")
	router.HandleFunc("/static", func(_ http.ResponseWriter, _ *http.Request) {}).Name("static")

	probe := httptest.NewRequest(http.MethodGet, "/probe", nil)
	router.HandleFunc("/probe", func(_ http.ResponseWriter, r *http.Request) {
		probe = r
	})
	router.ServeHTTP(httptest.NewRecorder(), probe)

	f.Add("article", "category", "tech", "id", "42")
	f.Add("user", "id", "550e8400-e29b-41d4-a716-446655440000", "", "")
	f.Add("post", "slug", "hello-world", "", "")
	f.Add("static", "", "", "", "")
	f.Add("missing", "x", "y", "", "")
	f.Add("", "", "", "", "")

	f.Fuzz(func(_ *testing.T, name, k1, v1, k2, v2 string) {
		pairs := []string{k1, v1, k2, v2}
		_, _ = Reverse(probe, name, pairs...)
		_, _ = Reverse(probe, name)
		_, _ = Reverse(probe, name, k1)
		_, _ = Reverse(probe, name, k1, v1)
		_ = Scheme(probe)
	})
}

// --- Benchmarks ---

func BenchmarkVars(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	vars := map[string]string{"id": "42", "name": "test", "action": "view"}
	r = setRouteContext(r, nil, vars)
	b.ResetTimer()
	for b.Loop() {
		Vars(r)
	}
}

func BenchmarkSetURLVars(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	vars := map[string]string{"id": "42", "name": "test"}
	b.ResetTimer()
	for b.Loop() {
		SetURLVars(r, vars)
	}
}

func BenchmarkCurrentRoute(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	route := &Route{}
	r = setRouteContext(r, route, nil)
	b.ResetTimer()
	for b.Loop() {
		CurrentRoute(r)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "ErrMethodMismatch has correct message",
			err:      ErrMethodMismatch,
			expected: "method is not allowed",
		},
		{
			name:     "ErrNotFound has correct message",
			err:      ErrNotFound,
			expected: "no matching route was found",
		},
		{
			name:     "SkipRouter has correct message",
			err:      SkipRouter,
			expected: "skip this router",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}
