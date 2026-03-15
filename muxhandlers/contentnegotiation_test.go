package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContentNegotiationMiddleware(t *testing.T) {
	tests := []struct {
		name         string
		offered      []string
		accept       string
		wantCode     int
		wantSelected string
	}{
		{
			name:         "exact match",
			offered:      []string{"application/json", "application/xml"},
			accept:       "application/json",
			wantCode:     http.StatusOK,
			wantSelected: "application/json",
		},
		{
			name:         "exact match second type",
			offered:      []string{"application/json", "application/xml"},
			accept:       "application/xml",
			wantCode:     http.StatusOK,
			wantSelected: "application/xml",
		},
		{
			name:         "wildcard accepts first offered",
			offered:      []string{"application/json", "text/html"},
			accept:       "*/*",
			wantCode:     http.StatusOK,
			wantSelected: "application/json",
		},
		{
			name:         "subtype wildcard",
			offered:      []string{"application/json", "text/html"},
			accept:       "text/*",
			wantCode:     http.StatusOK,
			wantSelected: "text/html",
		},
		{
			name:         "no Accept header defaults to first offered",
			offered:      []string{"application/json", "text/html"},
			accept:       "",
			wantCode:     http.StatusOK,
			wantSelected: "application/json",
		},
		{
			name:     "no match returns 406",
			offered:  []string{"application/json"},
			accept:   "text/html",
			wantCode: http.StatusNotAcceptable,
		},
		{
			name:         "quality preference selects higher q",
			offered:      []string{"application/json", "application/xml"},
			accept:       "application/json;q=0.5, application/xml;q=0.9",
			wantCode:     http.StatusOK,
			wantSelected: "application/xml",
		},
		{
			name:         "specific type preferred over wildcard",
			offered:      []string{"application/json", "text/html"},
			accept:       "text/html, */*;q=0.1",
			wantCode:     http.StatusOK,
			wantSelected: "text/html",
		},
		{
			name:         "type wildcard preferred over full wildcard",
			offered:      []string{"application/json", "text/html"},
			accept:       "text/*, */*;q=0.1",
			wantCode:     http.StatusOK,
			wantSelected: "text/html",
		},
		{
			name:     "q=0 explicitly excludes type",
			offered:  []string{"application/json"},
			accept:   "application/json;q=0",
			wantCode: http.StatusNotAcceptable,
		},
		{
			name:         "case insensitive matching",
			offered:      []string{"application/json"},
			accept:       "Application/JSON",
			wantCode:     http.StatusOK,
			wantSelected: "application/json",
		},
		{
			name:         "multiple types with mixed quality",
			offered:      []string{"application/json", "application/xml", "text/html"},
			accept:       "text/html;q=0.9, application/xml;q=0.8, application/json;q=1.0",
			wantCode:     http.StatusOK,
			wantSelected: "application/json",
		},
		{
			name:         "accept with extra whitespace",
			offered:      []string{"application/json"},
			accept:       " application/json , text/html ",
			wantCode:     http.StatusOK,
			wantSelected: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := ContentNegotiationMiddleware(ContentNegotiationConfig{
				Offered: tt.offered,
			})

			var selected string
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				selected = NegotiatedType(r)
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			if tt.wantSelected != "" {
				assert.Equal(t, tt.wantSelected, selected)
			}
		})
	}
}

func TestContentNegotiationAcceptAll(t *testing.T) {
	t.Run("empty offered accepts any type", func(t *testing.T) {
		mw := ContentNegotiationMiddleware(ContentNegotiationConfig{})

		var selected string
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			selected = NegotiatedType(r)
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/csv")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "*/*", selected)
	})

	t.Run("empty offered with no Accept header defaults to wildcard", func(t *testing.T) {
		mw := ContentNegotiationMiddleware(ContentNegotiationConfig{})

		var selected string
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			selected = NegotiatedType(r)
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "*/*", selected)
	})

	t.Run("empty offered never returns 406", func(t *testing.T) {
		mw := ContentNegotiationMiddleware(ContentNegotiationConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/csv")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestNegotiatedType(t *testing.T) {
	t.Run("returns empty when no negotiation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		assert.Equal(t, "", NegotiatedType(req))
	})
}

func BenchmarkContentNegotiationMiddleware(b *testing.B) {
	b.Run("single type", func(b *testing.B) {
		mw := ContentNegotiationMiddleware(ContentNegotiationConfig{
			Offered: []string{"application/json"},
		})
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "application/json")

		b.ResetTimer()
		for b.Loop() {
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("multiple types with quality", func(b *testing.B) {
		mw := ContentNegotiationMiddleware(ContentNegotiationConfig{
			Offered: []string{"application/json", "application/xml", "text/html"},
		})
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/html;q=0.9, application/json;q=1.0, */*;q=0.1")

		b.ResetTimer()
		for b.Loop() {
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
