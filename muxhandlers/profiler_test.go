package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

func TestProfilerHandler(t *testing.T) {
	r := mux.NewRouter()
	r.PathPrefix("/debug/pprof").Handler(ProfilerHandler())

	tests := []struct {
		name         string
		path         string
		expectedCode int
		contentType  string
	}{
		{
			name:         "index page",
			path:         "/debug/pprof/",
			expectedCode: http.StatusOK,
			contentType:  "text/html; charset=utf-8",
		},
		{
			name:         "cmdline",
			path:         "/debug/pprof/cmdline",
			expectedCode: http.StatusOK,
			contentType:  "text/plain; charset=utf-8",
		},
		{
			name:         "symbol",
			path:         "/debug/pprof/symbol",
			expectedCode: http.StatusOK,
		},
		{
			name:         "heap profile via index",
			path:         "/debug/pprof/heap",
			expectedCode: http.StatusOK,
			contentType:  "application/octet-stream",
		},
		{
			name:         "goroutine profile via index",
			path:         "/debug/pprof/goroutine?debug=1",
			expectedCode: http.StatusOK,
		},
		{
			name:         "allocs profile via index",
			path:         "/debug/pprof/allocs",
			expectedCode: http.StatusOK,
			contentType:  "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
			if tt.contentType != "" {
				assert.Equal(t, tt.contentType, w.Header().Get("Content-Type"))
			}
		})
	}
}
