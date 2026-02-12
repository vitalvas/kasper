package mux

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteMatch(t *testing.T) {
	t.Run("matches path", func(t *testing.T) {
		router := NewRouter()
		handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		router.HandleFunc("/users/{id}", handler)

		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))
		assert.Equal(t, "42", match.Vars["id"])
	})

	t.Run("does not match wrong path", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {})

		req := httptest.NewRequest(http.MethodGet, "/posts/42", nil)
		match := &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})
}

func TestRouteMatchers(t *testing.T) {
	t.Run("method matcher", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Methods(http.MethodPost)

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))

		req = httptest.NewRequest(http.MethodGet, "/users", nil)
		match = &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})

	t.Run("header matcher", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/api", func(_ http.ResponseWriter, _ *http.Request) {}).
			Headers("Content-Type", "application/json")

		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("Content-Type", "application/json")
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))

		req = httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("Content-Type", "text/html")
		match = &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})

	t.Run("header regex matcher", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/api", func(_ http.ResponseWriter, _ *http.Request) {}).
			HeadersRegexp("Content-Type", "application/.*")

		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("Content-Type", "application/json")
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))

		req = httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("Content-Type", "text/html")
		match = &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})

	t.Run("scheme matcher", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/secure", func(_ http.ResponseWriter, _ *http.Request) {}).
			Schemes("https")

		req := httptest.NewRequest(http.MethodGet, "https://example.com/secure", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))

		req = httptest.NewRequest(http.MethodGet, "http://example.com/secure", nil)
		match = &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})

	t.Run("query matcher", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/search", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("q", "{query}")

		req := httptest.NewRequest(http.MethodGet, "/search?q=golang", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))
		assert.Equal(t, "golang", match.Vars["query"])

		req = httptest.NewRequest(http.MethodGet, "/search", nil)
		match = &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})

	t.Run("query matcher with pattern", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/page", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("page", "{page:[0-9]+}")

		req := httptest.NewRequest(http.MethodGet, "/page?page=5", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))
		assert.Equal(t, "5", match.Vars["page"])

		req = httptest.NewRequest(http.MethodGet, "/page?page=abc", nil)
		match = &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})
}

func TestRouteURLBuilding(t *testing.T) {
	t.Run("builds URL with path variables", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id}/posts/{pid}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("user-post")

		u, err := route.URL("id", "42", "pid", "123")
		require.NoError(t, err)
		assert.Equal(t, "/users/42/posts/123", u.Path)
	})

	t.Run("builds URL with host and path", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{subdomain}.example.com").
			Path("/users/{id}").
			Name("user")

		u, err := route.URL("subdomain", "api", "id", "42")
		require.NoError(t, err)
		assert.Equal(t, "api.example.com", u.Host)
		assert.Equal(t, "/users/42", u.Path)
		assert.Equal(t, "http", u.Scheme)
	})

	t.Run("URLPath builds only path", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/articles/{category}/{id:[0-9]+}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("article")

		u, err := route.URLPath("category", "tech", "id", "42")
		require.NoError(t, err)
		assert.Equal(t, "/articles/tech/42", u.Path)
		assert.Empty(t, u.Host)
	})

	t.Run("URLHost builds only host", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub}.example.com").
			Name("host-route")

		u, err := route.URLHost("sub", "api")
		require.NoError(t, err)
		assert.Equal(t, "api.example.com", u.Host)
	})

	t.Run("URL returns error for missing variable", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("user")

		_, err := route.URL()
		assert.Error(t, err)
	})

	t.Run("URLPath errors when no path template", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("example.com").Name("host-only")
		_, err := route.URLPath()
		assert.Error(t, err)
	})

	t.Run("URLHost errors when no host template", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/path", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("path-only")
		_, err := route.URLHost()
		assert.Error(t, err)
	})
}

func TestRouteInspection(t *testing.T) {
	t.Run("GetPathTemplate", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {})

		tpl, err := route.GetPathTemplate()
		require.NoError(t, err)
		assert.Equal(t, "/users/{id}", tpl)
	})

	t.Run("GetPathRegexp", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id:[0-9]+}", func(_ http.ResponseWriter, _ *http.Request) {})

		re, err := route.GetPathRegexp()
		require.NoError(t, err)
		assert.Contains(t, re, "[0-9]+")
	})

	t.Run("GetHostTemplate", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub}.example.com")

		tpl, err := route.GetHostTemplate()
		require.NoError(t, err)
		assert.Equal(t, "{sub}.example.com", tpl)
	})

	t.Run("GetMethods", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet, http.MethodPost)

		methods, err := route.GetMethods()
		require.NoError(t, err)
		assert.Contains(t, methods, http.MethodGet)
		assert.Contains(t, methods, http.MethodPost)
	})

	t.Run("GetMethods errors when no methods", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
		_, err := route.GetMethods()
		assert.Error(t, err)
	})

	t.Run("GetQueriesTemplates", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/search", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("q", "{query}", "page", "{page:[0-9]+}")

		templates, err := route.GetQueriesTemplates()
		require.NoError(t, err)
		assert.Len(t, templates, 2)
	})

	t.Run("GetQueriesRegexp", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/search", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("q", "{query}")

		regexps, err := route.GetQueriesRegexp()
		require.NoError(t, err)
		assert.Len(t, regexps, 1)
	})

	t.Run("GetVarNames", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub}.example.com").
			Path("/users/{id}")

		names, err := route.GetVarNames()
		require.NoError(t, err)
		assert.Contains(t, names, "sub")
		assert.Contains(t, names, "id")
	})

	t.Run("GetError returns nil for valid route", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
		assert.NoError(t, route.GetError())
	})

	t.Run("GetName", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")
		assert.Equal(t, "users", route.GetName())
	})

	t.Run("GetHandler", func(t *testing.T) {
		router := NewRouter()
		h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		route := router.Handle("/users", h)
		assert.NotNil(t, route.GetHandler())
	})
}

func TestRouteErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Router) *Route
	}{
		{
			name: "duplicate name error",
			setup: func(r *Router) *Route {
				return r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
					Name("users").
					Name("users2")
			},
		},
		{
			name: "odd query pairs error",
			setup: func(r *Router) *Route {
				return r.HandleFunc("/search", func(_ http.ResponseWriter, _ *http.Request) {}).
					Queries("q")
			},
		},
		{
			name: "odd header pairs error",
			setup: func(r *Router) *Route {
				return r.HandleFunc("/api", func(_ http.ResponseWriter, _ *http.Request) {}).
					Headers("Content-Type")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			route := tt.setup(router)
			assert.Error(t, route.GetError())
		})
	}
}

func TestRouteBuildOnly(t *testing.T) {
	t.Run("build only route does not match", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/internal", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("internal").
			BuildOnly()

		req := httptest.NewRequest(http.MethodGet, "/internal", nil)
		match := &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})

	t.Run("build only route can build URLs", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/articles/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("article").
			BuildOnly()

		u, err := route.URL("id", "42")
		require.NoError(t, err)
		assert.Equal(t, "/articles/42", u.Path)
	})
}

func TestRouteMethodMatcher(t *testing.T) {
	tests := []struct {
		name      string
		methods   []string
		reqMethod string
		expected  bool
	}{
		{
			name:      "matches correct method",
			methods:   []string{http.MethodGet, http.MethodPost},
			reqMethod: http.MethodGet,
			expected:  true,
		},
		{
			name:      "does not match wrong method",
			methods:   []string{http.MethodGet},
			reqMethod: http.MethodPost,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := methodMatcher(tt.methods)
			req := httptest.NewRequest(tt.reqMethod, "/", nil)
			assert.Equal(t, tt.expected, m.Match(req, &RouteMatch{}))
		})
	}
}

func TestRouteHeaderMatcher(t *testing.T) {
	tests := []struct {
		name     string
		matcher  headerMatcher
		header   string
		value    string
		expected bool
	}{
		{
			name:     "matches header",
			matcher:  headerMatcher{"Content-Type": "application/json"},
			header:   "Content-Type",
			value:    "application/json",
			expected: true,
		},
		{
			name:     "matches header presence only",
			matcher:  headerMatcher{"X-Custom": ""},
			header:   "X-Custom",
			value:    "anything",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(tt.header, tt.value)
			assert.Equal(t, tt.expected, tt.matcher.Match(req, &RouteMatch{}))
		})
	}
}

func TestRouteSkipClean(t *testing.T) {
	tests := []struct {
		name      string
		skipClean bool
		expected  bool
	}{
		{
			name:      "returns false by default",
			skipClean: false,
			expected:  false,
		},
		{
			name:      "returns true when router has SkipClean enabled",
			skipClean: true,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			if tt.skipClean {
				router.SkipClean(true)
			}
			route := router.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {})
			assert.Equal(t, tt.expected, route.SkipClean())
		})
	}
}

func TestRouteBuildVarsFunc(t *testing.T) {
	t.Run("applies build vars function", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("user").
			BuildVarsFunc(func(m map[string]string) map[string]string {
				m["id"] = "prefix-" + m["id"]
				return m
			})

		u, err := route.URL("id", "42")
		require.NoError(t, err)
		assert.Equal(t, "/users/prefix-42", u.Path)
	})

	t.Run("chains multiple build vars functions", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("user").
			BuildVarsFunc(func(m map[string]string) map[string]string {
				m["id"] = "a-" + m["id"]
				return m
			}).
			BuildVarsFunc(func(m map[string]string) map[string]string {
				m["id"] = "b-" + m["id"]
				return m
			})

		u, err := route.URL("id", "42")
		require.NoError(t, err)
		assert.Equal(t, "/users/b-a-42", u.Path)
	})
}

func TestRouteMatchWithBuildVarsFunc(t *testing.T) {
	t.Run("applies buildVarsFunc on match", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			BuildVarsFunc(func(m map[string]string) map[string]string {
				m["id"] = "modified-" + m["id"]
				return m
			})

		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))
		assert.Equal(t, "modified-42", match.Vars["id"])
	})
}

func TestRouteErrorPropagation(t *testing.T) {
	t.Run("Headers skipped when route has error", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/api", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("q"). // odd pairs -> error
			Headers("Content-Type", "application/json")
		assert.Error(t, route.GetError())
	})

	t.Run("HeadersRegexp skipped when route has error", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/api", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("q"). // odd pairs -> error
			HeadersRegexp("Content-Type", "application/.*")
		assert.Error(t, route.GetError())
	})

	t.Run("HeadersRegexp with invalid regex", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/api", func(_ http.ResponseWriter, _ *http.Request) {}).
			HeadersRegexp("Content-Type", "[invalid")
		assert.Error(t, route.GetError())
	})

	t.Run("Queries skipped when route has error", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/search", func(_ http.ResponseWriter, _ *http.Request) {}).
			Headers("odd"). // odd pairs -> error
			Queries("q", "{query}")
		assert.Error(t, route.GetError())
	})
}

func TestRouteInspectionWithErrors(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*Route) (any, error)
	}{
		{
			name: "GetPathTemplate with route error",
			fn:   func(r *Route) (any, error) { return r.GetPathTemplate() },
		},
		{
			name: "GetPathRegexp with route error",
			fn:   func(r *Route) (any, error) { return r.GetPathRegexp() },
		},
		{
			name: "GetHostTemplate with route error",
			fn:   func(r *Route) (any, error) { return r.GetHostTemplate() },
		},
		{
			name: "GetMethods with route error",
			fn:   func(r *Route) (any, error) { return r.GetMethods() },
		},
		{
			name: "GetQueriesTemplates with route error",
			fn:   func(r *Route) (any, error) { return r.GetQueriesTemplates() },
		},
		{
			name: "GetQueriesRegexp with route error",
			fn:   func(r *Route) (any, error) { return r.GetQueriesRegexp() },
		},
		{
			name: "GetVarNames with route error",
			fn:   func(r *Route) (any, error) { return r.GetVarNames() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			errRoute := router.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {}).
				Queries("odd") // odd pairs -> error
			_, err := tt.fn(errRoute)
			assert.Error(t, err)
		})
	}
}

func TestRouteURLWithErrors(t *testing.T) {
	errRoute := func() *Route {
		router := NewRouter()
		return router.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("odd") // odd pairs -> error
	}

	t.Run("URL with route error", func(t *testing.T) {
		_, err := errRoute().URL()
		assert.Error(t, err)
	})

	t.Run("URLHost with route error", func(t *testing.T) {
		_, err := errRoute().URLHost()
		assert.Error(t, err)
	})

	t.Run("URLPath with route error", func(t *testing.T) {
		_, err := errRoute().URLPath()
		assert.Error(t, err)
	})

	t.Run("URL with odd pairs", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("user")
		_, err := route.URL("id")
		assert.Error(t, err)
	})

	t.Run("URLHost with odd pairs", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub}.example.com").Name("host")
		_, err := route.URLHost("sub")
		assert.Error(t, err)
	})

	t.Run("URLPath with odd pairs", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("user")
		_, err := route.URLPath("id")
		assert.Error(t, err)
	})
}

func TestRouteURLWithScheme(t *testing.T) {
	t.Run("URL uses buildScheme from Schemes", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub}.example.com").
			Path("/users/{id}").
			Schemes("https").
			Name("user")

		u, err := route.URL("sub", "api", "id", "42")
		require.NoError(t, err)
		assert.Equal(t, "https", u.Scheme)
	})

	t.Run("URLHost uses buildScheme from Schemes", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub}.example.com").
			Schemes("https").
			Name("host")

		u, err := route.URLHost("sub", "api")
		require.NoError(t, err)
		assert.Equal(t, "https", u.Scheme)
	})
}

func TestRouteGetNamedRoutes(t *testing.T) {
	t.Run("returns named routes map", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).Name("users")
		route := router.routes[0]
		named := route.getNamedRoutes()
		assert.NotNil(t, named)
		assert.Contains(t, named, "users")
	})
}

func TestRouteBuildVarsDirect(t *testing.T) {
	t.Run("returns vars when no buildVarsFunc and no parent", func(t *testing.T) {
		route := &Route{}
		vars := map[string]string{"id": "42"}
		result := route.buildVars(vars)
		assert.Equal(t, "42", result["id"])
	})

	t.Run("applies buildVarsFunc in buildVars", func(t *testing.T) {
		route := &Route{
			buildVarsFunc: func(m map[string]string) map[string]string {
				m["id"] = "modified-" + m["id"]
				return m
			},
		}
		vars := map[string]string{"id": "42"}
		result := route.buildVars(vars)
		assert.Equal(t, "modified-42", result["id"])
	})
}

func TestRouteMatchMethodNotAllowed(t *testing.T) {
	t.Run("sets MatchErr on method mismatch", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		match := &RouteMatch{}
		assert.False(t, router.routes[0].Match(req, match))
		assert.Equal(t, ErrMethodMismatch, match.MatchErr)
	})

	t.Run("does not set MatchErr when path does not match", func(t *testing.T) {
		router := NewRouter()
		router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet)

		req := httptest.NewRequest(http.MethodPost, "/posts", nil)
		match := &RouteMatch{}
		assert.False(t, router.routes[0].Match(req, match))
		assert.Nil(t, match.MatchErr)
	})

	t.Run("returns false when route has error", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {}).
			Queries("odd") // odd pairs -> error

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		match := &RouteMatch{}
		assert.False(t, route.Match(req, match))
	})

	t.Run("returns false when host does not match", func(t *testing.T) {
		router := NewRouter()
		router.Host("example.com").Path("/test").HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})

		req := httptest.NewRequest(http.MethodGet, "http://other.com/test", nil)
		req.Host = "other.com"
		match := &RouteMatch{}
		assert.False(t, router.routes[0].Match(req, match))
	})
}

func TestRouteAddRegexpMatcherHostParent(t *testing.T) {
	t.Run("appends parent host template", func(t *testing.T) {
		router := NewRouter()
		parentRoute := router.Host("example.com")
		sub := parentRoute.Subrouter()
		childRoute := sub.Host("api")
		// Child host "api" should be appended with parent ".example.com"
		tpl, err := childRoute.GetHostTemplate()
		require.NoError(t, err)
		assert.Equal(t, "api.example.com", tpl)
	})
}

func TestRouteInspectionMissingTemplate(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Router) *Route
		fn    func(*Route) (any, error)
	}{
		{
			name:  "GetPathTemplate without path",
			setup: func(r *Router) *Route { return r.Host("example.com") },
			fn:    func(r *Route) (any, error) { return r.GetPathTemplate() },
		},
		{
			name:  "GetPathRegexp without path",
			setup: func(r *Router) *Route { return r.Host("example.com") },
			fn:    func(r *Route) (any, error) { return r.GetPathRegexp() },
		},
		{
			name: "GetHostTemplate without host",
			setup: func(r *Router) *Route {
				return r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
			},
			fn: func(r *Route) (any, error) { return r.GetHostTemplate() },
		},
		{
			name: "GetQueriesTemplates without queries",
			setup: func(r *Router) *Route {
				return r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
			},
			fn: func(r *Route) (any, error) { return r.GetQueriesTemplates() },
		},
		{
			name: "GetQueriesRegexp without queries",
			setup: func(r *Router) *Route {
				return r.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
			},
			fn: func(r *Route) (any, error) { return r.GetQueriesRegexp() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			route := tt.setup(router)
			_, err := tt.fn(route)
			assert.Error(t, err)
		})
	}
}

func TestRouteURLBuildErrors(t *testing.T) {
	t.Run("URL returns error for invalid host variable", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub:[a-z]+}.example.com").
			Path("/users/{id}").
			Name("user")

		_, err := route.URL("sub", "123", "id", "42")
		assert.Error(t, err)
	})

	t.Run("URLHost returns error for invalid host variable", func(t *testing.T) {
		router := NewRouter()
		route := router.Host("{sub:[a-z]+}.example.com").Name("host")

		_, err := route.URLHost("sub", "123")
		assert.Error(t, err)
	})

	t.Run("URLPath returns error for invalid path variable", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users/{id:[0-9]+}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("user")

		_, err := route.URLPath("id", "abc")
		assert.Error(t, err)
	})
}

func TestRouteAddRegexpMatcherUniqueness(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Router) *Route
	}{
		{
			name: "duplicate vars between host and path",
			setup: func(r *Router) *Route {
				return r.Host("{id}.example.com").Path("/users/{id}")
			},
		},
		{
			name: "duplicate vars between path and host",
			setup: func(r *Router) *Route {
				return r.NewRoute().Path("/users/{id}").Host("{id}.example.com")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			route := tt.setup(router)
			assert.Error(t, route.GetError())
		})
	}
}

func TestRouteSchemeMatcher(t *testing.T) {
	tests := []struct {
		name        string
		scheme      string
		url         string
		clearScheme bool
		tls         bool
		expected    bool
	}{
		{
			name:     "matches https via URL scheme",
			scheme:   "https",
			url:      "https://example.com/",
			expected: true,
		},
		{
			name:     "matches http scheme",
			scheme:   "http",
			url:      "http://example.com/",
			expected: true,
		},
		{
			name:     "does not match wrong scheme",
			scheme:   "https",
			url:      "http://example.com/",
			expected: false,
		},
		{
			name:        "falls back to https when TLS is set",
			scheme:      "https",
			url:         "/",
			clearScheme: true,
			tls:         true,
			expected:    true,
		},
		{
			name:        "falls back to http when no TLS",
			scheme:      "http",
			url:         "/",
			clearScheme: true,
			tls:         false,
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := schemeMatcher{tt.scheme}
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			if tt.clearScheme {
				req.URL.Scheme = ""
			}
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			assert.Equal(t, tt.expected, m.Match(req, &RouteMatch{}))
		})
	}
}

func TestRouteDuplicateMethods(t *testing.T) {
	t.Run("second Methods call replaces first", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet).Methods(http.MethodPost)

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		match := &RouteMatch{}
		assert.True(t, route.Match(req, match), "POST should match after Methods(POST) replaces Methods(GET)")

		req = httptest.NewRequest(http.MethodGet, "/users", nil)
		match = &RouteMatch{}
		assert.False(t, route.Match(req, match), "GET should not match after Methods(POST) replaces Methods(GET)")
	})

	t.Run("second Methods call with multiple methods", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet, http.MethodPost).Methods(http.MethodPut, http.MethodPatch)

		req := httptest.NewRequest(http.MethodPut, "/users", nil)
		match := &RouteMatch{}
		assert.True(t, route.Match(req, match), "PUT should match")

		req = httptest.NewRequest(http.MethodPatch, "/users", nil)
		match = &RouteMatch{}
		assert.True(t, route.Match(req, match), "PATCH should match")

		req = httptest.NewRequest(http.MethodGet, "/users", nil)
		match = &RouteMatch{}
		assert.False(t, route.Match(req, match), "GET should not match after replacement")
	})

	t.Run("GetMethods returns methods from last call", func(t *testing.T) {
		router := NewRouter()
		route := router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
			Methods(http.MethodGet).Methods(http.MethodPost, http.MethodPut)

		methods, err := route.GetMethods()
		require.NoError(t, err)
		assert.Equal(t, []string{http.MethodPost, http.MethodPut}, methods)
	})
}

// --- Benchmarks ---

func BenchmarkRouteMatchSimple(b *testing.B) {
	router := NewRouter()
	router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})
	route := router.routes[0]
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	b.ResetTimer()
	for b.Loop() {
		route.Match(req, &RouteMatch{})
	}
}

func BenchmarkRouteMatchWithVars(b *testing.B) {
	router := NewRouter()
	router.HandleFunc("/users/{id:[0-9]+}/posts/{pid}", func(_ http.ResponseWriter, _ *http.Request) {})
	route := router.routes[0]
	req := httptest.NewRequest(http.MethodGet, "/users/42/posts/123", nil)
	b.ResetTimer()
	for b.Loop() {
		route.Match(req, &RouteMatch{})
	}
}

func BenchmarkRouteMatchWithMethod(b *testing.B) {
	router := NewRouter()
	router.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {}).
		Methods(http.MethodGet, http.MethodPost)
	route := router.routes[0]
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	b.ResetTimer()
	for b.Loop() {
		route.Match(req, &RouteMatch{})
	}
}

func BenchmarkRouteMatchWithHost(b *testing.B) {
	router := NewRouter()
	router.Host("{sub}.example.com").Path("/users/{id}").
		HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	route := router.routes[0]
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/users/42", nil)
	req.Host = "api.example.com"
	b.ResetTimer()
	for b.Loop() {
		route.Match(req, &RouteMatch{})
	}
}

func BenchmarkRouteURLBuilding(b *testing.B) {
	router := NewRouter()
	route := router.Host("{sub}.example.com").
		Path("/users/{id}/posts/{pid}").
		Name("user-post")
	b.ResetTimer()
	for b.Loop() {
		route.URL("sub", "api", "id", "42", "pid", "123") //nolint:errcheck
	}
}

func BenchmarkRouteURLPath(b *testing.B) {
	router := NewRouter()
	route := router.HandleFunc("/users/{id:[0-9]+}/posts/{pid}", func(_ http.ResponseWriter, _ *http.Request) {}).
		Name("user-post")
	b.ResetTimer()
	for b.Loop() {
		route.URLPath("id", "42", "pid", "123") //nolint:errcheck
	}
}

func TestRouteSubrouter(t *testing.T) {
	t.Run("creates subrouter", func(t *testing.T) {
		router := NewRouter()
		sub := router.PathPrefix("/api").Subrouter()
		require.NotNil(t, sub)

		sub.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))
	})

	t.Run("subrouter does not match outside prefix", func(t *testing.T) {
		router := NewRouter()
		sub := router.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users", func(_ http.ResponseWriter, _ *http.Request) {})

		req := httptest.NewRequest(http.MethodGet, "/web/users", nil)
		match := &RouteMatch{}
		assert.False(t, router.Match(req, match))
	})

	t.Run("subrouter extracts variables", func(t *testing.T) {
		router := NewRouter()
		sub := router.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {})

		req := httptest.NewRequest(http.MethodGet, "/api/users/42", nil)
		match := &RouteMatch{}
		assert.True(t, router.Match(req, match))
		assert.Equal(t, "42", match.Vars["id"])
	})

	t.Run("subrouter shares named routes", func(t *testing.T) {
		router := NewRouter()
		sub := router.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users/{id}", func(_ http.ResponseWriter, _ *http.Request) {}).
			Name("api-user")

		route := router.Get("api-user")
		require.NotNil(t, route)
		assert.Equal(t, "api-user", route.GetName())
	})

	t.Run("subrouter inherits strictSlash", func(t *testing.T) {
		router := NewRouter()
		router.StrictSlash(true)
		sub := router.PathPrefix("/api").Subrouter()
		assert.True(t, sub.strictSlash)
	})

	t.Run("subrouter inherits skipClean", func(t *testing.T) {
		router := NewRouter()
		router.SkipClean(true)
		sub := router.PathPrefix("/api").Subrouter()
		assert.True(t, sub.skipClean)
	})

	t.Run("subrouter inherits useEncodedPath", func(t *testing.T) {
		router := NewRouter()
		router.UseEncodedPath()
		sub := router.PathPrefix("/api").Subrouter()
		assert.True(t, sub.useEncodedPath)
	})

	t.Run("subrouter strictSlash redirect works", func(t *testing.T) {
		router := NewRouter()
		router.StrictSlash(true)
		sub := router.PathPrefix("/api").Subrouter()
		sub.HandleFunc("/users/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusPermanentRedirect, w.Code)
	})
}
