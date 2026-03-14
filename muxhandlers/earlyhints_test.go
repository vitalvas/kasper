package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEarlyHintsMiddleware(t *testing.T) {
	t.Run("config error no links", func(t *testing.T) {
		_, err := EarlyHintsMiddleware(EarlyHintsConfig{})
		assert.ErrorIs(t, err, ErrNoLinks)
	})

	t.Run("sends 103 with single link", func(t *testing.T) {
		mw, err := EarlyHintsMiddleware(EarlyHintsConfig{
			Links: []string{`</style.css>; rel=preload; as=style`},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		recorder := newInformationalRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(recorder, req)

		require.Len(t, recorder.informational, 1)
		assert.Equal(t, http.StatusEarlyHints, recorder.informational[0].code)
		assert.Equal(t,
			[]string{`</style.css>; rel=preload; as=style`},
			recorder.informational[0].header.Values("Link"),
		)
		assert.Equal(t, http.StatusOK, recorder.finalCode)
	})

	t.Run("sends 103 with multiple links", func(t *testing.T) {
		mw, err := EarlyHintsMiddleware(EarlyHintsConfig{
			Links: []string{
				`</style.css>; rel=preload; as=style`,
				`</app.js>; rel=preload; as=script`,
				`</font.woff2>; rel=preload; as=font; crossorigin`,
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		recorder := newInformationalRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(recorder, req)

		require.Len(t, recorder.informational, 1)
		links := recorder.informational[0].header.Values("Link")
		assert.Len(t, links, 3)
		assert.Equal(t, `</style.css>; rel=preload; as=style`, links[0])
		assert.Equal(t, `</app.js>; rel=preload; as=script`, links[1])
		assert.Equal(t, `</font.woff2>; rel=preload; as=font; crossorigin`, links[2])
	})

	t.Run("final response has its own headers", func(t *testing.T) {
		mw, err := EarlyHintsMiddleware(EarlyHintsConfig{
			Links: []string{`</style.css>; rel=preload; as=style`},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Custom", "value")
			w.WriteHeader(http.StatusOK)
		}))

		recorder := newInformationalRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.finalCode)
		assert.Equal(t, "value", recorder.finalHeader.Get("X-Custom"))
	})

	t.Run("internal copy is not affected by external mutation", func(t *testing.T) {
		links := []string{`</a.css>; rel=preload; as=style`}

		mw, err := EarlyHintsMiddleware(EarlyHintsConfig{Links: links})
		require.NoError(t, err)

		// Mutate the original slice after middleware creation.
		links[0] = "mutated"

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		recorder := newInformationalRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(recorder, req)

		require.Len(t, recorder.informational, 1)
		assert.Equal(t,
			[]string{`</a.css>; rel=preload; as=style`},
			recorder.informational[0].header.Values("Link"),
		)
	})
}

func BenchmarkEarlyHintsMiddleware(b *testing.B) {
	mw, err := EarlyHintsMiddleware(EarlyHintsConfig{
		Links: []string{
			`</style.css>; rel=preload; as=style`,
			`</app.js>; rel=preload; as=script`,
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ResetTimer()
	for b.Loop() {
		handler.ServeHTTP(newInformationalRecorder(), req)
	}
}

// informationalResponse captures a 1xx informational response.
type informationalResponse struct {
	code   int
	header http.Header
}

// informationalRecorder is an http.ResponseWriter that captures 1xx
// informational responses separately from the final response.
type informationalRecorder struct {
	informational []informationalResponse
	finalCode     int
	finalHeader   http.Header
	header        http.Header
}

func newInformationalRecorder() *informationalRecorder {
	return &informationalRecorder{
		header: make(http.Header),
	}
}

func (r *informationalRecorder) Header() http.Header {
	return r.header
}

func (r *informationalRecorder) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		snapshot := make(http.Header, len(r.header))
		for k, v := range r.header {
			snapshot[k] = append([]string(nil), v...)
		}
		r.informational = append(r.informational, informationalResponse{
			code:   code,
			header: snapshot,
		})
		// Clear Link headers after 1xx, mimicking net/http behavior
		// where 1xx headers are sent separately from the final response.
		r.header.Del("Link")
		return
	}
	r.finalCode = code
	r.finalHeader = make(http.Header, len(r.header))
	for k, v := range r.header {
		r.finalHeader[k] = append([]string(nil), v...)
	}
}

func (r *informationalRecorder) Write(b []byte) (int, error) {
	if r.finalCode == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return len(b), nil
}
