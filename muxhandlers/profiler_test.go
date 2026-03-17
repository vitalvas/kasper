package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

func TestRegisterProfiler(t *testing.T) {
	endpoints := []struct {
		name         string
		suffix       string
		expectedCode int
		contentType  string
	}{
		{
			name:         "index page",
			suffix:       "/debug/pprof/",
			expectedCode: http.StatusOK,
			contentType:  "text/html; charset=utf-8",
		},
		{
			name:         "cmdline",
			suffix:       "/debug/pprof/cmdline",
			expectedCode: http.StatusOK,
			contentType:  "text/plain; charset=utf-8",
		},
		{
			name:         "symbol",
			suffix:       "/debug/pprof/symbol",
			expectedCode: http.StatusOK,
		},
		{
			name:         "heap profile via index",
			suffix:       "/debug/pprof/heap",
			expectedCode: http.StatusOK,
			contentType:  "application/octet-stream",
		},
		{
			name:         "goroutine profile via index",
			suffix:       "/debug/pprof/goroutine?debug=1",
			expectedCode: http.StatusOK,
		},
		{
			name:         "allocs profile via index",
			suffix:       "/debug/pprof/allocs",
			expectedCode: http.StatusOK,
			contentType:  "application/octet-stream",
		},
		{
			name:         "expvar",
			suffix:       "/debug/vars",
			expectedCode: http.StatusOK,
			contentType:  "application/json; charset=utf-8",
		},
	}

	prefixes := []struct {
		name   string
		prefix string
	}{
		{"root mount", ""},
		{"/_internal prefix", "/_internal"},
		{"/ops prefix", "/ops"},
	}

	for _, pp := range prefixes {
		t.Run(pp.name, func(t *testing.T) {
			r := mux.NewRouter()
			if pp.prefix == "" {
				RegisterProfiler(r)
			} else {
				r.Route(pp.prefix, RegisterProfiler)
			}

			for _, tt := range endpoints {
				t.Run(tt.name, func(t *testing.T) {
					w := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, pp.prefix+tt.suffix, nil)
					r.ServeHTTP(w, req)

					assert.Equal(t, tt.expectedCode, w.Code)
					if tt.contentType != "" {
						assert.Equal(t, tt.contentType, w.Header().Get("Content-Type"))
					}
				})
			}
		})
	}
}
