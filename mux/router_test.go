package mux

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouter(t *testing.T) {
	t.Run("creates router with initialized namedRoutes", func(t *testing.T) {
		r := NewRouter()
		require.NotNil(t, r)
		assert.NotNil(t, r.namedRoutes)
	})
}

func TestRouterServeHTTP(t *testing.T) {
	t.Run("dispatches to matched handler", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "world")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "world", w.Body.String())
	})

	t.Run("returns 404 for unmatched path", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/hello", func(_ http.ResponseWriter, _ *http.Request) {})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("uses custom NotFoundHandler", func(t *testing.T) {
		r := NewRouter()
		r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "custom 404")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "custom 404", w.Body.String())
	})

	t.Run("sets Vars in request context", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
			vars := Vars(req)
			fmt.Fprint(w, vars["id"])
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "42", w.Body.String())
	})

	t.Run("sets CurrentRoute in request context", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, req *http.Request) {
			route := CurrentRoute(req)
			assert.NotNil(t, route)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
	})

	t.Run("cleans path by default", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/../users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed returns 405", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("returns 404 when matched route has nil handler", func(t *testing.T) {
		r := NewRouter()
		r.NewRoute().Path("/test") // No handler set

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("uses custom MethodNotAllowedHandler", func(t *testing.T) {
		r := NewRouter()
		r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprint(w, "custom 405")
		})
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "custom 405", w.Body.String())
	})
}

func TestRouterStrictSlash(t *testing.T) {
	t.Run("redirects to slash when template has slash", func(t *testing.T) {
		r := NewRouter()
		r.StrictSlash(true)
		r.HandleFunc("/users/", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusPermanentRedirect, w.Code)
	})

	t.Run("redirects to no-slash when template has no slash", func(t *testing.T) {
		r := NewRouter()
		r.StrictSlash(true)
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusPermanentRedirect, w.Code)
	})

	t.Run("uses 308 to preserve method on redirect", func(t *testing.T) {
		r := NewRouter()
		r.StrictSlash(true)
		r.HandleFunc("/users/", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusPermanentRedirect, w.Code)
	})

	t.Run("redirect from slash to no-slash uses 308", func(t *testing.T) {
		r := NewRouter()
		r.StrictSlash(true)
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusPermanentRedirect, w.Code)
	})

	t.Run("serves normally when slash matches", func(t *testing.T) {
		r := NewRouter()
		r.StrictSlash(true)
		r.HandleFunc("/users/", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRouterSkipClean(t *testing.T) {
	t.Run("does not clean path when SkipClean is true", func(t *testing.T) {
		r := NewRouter()
		r.SkipClean(true)
		r.HandleFunc("/users/../admin", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/../admin", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRouterUseEncodedPath(t *testing.T) {
	t.Run("matches encoded path", func(_ *testing.T) {
		r := NewRouter()
		r.UseEncodedPath()
		r.HandleFunc("/a]b", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/a%5Db", nil)
		r.ServeHTTP(w, req)
	})
}

func TestRouterMatch(t *testing.T) {
	t.Run("matches first registered route", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/first", func(_ http.ResponseWriter, _ *http.Request) {}).Name("first")
		r.HandleFunc("/second", func(_ http.ResponseWriter, _ *http.Request) {}).Name("second")

		req := httptest.NewRequest(http.MethodGet, "/first", nil)
		match := &RouteMatch{}
		assert.True(t, r.Match(req, match))
		assert.Equal(t, "first", match.Route.GetName())
	})

	t.Run("returns false for no match", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})

		req := httptest.NewRequest(http.MethodGet, "/posts", nil)
		match := &RouteMatch{}
		assert.False(t, r.Match(req, match))
	})
}

func TestRouterRouteFactoryMethods(t *testing.T) {
	t.Run("Handle registers route with handler", func(t *testing.T) {
		r := NewRouter()
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "handled")
		})
		r.Handle("/test", h)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "handled", w.Body.String())
	})

	t.Run("Path creates route with path matcher", func(t *testing.T) {
		r := NewRouter()
		r.Path("/path").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "path")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/path", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "path", w.Body.String())
	})

	t.Run("PathPrefix creates route with prefix matcher", func(t *testing.T) {
		r := NewRouter()
		r.PathPrefix("/api/").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "api")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "api", w.Body.String())
	})

	t.Run("Methods creates route with method matcher", func(t *testing.T) {
		r := NewRouter()
		r.Methods(http.MethodPost).Path("/submit").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "submitted")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/submit", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "submitted", w.Body.String())
	})

	t.Run("Host creates route with host matcher", func(t *testing.T) {
		r := NewRouter()
		r.Host("example.com").Path("/test").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "host matched")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		req.Host = "example.com"
		r.ServeHTTP(w, req)
		assert.Equal(t, "host matched", w.Body.String())
	})

	t.Run("Schemes creates route with scheme matcher", func(t *testing.T) {
		r := NewRouter()
		r.Schemes("https").Path("/secure").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "secure")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://example.com/secure", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "secure", w.Body.String())
	})

	t.Run("Headers creates route with header matcher", func(t *testing.T) {
		r := NewRouter()
		r.Headers("X-Token", "secret").Path("/protected").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "protected")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("X-Token", "secret")
		r.ServeHTTP(w, req)
		assert.Equal(t, "protected", w.Body.String())
	})

	t.Run("Queries creates route with query matcher", func(t *testing.T) {
		r := NewRouter()
		r.Queries("key", "val").Path("/search").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "found")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/search?key=val", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "found", w.Body.String())
	})

	t.Run("Name creates route with name", func(t *testing.T) {
		r := NewRouter()
		r.Name("named").Path("/named").HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		assert.NotNil(t, r.Get("named"))
	})
}

func TestRouterGet(t *testing.T) {
	t.Run("returns named route", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")

		route := r.Get("users")
		require.NotNil(t, route)
		assert.Equal(t, "users", route.GetName())
	})

	t.Run("returns nil for unknown name", func(t *testing.T) {
		r := NewRouter()
		assert.Nil(t, r.Get("unknown"))
	})

	t.Run("GetRoute is alias for Get", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")
		assert.Equal(t, r.Get("users"), r.GetRoute("users"))
	})
}

func TestRouterWalk(t *testing.T) {
	t.Run("walks all routes", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/a", func(_ http.ResponseWriter, _ *http.Request) {}).Name("a")
		r.HandleFunc("/b", func(_ http.ResponseWriter, _ *http.Request) {}).Name("b")

		var names []string
		err := r.Walk(func(route *Route, _ *Router, _ []*Route) error {
			names = append(names, route.GetName())
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, names)
	})

	t.Run("walks subrouter routes", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/root", func(_ http.ResponseWriter, _ *http.Request) {}).Name("root")
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("api-users")

		var names []string
		err := r.Walk(func(route *Route, _ *Router, _ []*Route) error {
			if route.GetName() != "" {
				names = append(names, route.GetName())
			}
			return nil
		})
		require.NoError(t, err)
		assert.Contains(t, names, "root")
		assert.Contains(t, names, "api-users")
	})

	t.Run("passes ancestors for subrouter routes", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")

		err := r.Walk(func(route *Route, _ *Router, ancestors []*Route) error {
			if route.GetName() == "users" {
				assert.Len(t, ancestors, 1)
			}
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("SkipRouter skips subrouter", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")
		r.HandleFunc("/other", func(_ http.ResponseWriter, _ *http.Request) {}).Name("other")

		var names []string
		err := r.Walk(func(route *Route, _ *Router, _ []*Route) error {
			if route.GetName() != "" {
				names = append(names, route.GetName())
			}
			tpl, _ := route.GetPathTemplate()
			if tpl == "/api" {
				return SkipRouter
			}
			return nil
		})
		require.NoError(t, err)
		assert.NotContains(t, names, "users")
		assert.Contains(t, names, "other")
	})

	t.Run("propagates walk errors", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {})

		expectedErr := errors.New("walk error")
		err := r.Walk(func(_ *Route, _ *Router, _ []*Route) error {
			return expectedErr
		})
		assert.Equal(t, expectedErr, err)
	})
}

func TestRouterMultipleRoutes(t *testing.T) {
	t.Run("matches routes in registration order", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "first")
		})
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "second")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "first", w.Body.String())
	})

	t.Run("falls through to next route on method mismatch", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "get")
		}).Methods(http.MethodGet)
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "post")
		}).Methods(http.MethodPost)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "post", w.Body.String())
	})
}

func TestRouterComplexRouting(t *testing.T) {
	t.Run("path variables with multiple routes", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users/{id:[0-9]+}", func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, "user:"+Vars(req)["id"])
		})
		r.HandleFunc("/users/{name:[a-z]+}", func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, "name:"+Vars(req)["name"])
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "user:42", w.Body.String())

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/users/alice", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "name:alice", w.Body.String())
	})

	t.Run("host and path combined", func(t *testing.T) {
		r := NewRouter()
		r.Host("{subdomain}.example.com").
			Path("/users/{id}").
			HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				vars := Vars(req)
				fmt.Fprintf(w, "%s:%s", vars["subdomain"], vars["id"])
			})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://api.example.com/users/42", nil)
		req.Host = "api.example.com"
		r.ServeHTTP(w, req)
		assert.Equal(t, "api:42", w.Body.String())
	})

	t.Run("prefix with subrouter and variables", func(t *testing.T) {
		r := NewRouter()
		api := r.PathPrefix("/api/v1").Subrouter()
		api.HandleFunc("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, "user:"+Vars(req)["id"])
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/42", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "user:42", w.Body.String())
	})
}

func TestRouterHeadersRegexp(t *testing.T) {
	t.Run("creates route with header regex matcher", func(t *testing.T) {
		r := NewRouter()
		r.HeadersRegexp("Content-Type", "application/.*").Path("/api").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "api")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		assert.Equal(t, "api", w.Body.String())

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("Content-Type", "text/html")
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestRouterBuildVarsFunc(t *testing.T) {
	t.Run("creates route with build vars function", func(t *testing.T) {
		r := NewRouter()
		route := r.BuildVarsFunc(func(m map[string]string) map[string]string {
			m["id"] = "modified-" + m["id"]
			return m
		}).Path("/users/{id}").Name("user")

		u, err := route.URL("id", "42")
		require.NoError(t, err)
		assert.Equal(t, "/users/modified-42", u.Path)
	})
}

func TestRouterWalkSubrouterError(t *testing.T) {
	t.Run("propagates error from subrouter walk", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")

		expectedErr := errors.New("subrouter walk error")
		err := r.Walk(func(route *Route, _ *Router, _ []*Route) error {
			if route.GetName() == "users" {
				return expectedErr
			}
			return nil
		})
		assert.Equal(t, expectedErr, err)
	})
}

func TestRouterMatchErrMethodMismatch(t *testing.T) {
	t.Run("sets MatchErr to ErrMethodMismatch", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodDelete, "/users", nil)
		match := &RouteMatch{}
		assert.False(t, r.Match(req, match))
		assert.Equal(t, ErrMethodMismatch, match.MatchErr)
	})
}

func TestRouterServeHTTPEncodedPath(t *testing.T) {
	t.Run("cleans encoded path when useEncodedPath is true", func(t *testing.T) {
		r := NewRouter()
		r.UseEncodedPath()
		r.HandleFunc("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, Vars(req)["id"])
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/hello%20world", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "hello world", w.Body.String())
	})
}

func TestRouterMatchErrMethodMismatchFromMatcher(t *testing.T) {
	t.Run("sets MatchErr when custom matcher signals method mismatch", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			MatcherFunc(func(_ *http.Request, match *RouteMatch) bool {
				match.MatchErr = ErrMethodMismatch
				return false
			})

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		match := &RouteMatch{}
		assert.False(t, r.Match(req, match))
		assert.Equal(t, ErrMethodMismatch, match.MatchErr)
	})
}

func TestRouterServeHTTPEncodedPathClean(t *testing.T) {
	t.Run("cleans encoded path with double slashes", func(t *testing.T) {
		r := NewRouter()
		r.UseEncodedPath()
		r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/../users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRouterGetNamedRoutes(t *testing.T) {
	t.Run("returns named routes map", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")
		named := r.getNamedRoutes()
		assert.NotNil(t, named)
		assert.Contains(t, named, "users")
	})
}

// --- Benchmarks ---

func BenchmarkRouterServeHTTPSimple(b *testing.B) {
	r := NewRouter()
	r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkRouterServeHTTPWithVars(b *testing.B) {
	r := NewRouter()
	r.HandleFunc("/users/{id:[0-9]+}/posts/{pid}", func(_ http.ResponseWriter, _ *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/users/42/posts/123", nil)
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkRouterServeHTTPMultipleRoutes(b *testing.B) {
	r := NewRouter()
	for i := range 10 {
		r.HandleFunc(fmt.Sprintf("/route%d/{id}", i), func(_ http.ResponseWriter, _ *http.Request) {})
	}
	// Match the last route to test worst-case scan
	req := httptest.NewRequest(http.MethodGet, "/route9/42", nil)
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkRouterServeHTTPWithMiddleware(b *testing.B) {
	r := NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)
		})
	})
	r.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkRouterMatch(b *testing.B) {
	r := NewRouter()
	r.HandleFunc("/users/{id:[0-9]+}", func(_ http.ResponseWriter, _ *http.Request) {}).
		Methods(http.MethodGet)
	r.HandleFunc("/users/{id:[0-9]+}", func(_ http.ResponseWriter, _ *http.Request) {}).
		Methods(http.MethodPost)
	r.HandleFunc("/posts/{id}", func(_ http.ResponseWriter, _ *http.Request) {})
	req := httptest.NewRequest(http.MethodPost, "/users/42", nil)
	b.ResetTimer()
	for b.Loop() {
		r.Match(req, &RouteMatch{})
	}
}

func BenchmarkRouterNotFound(b *testing.B) {
	r := NewRouter()
	r.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {})
	r.HandleFunc("/posts/{id}", func(_ http.ResponseWriter, _ *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// --- Fuzz ---

func FuzzRouterMatch(f *testing.F) {
	f.Add("/users/42")
	f.Add("/posts/abc")
	f.Add("/")
	f.Add("/users/../admin")
	f.Add("/a/b/c/d/e")
	f.Add("/users/42/posts/123")
	f.Add("/%2F%2F")

	r := NewRouter()
	r.HandleFunc("/users/{id:[0-9]+}", func(_ http.ResponseWriter, _ *http.Request) {})
	r.HandleFunc("/posts/{slug}", func(_ http.ResponseWriter, _ *http.Request) {})
	r.HandleFunc("/articles/{category}/{id}", func(_ http.ResponseWriter, _ *http.Request) {})

	f.Fuzz(func(_ *testing.T, path string) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = path
		r.Match(req, &RouteMatch{})
	})
}

func TestSubrouterMethodNotAllowed(t *testing.T) {
	t.Run("subrouter returns 405 with multiple routes", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)
		sub.HandleFunc("/posts", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("subrouter returns 405 with multiple methods on same path", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodPost)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("method mismatch propagates from subrouter", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
		match := &RouteMatch{}
		r.Match(req, match)
		assert.Equal(t, ErrMethodMismatch, match.MatchErr)
		assert.True(t, match.methodNotAllowed)
	})
}

func TestSubrouterNotFoundHandler(t *testing.T) {
	t.Run("subrouter custom handler", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "subrouter 404")
		})
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "subrouter 404", w.Body.String())
	})

	t.Run("subrouter falls through without handler", func(t *testing.T) {
		r := NewRouter()
		r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "root 404")
		})
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "root 404", w.Body.String())
	})

	t.Run("subrouter preserves method not allowed", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "should not appear")
		})
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.NotContains(t, w.Body.String(), "should not appear")
	})

	t.Run("subrouter middleware applied", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		var middlewareCalled bool
		sub.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				middlewareCalled = true
				next.ServeHTTP(w, req)
			})
		})
		sub.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "sub 404")
		})
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "sub 404", w.Body.String())
		assert.True(t, middlewareCalled)
	})

	t.Run("nested subrouter handler", func(t *testing.T) {
		r := NewRouter()
		r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "root 404")
		})
		api := r.PathPrefix("/api").Subrouter()
		api.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "api 404")
		})
		v1 := api.PathPrefix("/v1").Subrouter()
		v1.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "v1 404")
		})
		v1.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "v1 404", w.Body.String())
	})
}

func TestMethodNotAllowedAllowHeader(t *testing.T) {
	t.Run("sets sorted Allow header on 405", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodPost)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "GET, POST", w.Header().Get("Allow"))
	})

	t.Run("sets Allow header with custom handler", func(t *testing.T) {
		r := NewRouter()
		r.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprint(w, "custom 405")
		})
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodPost)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "GET, POST", w.Header().Get("Allow"))
	})

	t.Run("sets Allow header for subrouter routes", func(t *testing.T) {
		r := NewRouter()
		sub := r.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodGet)
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodPost)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "GET, POST", w.Header().Get("Allow"))
	})

	t.Run("always sets Allow header even when empty per RFC 7231", func(t *testing.T) {
		r := NewRouter()
		r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			MatcherFunc(func(_ *http.Request, match *RouteMatch) bool {
				match.MatchErr = ErrMethodMismatch
				return false
			})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "", w.Header().Get("Allow"))
	})
}

func TestRouterMatcherFunc(t *testing.T) {
	t.Run("custom matcher function", func(t *testing.T) {
		r := NewRouter()
		r.MatcherFunc(func(req *http.Request, _ *RouteMatch) bool {
			return req.Header.Get("X-Custom") == "yes"
		}).Path("/custom").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "custom")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom", nil)
		req.Header.Set("X-Custom", "yes")
		r.ServeHTTP(w, req)
		assert.Equal(t, "custom", w.Body.String())

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/custom", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
