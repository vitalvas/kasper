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
	t.Run("config validation", func(t *testing.T) {
		r := newTestRouter()

		t.Run("wildcard origin with credentials", func(t *testing.T) {
			_, err := CORSMiddleware(r, CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true,
			})
			assert.ErrorIs(t, err, ErrWildcardCredentials)
		})

		t.Run("multiple wildcards in pattern", func(t *testing.T) {
			_, err := CORSMiddleware(r, CORSConfig{AllowedOrigins: []string{"https://*.*"}})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "multiple wildcards")
		})
	})

	type corsTest struct {
		name               string
		config             CORSConfig
		method             string
		headers            map[string]string
		wantCode           int
		wantHeaders        map[string]string // exact match; "" = assert empty
		wantVary           []string
		wantNotVary        []string
		wantMethodsContain []string
	}

	tests := []corsTest{
		// Actual request: origin matching
		{
			name:     "allows matching origin",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://example.com",
			},
		},
		{
			name:     "blocks non-matching origin",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://evil.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "",
			},
		},
		{
			name:     "allows wildcard origin",
			config:   CORSConfig{AllowedOrigins: []string{"*"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://any.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "*",
			},
		},
		{
			name: "allows origin via AllowOriginFunc",
			config: CORSConfig{
				AllowOriginFunc: func(origin string) bool {
					return strings.HasSuffix(origin, ".example.com")
				},
			},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://sub.example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://sub.example.com",
			},
		},
		{
			name:     "skips CORS headers when no Origin header",
			config:   CORSConfig{AllowedOrigins: []string{"*"}},
			method:   http.MethodGet,
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "",
			},
		},
		{
			name:     "reflects specific origin with multiple allowed",
			config:   CORSConfig{AllowedOrigins: []string{"https://a.com", "https://b.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://b.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://b.com",
			},
		},

		// Actual request: credentials
		{
			name:     "sets Allow-Credentials when configured",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}, AllowCredentials: true},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Credentials": "true",
			},
		},
		{
			name:     "does not set Allow-Credentials when not configured",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Credentials": "",
			},
		},
		{
			name: "allows credentials with AllowOriginFunc",
			config: CORSConfig{
				AllowOriginFunc:  func(_ string) bool { return true },
				AllowCredentials: true,
			},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin":      "https://example.com",
				"Access-Control-Allow-Credentials": "true",
			},
		},

		// Actual request: expose headers
		{
			name:     "sets Expose-Headers on actual request",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}, ExposeHeaders: []string{"X-Request-Id", "X-Total-Count"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Expose-Headers": "X-Request-Id,X-Total-Count",
			},
		},
		{
			name:     "does not set Expose-Headers when empty",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Expose-Headers": "",
			},
		},

		// Actual request: Vary header
		{
			name:     "adds Vary Origin for reflected origin",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://example.com"},
			wantCode: http.StatusOK,
			wantVary: []string{"Origin"},
		},
		{
			name:        "does not add Vary Origin for wildcard",
			config:      CORSConfig{AllowedOrigins: []string{"*"}},
			method:      http.MethodGet,
			headers:     map[string]string{"Origin": "https://example.com"},
			wantCode:    http.StatusOK,
			wantNotVary: []string{"Origin"},
		},

		// Actual request: origin case normalization
		{
			name:     "matches origin case-insensitively",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "HTTPS://EXAMPLE.COM"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "HTTPS://EXAMPLE.COM",
			},
		},
		{
			name:     "reflects original origin casing in response header",
			config:   CORSConfig{AllowedOrigins: []string{"https://EXAMPLE.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://Example.COM"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://Example.COM",
			},
		},

		// Actual request: subdomain wildcard patterns
		{
			name:     "matches subdomain wildcard pattern",
			config:   CORSConfig{AllowedOrigins: []string{"https://*.example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://app.example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://app.example.com",
			},
		},
		{
			name:     "matches deep subdomain wildcard pattern",
			config:   CORSConfig{AllowedOrigins: []string{"https://*.example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://a.b.example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://a.b.example.com",
			},
		},
		{
			name:     "rejects origin not matching wildcard pattern",
			config:   CORSConfig{AllowedOrigins: []string{"https://*.example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://evil.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "",
			},
		},
		{
			name:     "mixes exact and wildcard origins with exact match",
			config:   CORSConfig{AllowedOrigins: []string{"https://exact.com", "https://*.example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://exact.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://exact.com",
			},
		},
		{
			name:     "mixes exact and wildcard origins with wildcard match",
			config:   CORSConfig{AllowedOrigins: []string{"https://exact.com", "https://*.example.com"}},
			method:   http.MethodGet,
			headers:  map[string]string{"Origin": "https://app.example.com"},
			wantCode: http.StatusOK,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Origin": "https://app.example.com",
			},
		},

		// Vary on non-CORS requests
		{
			name:     "Vary Origin on non-CORS request with specific origins",
			config:   CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method:   http.MethodGet,
			wantCode: http.StatusOK,
			wantVary: []string{"Origin"},
		},
		{
			name:        "no Vary Origin on non-CORS request with wildcard only",
			config:      CORSConfig{AllowedOrigins: []string{"*"}},
			method:      http.MethodGet,
			wantCode:    http.StatusOK,
			wantNotVary: []string{"Origin"},
		},
		{
			name:     "Vary Origin on non-CORS request with AllowOriginFunc",
			config:   CORSConfig{AllowOriginFunc: func(_ string) bool { return true }},
			method:   http.MethodGet,
			wantCode: http.StatusOK,
			wantVary: []string{"Origin"},
		},
		{
			name:     "Vary Origin on non-CORS request with wildcard pattern",
			config:   CORSConfig{AllowedOrigins: []string{"https://*.example.com"}},
			method:   http.MethodGet,
			wantCode: http.StatusOK,
			wantVary: []string{"Origin"},
		},

		// Preflight: methods
		{
			name: "sets Allow-Methods from config on preflight",
			config: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
				AllowedMethods: []string{http.MethodGet, http.MethodPut, http.MethodDelete},
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "PUT",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Methods": "GET,PUT,DELETE",
			},
		},
		{
			name:   "auto-discovers methods on preflight when AllowedMethods empty",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode:           http.StatusNoContent,
			wantMethodsContain: []string{http.MethodGet, http.MethodPost},
		},

		// Preflight: headers
		{
			name: "sets Allow-Headers from config on preflight",
			config: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
				AllowedHeaders: []string{"Content-Type", "Authorization"},
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Headers": "Content-Type,Authorization",
			},
		},
		{
			name:   "reflects request headers on preflight when AllowedHeaders empty",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         "https://example.com",
				"Access-Control-Request-Method":  "POST",
				"Access-Control-Request-Headers": "X-Custom, X-Other",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Headers": "X-Custom, X-Other",
			},
		},
		{
			name: "reflects request headers when AllowedHeaders contains wildcard",
			config: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
				AllowedHeaders: []string{"*"},
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         "https://example.com",
				"Access-Control-Request-Method":  "POST",
				"Access-Control-Request-Headers": "X-Custom, X-Other",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Headers": "X-Custom, X-Other",
			},
		},
		{
			name: "AllowedHeaders wildcard with no request headers",
			config: CORSConfig{
				AllowedOrigins: []string{"https://example.com"},
				AllowedHeaders: []string{"*"},
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Headers": "",
			},
		},

		// Preflight: Max-Age
		{
			name:   "sets Max-Age on preflight",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}, MaxAge: 3600},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Max-Age": "3600",
			},
		},
		{
			name:   "sends Max-Age 0 for negative config",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}, MaxAge: -1},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Max-Age": "0",
			},
		},
		{
			name:   "omits Max-Age when zero",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Max-Age": "",
			},
		},

		// Preflight: status code
		{
			name: "uses custom OptionsStatusCode for preflight",
			config: CORSConfig{
				AllowedOrigins:    []string{"https://example.com"},
				OptionsStatusCode: http.StatusOK,
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "defaults to 204 when OptionsStatusCode is zero",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
		},

		// Preflight: Vary headers
		{
			name:   "adds Vary preflight headers on preflight",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
			wantVary: []string{"Access-Control-Request-Method", "Access-Control-Request-Headers"},
		},

		// Preflight: private network access
		{
			name: "sets Allow-Private-Network on preflight when enabled",
			config: CORSConfig{
				AllowedOrigins:      []string{"https://example.com"},
				AllowPrivateNetwork: true,
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                                 "https://example.com",
				"Access-Control-Request-Method":          "POST",
				"Access-Control-Request-Private-Network": "true",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Private-Network": "true",
			},
			wantVary: []string{"Access-Control-Request-Private-Network"},
		},
		{
			name:   "does not set Allow-Private-Network when disabled",
			config: CORSConfig{AllowedOrigins: []string{"https://example.com"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                                 "https://example.com",
				"Access-Control-Request-Method":          "POST",
				"Access-Control-Request-Private-Network": "true",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Private-Network": "",
			},
			wantNotVary: []string{"Access-Control-Request-Private-Network"},
		},
		{
			name: "does not set Allow-Private-Network when request header absent",
			config: CORSConfig{
				AllowedOrigins:      []string{"https://example.com"},
				AllowPrivateNetwork: true,
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://example.com",
				"Access-Control-Request-Method": "POST",
			},
			wantCode: http.StatusNoContent,
			wantHeaders: map[string]string{
				"Access-Control-Allow-Private-Network": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter()
			mw, err := CORSMiddleware(r, tt.config)
			require.NoError(t, err)
			r.Use(mw)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/users", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			for k, want := range tt.wantHeaders {
				if want == "" {
					assert.Empty(t, w.Header().Get(k), k)
				} else {
					assert.Equal(t, want, w.Header().Get(k), k)
				}
			}
			for _, m := range tt.wantMethodsContain {
				assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), m)
			}
			for _, v := range tt.wantVary {
				assert.Contains(t, w.Header().Values("Vary"), v)
			}
			for _, v := range tt.wantNotVary {
				assert.NotContains(t, w.Header().Values("Vary"), v)
			}
		})
	}

	// Handler chain behavioral tests require custom router setup to track
	// whether the next handler in the chain was called.
	t.Run("handler chain", func(t *testing.T) {
		chainTests := []struct {
			name              string
			config            CORSConfig
			methods           []string
			method            string
			headers           map[string]string
			wantHandlerCalled bool
			wantCode          int
		}{
			{
				name:    "preflight stops chain",
				config:  CORSConfig{AllowedOrigins: []string{"https://example.com"}},
				methods: []string{http.MethodGet, http.MethodPost},
				method:  http.MethodOptions,
				headers: map[string]string{
					"Origin":                        "https://example.com",
					"Access-Control-Request-Method": "POST",
				},
				wantHandlerCalled: false,
				wantCode:          http.StatusNoContent,
			},
			{
				name:    "OPTIONS without ACRM is not preflight",
				config:  CORSConfig{AllowedOrigins: []string{"https://example.com"}},
				methods: []string{http.MethodOptions},
				method:  http.MethodOptions,
				headers: map[string]string{
					"Origin": "https://example.com",
				},
				wantHandlerCalled: true,
				wantCode:          http.StatusOK,
			},
			{
				name: "OptionsPassthrough forwards preflight to next handler",
				config: CORSConfig{
					AllowedOrigins:     []string{"https://example.com"},
					OptionsPassthrough: true,
				},
				methods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
				method:  http.MethodOptions,
				headers: map[string]string{
					"Origin":                        "https://example.com",
					"Access-Control-Request-Method": "POST",
				},
				wantHandlerCalled: true,
				wantCode:          http.StatusOK,
			},
			{
				name: "OptionsPassthrough false stops chain on preflight",
				config: CORSConfig{
					AllowedOrigins:     []string{"https://example.com"},
					OptionsPassthrough: false,
				},
				methods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
				method:  http.MethodOptions,
				headers: map[string]string{
					"Origin":                        "https://example.com",
					"Access-Control-Request-Method": "POST",
				},
				wantHandlerCalled: false,
				wantCode:          http.StatusNoContent,
			},
		}

		for _, tt := range chainTests {
			t.Run(tt.name, func(t *testing.T) {
				handlerCalled := false
				r := mux.NewRouter()
				r.HandleFunc("/users", func(w http.ResponseWriter, _ *http.Request) {
					handlerCalled = true
					w.WriteHeader(http.StatusOK)
				}).Methods(tt.methods...)

				mw, err := CORSMiddleware(r, tt.config)
				require.NoError(t, err)
				r.Use(mw)

				w := httptest.NewRecorder()
				req := httptest.NewRequest(tt.method, "/users", nil)
				for k, v := range tt.headers {
					req.Header.Set(k, v)
				}
				r.ServeHTTP(w, req)

				assert.Equal(t, tt.wantHandlerCalled, handlerCalled)
				assert.Equal(t, tt.wantCode, w.Code)
			})
		}
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
