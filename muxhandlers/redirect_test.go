package muxhandlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedirectMiddleware(t *testing.T) {
	t.Run("empty rules returns error", func(t *testing.T) {
		_, err := RedirectMiddleware(RedirectConfig{})
		assert.ErrorIs(t, err, ErrRedirectNoRules)
	})

	t.Run("empty From returns error", func(t *testing.T) {
		_, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{{From: "", To: "/new"}},
		})
		assert.ErrorIs(t, err, ErrRedirectEmptyFrom)
	})

	t.Run("empty To returns error", func(t *testing.T) {
		_, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{{From: "/old", To: ""}},
		})
		assert.ErrorIs(t, err, ErrRedirectEmptyTo)
	})

	t.Run("From without leading slash returns error", func(t *testing.T) {
		_, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{{From: "old", To: "/new"}},
		})
		assert.ErrorIs(t, err, ErrRedirectFromNoSlash)
	})

	t.Run("exact match permanent redirect", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/old-page", To: "/new-page"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/old-page", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		assert.Equal(t, "/new-page", rec.Header().Get("Location"))
		assert.Equal(t, "text/html", rec.Header().Get("Content-Type"))
		assert.Contains(t, rec.Body.String(), `<meta http-equiv="refresh" content="0; url=/new-page">`)
		assert.Contains(t, rec.Body.String(), `<a href="/new-page">`)
	})

	t.Run("exact match temporary redirect", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			StatusCode: http.StatusTemporaryRedirect,
			Rules: []RedirectRule{
				{From: "/maintenance", To: "/status"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/maintenance", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		assert.Equal(t, "/status", rec.Header().Get("Location"))
		assert.Contains(t, rec.Body.String(), "Temporary Redirect")
	})

	t.Run("wildcard prefix redirect", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/blog/2023/*", To: "/archive/2023/"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/blog/2023/my-post", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		assert.Equal(t, "/archive/2023/my-post", rec.Header().Get("Location"))
	})

	t.Run("wildcard prefix with nested path", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/old/*", To: "/new/"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/old/a/b/c", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		assert.Equal(t, "/new/a/b/c", rec.Header().Get("Location"))
	})

	t.Run("external URL redirect", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/github", To: "https://github.com/example", StatusCode: http.StatusTemporaryRedirect},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/github", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		assert.Equal(t, "https://github.com/example", rec.Header().Get("Location"))
	})

	t.Run("wildcard to external URL", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/docs/*", To: "https://docs.example.com/"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/docs/getting-started", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		assert.Equal(t, "https://docs.example.com/getting-started", rec.Header().Get("Location"))
	})

	t.Run("per-rule status code override", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			StatusCode: http.StatusMovedPermanently,
			Rules: []RedirectRule{
				{From: "/temp", To: "/target", StatusCode: http.StatusFound},
				{From: "/perm", To: "/target"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/temp", nil)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusFound, rec.Code)

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/perm", nil)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	})

	t.Run("non-matching request passes through", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/old", To: "/new"},
			},
		})
		require.NoError(t, err)

		var called bool
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/other", nil)
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("first matching rule wins", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/path", To: "/first"},
				{From: "/path", To: "/second"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/path", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, "/first", rec.Header().Get("Location"))
	})

	t.Run("exact match does not match subpaths", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/exact", To: "/target"},
			},
		})
		require.NoError(t, err)

		var called bool
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/exact/sub", nil)
		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("redirect to internal route", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			Rules: []RedirectRule{
				{From: "/", To: "/swagger/"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("swagger")) //nolint:errcheck
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
		assert.Equal(t, "/swagger/", rec.Header().Get("Location"))
	})

	t.Run("HTML body contains status text", func(t *testing.T) {
		mw, err := RedirectMiddleware(RedirectConfig{
			StatusCode: http.StatusPermanentRedirect,
			Rules: []RedirectRule{
				{From: "/a", To: "/b"},
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/a", nil)
		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		assert.Contains(t, body, "308 Permanent Redirect")
		assert.Contains(t, body, `<meta http-equiv="refresh" content="0; url=/b">`)
	})
}

func BenchmarkRedirectMiddleware(b *testing.B) {
	mw, err := RedirectMiddleware(RedirectConfig{
		Rules: []RedirectRule{
			{From: "/old", To: "/new"},
			{From: "/blog/*", To: "/archive/"},
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.Run("exact match", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodGet, "/old", nil)
		b.ResetTimer()
		for b.Loop() {
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("wildcard match", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodGet, "/blog/my-post", nil)
		b.ResetTimer()
		for b.Loop() {
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("no match passthrough", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodGet, "/other", nil)
		b.ResetTimer()
		for b.Loop() {
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
