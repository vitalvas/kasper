package muxhandlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
)

func TestNELMiddleware(t *testing.T) {
	validGroup := ReportToGroup{
		Group:  "nel",
		MaxAge: 86400,
		Endpoints: []ReportToEndpoint{
			{URL: "https://reports.example.com/nel"},
		},
	}

	t.Run("default group name", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/api/v1/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		mw, err := NELMiddleware(r, NELConfig{
			MaxAge:         3600,
			ReportToGroups: []ReportToGroup{validGroup},
		})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var payload nelPayload
		require.NoError(t, json.Unmarshal([]byte(w.Header().Get("NEL")), &payload))
		assert.Equal(t, "nel", payload.ReportTo)
		assert.Equal(t, 3600, payload.MaxAge)
		assert.False(t, payload.IncludeSubdomains)
		assert.Equal(t, 0.0, payload.SuccessFraction)
		assert.Equal(t, 0.0, payload.FailureFraction)

		reportTo := w.Header().Get("Report-To")
		assert.Contains(t, reportTo, `"group":"nel"`)
		assert.Contains(t, reportTo, `"max_age":86400`)
		assert.Contains(t, reportTo, `"url":"https://reports.example.com/nel"`)
	})

	t.Run("custom group name", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge:   3600,
			ReportTo: "default",
			ReportToGroups: []ReportToGroup{{
				Group:  "default",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/default"},
				},
			}},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		var payload nelPayload
		require.NoError(t, json.Unmarshal([]byte(w.Header().Get("NEL")), &payload))
		assert.Equal(t, "default", payload.ReportTo)
	})

	t.Run("include subdomains and sampling fractions", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge:            3600,
			IncludeSubdomains: true,
			SuccessFraction:   0.5,
			FailureFraction:   1.0,
			ReportToGroups:    []ReportToGroup{validGroup},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		var payload nelPayload
		require.NoError(t, json.Unmarshal([]byte(w.Header().Get("NEL")), &payload))
		assert.True(t, payload.IncludeSubdomains)
		assert.Equal(t, 0.5, payload.SuccessFraction)
		assert.Equal(t, 1.0, payload.FailureFraction)
	})

	t.Run("multiple report-to groups", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{
				{
					Group:  "nel",
					MaxAge: 86400,
					Endpoints: []ReportToEndpoint{
						{URL: "https://reports.example.com/nel"},
					},
				},
				{
					Group:             "csp",
					MaxAge:            7200,
					IncludeSubdomains: true,
					Endpoints: []ReportToEndpoint{
						{URL: "https://reports.example.com/csp", Priority: 1, Weight: 1},
						{URL: "https://reports-backup.example.com/csp", Priority: 2, Weight: 1},
					},
				},
			},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		reportTo := w.Header().Get("Report-To")
		parts := strings.Split(reportTo, ", ")
		require.Len(t, parts, 2)

		var first, second ReportToGroup
		require.NoError(t, json.Unmarshal([]byte(parts[0]), &first))
		require.NoError(t, json.Unmarshal([]byte(parts[1]), &second))

		assert.Equal(t, "nel", first.Group)
		assert.Equal(t, "csp", second.Group)
		assert.True(t, second.IncludeSubdomains)
		assert.Len(t, second.Endpoints, 2)
		assert.Equal(t, 1, second.Endpoints[0].Priority)
		assert.Equal(t, 2, second.Endpoints[1].Priority)
	})

	t.Run("headers set on every response", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge:         3600,
			ReportToGroups: []ReportToGroup{validGroup},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet, http.MethodPost)
		r.Use(mw)

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(method, "/test", nil))

			assert.NotEmpty(t, w.Header().Get("NEL"))
			assert.NotEmpty(t, w.Header().Get("Report-To"))
		}
	})

	t.Run("zero max-age returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			ReportToGroups: []ReportToGroup{validGroup},
		})
		assert.ErrorIs(t, err, ErrNELMaxAgeNotPositive)
	})

	t.Run("negative max-age returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge:         -1,
			ReportToGroups: []ReportToGroup{validGroup},
		})
		assert.ErrorIs(t, err, ErrNELMaxAgeNotPositive)
	})

	t.Run("success fraction out of range returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge:          3600,
			SuccessFraction: 1.5,
			ReportToGroups:  []ReportToGroup{validGroup},
		})
		assert.ErrorIs(t, err, ErrNELSuccessFractionOutOfRange)

		_, err = NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge:          3600,
			SuccessFraction: -0.1,
			ReportToGroups:  []ReportToGroup{validGroup},
		})
		assert.ErrorIs(t, err, ErrNELSuccessFractionOutOfRange)
	})

	t.Run("failure fraction out of range returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge:          3600,
			FailureFraction: 2.0,
			ReportToGroups:  []ReportToGroup{validGroup},
		})
		assert.ErrorIs(t, err, ErrNELFailureFractionOutOfRange)

		_, err = NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge:          3600,
			FailureFraction: -0.5,
			ReportToGroups:  []ReportToGroup{validGroup},
		})
		assert.ErrorIs(t, err, ErrNELFailureFractionOutOfRange)
	})

	t.Run("no report-to groups returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
		})
		assert.ErrorIs(t, err, ErrNELNoReportToGroups)
	})

	t.Run("empty group name returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/nel"},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELReportToGroupName)
	})

	t.Run("duplicate group name returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{
				validGroup,
				validGroup,
			},
		})
		assert.ErrorIs(t, err, ErrNELReportToGroupNameDuplicate)
	})

	t.Run("group max-age not positive returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 0,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/nel"},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELReportToGroupMaxAge)
	})

	t.Run("no endpoints returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
			}},
		})
		assert.ErrorIs(t, err, ErrNELReportToGroupNoEndpoints)
	})

	t.Run("invalid endpoint URL returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: ""},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointURL)

		_, err = NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "not-a-url"},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointURL)

		_, err = NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://"},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointURL)
	})

	t.Run("non-https endpoint URL returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "http://reports.example.com/nel"},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointScheme)
	})

	t.Run("negative endpoint priority returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/nel", Priority: -1},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointPriority)
	})

	t.Run("negative endpoint weight returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/nel", Weight: -1},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointWeight)
	})

	t.Run("report-to group missing returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge:   3600,
			ReportTo: "missing-group",
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/nel"},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELReportToGroupMissing)
	})

	t.Run("default group must exist when ReportTo is empty", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "custom",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/custom"},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELReportToGroupMissing)
	})

	t.Run("endpoint priority and weight serialized", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{URL: "https://reports.example.com/nel", Priority: 3, Weight: 5},
				},
			}},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		reportTo := w.Header().Get("Report-To")
		assert.Contains(t, reportTo, `"priority":3`)
		assert.Contains(t, reportTo, `"weight":5`)
	})

	t.Run("zero sampling fractions are omitted", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge:         3600,
			ReportToGroups: []ReportToGroup{validGroup},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		nel := w.Header().Get("NEL")
		assert.NotContains(t, nel, "success_fraction")
		assert.NotContains(t, nel, "failure_fraction")
	})
}

func TestNELMiddlewareDynamicEndpoints(t *testing.T) {
	t.Run("URLFunc replaces static URL per request", func(t *testing.T) {
		r := mux.NewRouter()
		var gotRouter *mux.Router
		var calls int
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{{
					URL: "https://reports.example.com/static",
					URLFunc: func(router *mux.Router, req *http.Request) string {
						calls++
						gotRouter = router
						u := url.URL{Scheme: "https", Host: "reports.example.com", Path: "/dynamic"}
						q := u.Query()
						q.Set("path", req.URL.Path)
						u.RawQuery = q.Encode()
						return u.String()
					},
				}},
			}},
		})
		require.NoError(t, err)

		r.HandleFunc("/api/v1/users", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))

		assert.Equal(t, 1, calls)
		assert.Same(t, r, gotRouter)
		reportTo := w.Header().Get("Report-To")
		assert.Contains(t, reportTo, `"url":"https://reports.example.com/dynamic?path=%2Fapi%2Fv1%2Fusers"`)
		assert.NotContains(t, reportTo, "static")
	})

	t.Run("URLFunc empty result falls back to static URL", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{{
					URL:     "https://reports.example.com/static",
					URLFunc: func(_ *mux.Router, _ *http.Request) string { return "" },
				}},
			}},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		reportTo := w.Header().Get("Report-To")
		assert.Contains(t, reportTo, `"url":"https://reports.example.com/static"`)
	})

	t.Run("URLFunc only, no static URL", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{{
					URLFunc: func(_ *mux.Router, _ *http.Request) string {
						return "https://reports.example.com/only-dynamic"
					},
				}},
			}},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		reportTo := w.Header().Get("Report-To")
		assert.Contains(t, reportTo, `"url":"https://reports.example.com/only-dynamic"`)
	})

	t.Run("URLFunc empty and no static URL drops endpoint", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{
						URLFunc: func(_ *mux.Router, _ *http.Request) string { return "" },
					},
					{
						URL: "https://reports.example.com/fallback",
					},
				},
			}},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		reportTo := w.Header().Get("Report-To")
		assert.Contains(t, reportTo, `"url":"https://reports.example.com/fallback"`)
		var group ReportToGroup
		require.NoError(t, json.Unmarshal([]byte(reportTo), &group))
		assert.Len(t, group.Endpoints, 1)
	})

	t.Run("endpoint with no URL and no URLFunc returns error", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{
					{},
				},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointURL)
	})

	t.Run("URLFunc field omitted from JSON output", func(t *testing.T) {
		r := mux.NewRouter()
		mw, err := NELMiddleware(r, NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{{
					URLFunc: func(_ *mux.Router, _ *http.Request) string {
						return "https://reports.example.com/dynamic"
					},
				}},
			}},
		})
		require.NoError(t, err)

		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)
		r.Use(mw)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/test", nil))

		reportTo := w.Header().Get("Report-To")
		assert.NotContains(t, reportTo, "URLFunc")
		assert.NotContains(t, reportTo, "urlfunc")
	})

	t.Run("static endpoint validation still applies when URLFunc is set", func(t *testing.T) {
		_, err := NELMiddleware(mux.NewRouter(), NELConfig{
			MaxAge: 3600,
			ReportToGroups: []ReportToGroup{{
				Group:  "nel",
				MaxAge: 86400,
				Endpoints: []ReportToEndpoint{{
					URL:     "http://insecure.example.com/nel",
					URLFunc: func(_ *mux.Router, _ *http.Request) string { return "https://ok.example.com/nel" },
				}},
			}},
		})
		assert.ErrorIs(t, err, ErrNELEndpointScheme)
	})
}

func BenchmarkNELMiddleware(b *testing.B) {
	r := mux.NewRouter()
	r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	mw, err := NELMiddleware(r, NELConfig{
		MaxAge:            3600,
		IncludeSubdomains: true,
		FailureFraction:   1.0,
		ReportToGroups: []ReportToGroup{{
			Group:  "nel",
			MaxAge: 86400,
			Endpoints: []ReportToEndpoint{
				{URL: "https://reports.example.com/nel"},
			},
		}},
	})
	if err != nil {
		b.Fatal(err)
	}
	r.Use(mw)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}
