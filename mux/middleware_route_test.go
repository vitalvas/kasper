package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouterWith(t *testing.T) {
	t.Run("applies middleware to single route", func(t *testing.T) {
		r := NewRouter()
		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "applied")
				next.ServeHTTP(w, req)
			})
		}
		r.With(mw).HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "ok", w.Body.String())
		assert.Equal(t, "applied", w.Header().Get("X-MW"))
	})

	t.Run("middleware does not affect direct routes", func(t *testing.T) {
		r := NewRouter()
		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "applied")
				next.ServeHTTP(w, req)
			})
		}
		r.With(mw).HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "protected")
		})
		r.HandleFunc("/public", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "public")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/public", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "public", w.Body.String())
		assert.Empty(t, w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "protected", w.Body.String())
		assert.Equal(t, "applied", w.Header().Get("X-MW"))
	})

	t.Run("multiple middleware applied in order", func(t *testing.T) {
		r := NewRouter()
		var order []string
		mw1 := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "mw1")
				next.ServeHTTP(w, req)
			})
		})
		mw2 := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "mw2")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw1, mw2).HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"mw1", "mw2", "handler"}, order)
	})

	t.Run("works with path variables", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "yes")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).HandleFunc("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
			fmt.Fprint(w, "user:"+Vars(req)["id"])
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "user:42", w.Body.String())
		assert.Equal(t, "yes", w.Header().Get("X-MW"))
	})

	t.Run("works with Handle", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "handle")
				next.ServeHTTP(w, req)
			})
		})
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "handled")
		})
		r.With(mw).Handle("/test", h)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "handled", w.Body.String())
		assert.Equal(t, "handle", w.Header().Get("X-MW"))
	})

	t.Run("works with Methods", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "methods")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Methods(http.MethodPost).Path("/submit").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "submitted")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/submit", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "submitted", w.Body.String())
		assert.Equal(t, "methods", w.Header().Get("X-MW"))
	})

	t.Run("works with route-registration methods", func(t *testing.T) {
		tests := []struct {
			name     string
			register func(*MiddlewareRoute)
			reqURL   string
			reqSetup func(*http.Request)
			wantBody string
		}{
			{
				name: "Headers",
				register: func(mr *MiddlewareRoute) {
					mr.Headers("X-Token", "secret").Path("/api").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						fmt.Fprint(w, "api")
					})
				},
				reqURL:   "/api",
				reqSetup: func(req *http.Request) { req.Header.Set("X-Token", "secret") },
				wantBody: "api",
			},
			{
				name: "HeadersRegexp",
				register: func(mr *MiddlewareRoute) {
					mr.HeadersRegexp("Content-Type", "application/.*").Path("/data").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						fmt.Fprint(w, "data")
					})
				},
				reqURL:   "/data",
				reqSetup: func(req *http.Request) { req.Header.Set("Content-Type", "application/json") },
				wantBody: "data",
			},
			{
				name: "Path",
				register: func(mr *MiddlewareRoute) {
					mr.Path("/items").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						fmt.Fprint(w, "items")
					})
				},
				reqURL:   "/items",
				wantBody: "items",
			},
			{
				name: "PathPrefix",
				register: func(mr *MiddlewareRoute) {
					mr.PathPrefix("/api/").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						fmt.Fprint(w, "prefixed")
					})
				},
				reqURL:   "/api/users",
				wantBody: "prefixed",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				r := NewRouter()
				mw := MiddlewareFunc(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Header().Set("X-MW", tt.name)
						next.ServeHTTP(w, req)
					})
				})
				tt.register(r.With(mw))

				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, tt.reqURL, nil)
				if tt.reqSetup != nil {
					tt.reqSetup(req)
				}
				r.ServeHTTP(w, req)
				assert.Equal(t, tt.wantBody, w.Body.String())
				assert.Equal(t, tt.name, w.Header().Get("X-MW"))
			})
		}
	})

	t.Run("works with Queries", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "queries")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Queries("key", "val").Path("/search").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "found")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/search?key=val", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "found", w.Body.String())
		assert.Equal(t, "queries", w.Header().Get("X-MW"))
	})

	t.Run("works with Schemes", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "schemes")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Schemes("https").Path("/secure").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "secure")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "https://example.com/secure", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "secure", w.Body.String())
		assert.Equal(t, "schemes", w.Header().Get("X-MW"))
	})

	t.Run("works with Host", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "host")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Host("example.com").Path("/test").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "host")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		req.Host = "example.com"
		r.ServeHTTP(w, req)
		assert.Equal(t, "host", w.Body.String())
		assert.Equal(t, "host", w.Header().Get("X-MW"))
	})

	t.Run("works with MatcherFunc", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "matcher")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).MatcherFunc(func(req *http.Request, _ *RouteMatch) bool {
			return req.Header.Get("X-Custom") == "yes"
		}).Path("/custom").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "custom")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/custom", nil)
		req.Header.Set("X-Custom", "yes")
		r.ServeHTTP(w, req)
		assert.Equal(t, "custom", w.Body.String())
		assert.Equal(t, "matcher", w.Header().Get("X-MW"))
	})

	t.Run("works with Name", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "named")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Name("myroute").Path("/named").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "named")
		})

		route := r.Get("myroute")
		assert.NotNil(t, route)
		assert.Equal(t, "myroute", route.GetName())

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/named", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "named", w.Body.String())
		assert.Equal(t, "named", w.Header().Get("X-MW"))
	})

	t.Run("works with NewRoute", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "new")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).NewRoute().Path("/raw").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "raw")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/raw", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "raw", w.Body.String())
		assert.Equal(t, "new", w.Header().Get("X-MW"))
	})

	t.Run("combined with router-level middleware", func(t *testing.T) {
		r := NewRouter()
		var order []string
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "router")
				next.ServeHTTP(w, req)
			})
		})
		routeMW := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "route")
				next.ServeHTTP(w, req)
			})
		})
		r.With(routeMW).HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"router", "route", "handler"}, order)
	})

	t.Run("middleware can short-circuit", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(_ http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			})
		})
		r.With(mw).HandleFunc("/blocked", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "should not reach")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/blocked", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, w.Body.String())
	})

	t.Run("defensively copies middleware slice", func(t *testing.T) {
		r := NewRouter()
		original := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "original")
				next.ServeHTTP(w, req)
			})
		})
		replaced := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "replaced")
				next.ServeHTTP(w, req)
			})
		})

		slice := []MiddlewareFunc{original}
		mr := r.With(slice...)
		slice[0] = replaced // mutate caller's slice after With()

		mr.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "ok")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "ok", w.Body.String())
		assert.Equal(t, "original", w.Header().Get("X-MW"))
	})

	t.Run("chained With calls apply middleware in order", func(t *testing.T) {
		r := NewRouter()
		var order []string
		mw1 := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "mw1")
				next.ServeHTTP(w, req)
			})
		})
		mw2 := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "mw2")
				next.ServeHTTP(w, req)
			})
		})
		mw3 := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "mw3")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw1).With(mw2).With(mw3).HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"mw1", "mw2", "mw3", "handler"}, order)
	})

	t.Run("chained With does not affect parent MiddlewareRoute", func(t *testing.T) {
		r := NewRouter()
		base := r.With(MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Base", "yes")
				next.ServeHTTP(w, req)
			})
		}))
		extended := base.With(MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Extra", "yes")
				next.ServeHTTP(w, req)
			})
		}))

		base.HandleFunc("/base", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "base")
		})
		extended.HandleFunc("/extended", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "extended")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/base", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "base", w.Body.String())
		assert.Equal(t, "yes", w.Header().Get("X-Base"))
		assert.Empty(t, w.Header().Get("X-Extra"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/extended", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "extended", w.Body.String())
		assert.Equal(t, "yes", w.Header().Get("X-Base"))
		assert.Equal(t, "yes", w.Header().Get("X-Extra"))
	})

	t.Run("With and Use combined execute in order", func(t *testing.T) {
		r := NewRouter()
		var order []string
		withMW := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "with")
				next.ServeHTTP(w, req)
			})
		})
		useMW := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "use")
				next.ServeHTTP(w, req)
			})
		})

		r.With(withMW).HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		}).Use(useMW)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"with", "use", "handler"}, order)
	})

	t.Run("With and Use combined with router-level middleware", func(t *testing.T) {
		r := NewRouter()
		var order []string
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "router")
				next.ServeHTTP(w, req)
			})
		})
		withMW := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "with")
				next.ServeHTTP(w, req)
			})
		})
		useMW := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "use")
				next.ServeHTTP(w, req)
			})
		})

		r.With(withMW).HandleFunc("/test", func(_ http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
		}).Use(useMW)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"router", "with", "use", "handler"}, order)
	})

	t.Run("Use on With route does not affect other With routes", func(t *testing.T) {
		r := NewRouter()
		mr := r.With(MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-With", "yes")
				next.ServeHTTP(w, req)
			})
		}))

		mr.HandleFunc("/first", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "first")
		}).Use(MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-Use", "first-only")
				next.ServeHTTP(w, req)
			})
		}))

		mr.HandleFunc("/second", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "second")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/first", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "first", w.Body.String())
		assert.Equal(t, "yes", w.Header().Get("X-With"))
		assert.Equal(t, "first-only", w.Header().Get("X-Use"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/second", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "second", w.Body.String())
		assert.Equal(t, "yes", w.Header().Get("X-With"))
		assert.Empty(t, w.Header().Get("X-Use"))
	})

	t.Run("With combined with Route", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "with-route")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Route("/api", func(sub *Router) {
			sub.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "users")
			})
			sub.HandleFunc("/posts", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "posts")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "users", w.Body.String())
		assert.Equal(t, "with-route", w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/posts", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "posts", w.Body.String())
		assert.Equal(t, "with-route", w.Header().Get("X-MW"))
	})

	t.Run("With combined with Route does not affect other routes", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "scoped")
				next.ServeHTTP(w, req)
			})
		})
		r.HandleFunc("/public", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "public")
		})
		r.With(mw).Route("/admin", func(sub *Router) {
			sub.HandleFunc("/dashboard", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "dashboard")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "dashboard", w.Body.String())
		assert.Equal(t, "scoped", w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/public", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "public", w.Body.String())
		assert.Empty(t, w.Header().Get("X-MW"))
	})

	t.Run("With combined with Group", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "with-group")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Group(func(sub *Router) {
			sub.HandleFunc("/dashboard", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "dashboard")
			})
			sub.HandleFunc("/settings", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "settings")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "dashboard", w.Body.String())
		assert.Equal(t, "with-group", w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/settings", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "settings", w.Body.String())
		assert.Equal(t, "with-group", w.Header().Get("X-MW"))
	})

	t.Run("With combined with Group does not affect other routes", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "grouped")
				next.ServeHTTP(w, req)
			})
		})
		r.HandleFunc("/public", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "public")
		})
		r.With(mw).Group(func(sub *Router) {
			sub.HandleFunc("/private", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "private")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "private", w.Body.String())
		assert.Equal(t, "grouped", w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/public", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "public", w.Body.String())
		assert.Empty(t, w.Header().Get("X-MW"))
	})

	t.Run("With Route chaining returns MiddlewareRoute", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "chained")
				next.ServeHTTP(w, req)
			})
		})
		r.With(mw).Route("/api", func(sub *Router) {
			sub.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "users")
			})
		}).Route("/admin", func(sub *Router) {
			sub.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "stats")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "users", w.Body.String())
		assert.Equal(t, "chained", w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "stats", w.Body.String())
		assert.Equal(t, "chained", w.Header().Get("X-MW"))
	})

	t.Run("With inside Route callback", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "inner-with")
				next.ServeHTTP(w, req)
			})
		})
		r.Route("/api", func(sub *Router) {
			sub.With(mw).HandleFunc("/protected", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "protected")
			})
			sub.HandleFunc("/open", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "open")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "protected", w.Body.String())
		assert.Equal(t, "inner-with", w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/api/open", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "open", w.Body.String())
		assert.Empty(t, w.Header().Get("X-MW"))
	})

	t.Run("With inside Group callback", func(t *testing.T) {
		r := NewRouter()
		mw := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("X-MW", "inner-with")
				next.ServeHTTP(w, req)
			})
		})
		r.Group(func(sub *Router) {
			sub.With(mw).HandleFunc("/guarded", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "guarded")
			})
			sub.HandleFunc("/free", func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, "free")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "guarded", w.Body.String())
		assert.Equal(t, "inner-with", w.Header().Get("X-MW"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/free", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, "free", w.Body.String())
		assert.Empty(t, w.Header().Get("X-MW"))
	})

	t.Run("With Route Group and Use all combined", func(t *testing.T) {
		r := NewRouter()
		var order []string
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "router")
				next.ServeHTTP(w, req)
			})
		})
		withMW := MiddlewareFunc(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, "with")
				next.ServeHTTP(w, req)
			})
		})
		r.With(withMW).Route("/api", func(sub *Router) {
			sub.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					order = append(order, "sub-use")
					next.ServeHTTP(w, req)
				})
			})
			sub.HandleFunc("/data", func(_ http.ResponseWriter, _ *http.Request) {
				order = append(order, "handler")
			})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, []string{"router", "with", "sub-use", "handler"}, order)
	})
}
