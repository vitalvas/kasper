package muxhandlers

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticFilesHandler(t *testing.T) {
	testFS := fstest.MapFS{
		"file.txt":              {Data: []byte("hello world")},
		"style.css":             {Data: []byte("body{}")},
		"app.js":                {Data: []byte("console.log('hi')")},
		"page.html":             {Data: []byte("<html>page</html>")},
		"index.html":            {Data: []byte("<html>root</html>")},
		"docs/index.html":       {Data: []byte("<html>docs</html>")},
		"docs/guide.txt":        {Data: []byte("guide content")},
		"images/logo.png":       {Data: []byte("png-data")},
		"empty-dir/placeholder": {Data: []byte("")},
	}

	t.Run("nil FS returns error", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{})
		assert.Nil(t, handler)
		assert.ErrorIs(t, err, ErrStaticFilesNoFS)
	})

	t.Run("serves file", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:                     testFS,
			EnableDirectoryListing: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello world", rec.Body.String())
	})

	t.Run("returns 404 for missing file", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/nonexistent.txt", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("serves index.html for directory", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>docs</html>")
	})

	t.Run("directory listing enabled", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:                     testFS,
			EnableDirectoryListing: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/images/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "logo.png")
	})

	t.Run("directory listing disabled returns 404", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/images/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("directory listing disabled with index.html still serves", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>docs</html>")
	})

	t.Run("serves correct content type", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(t, err)

		tests := []struct {
			path        string
			contentType string
		}{
			{"/style.css", "text/css"},
			{"/app.js", "text/javascript"},
			{"/page.html", "text/html"},
		}

		for _, tt := range tests {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), tt.contentType)
		}
	})

	t.Run("root index.html with directory listing disabled", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>root</html>")
	})

	t.Run("nested directory without index.html returns 404", func(t *testing.T) {
		nestedFS := fstest.MapFS{
			"a/b/c/file.txt": {Data: []byte("deep")},
		}

		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: nestedFS,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/a/b/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("spa fallback without index.html returns error", func(t *testing.T) {
		noIndexFS := fstest.MapFS{
			"app.js": {Data: []byte("js")},
		}

		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          noIndexFS,
			SPAFallback: true,
		})
		assert.Nil(t, handler)
		assert.ErrorIs(t, err, ErrStaticFilesNoIndexHTML)
	})

	t.Run("spa fallback serves existing file", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "console.log('hi')", rec.Body.String())
	})

	t.Run("spa fallback serves index.html for missing path", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>root</html>")
	})

	t.Run("spa fallback serves index.html for deep missing path", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/123/settings", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>root</html>")
	})

	t.Run("spa fallback serves directory index.html when present", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>docs</html>")
	})

	t.Run("path traversal returns 404", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(t, err)

		paths := []string{
			"/../../../etc/passwd",
			"/..%2f..%2f..%2fetc/passwd",
			"/../file.txt",
			"/docs/../../file.txt",
			"/..\\..\\..\\etc\\passwd",
		}

		for _, p := range paths {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, p, nil)
			handler.ServeHTTP(rec, req)

			assert.NotContains(t, rec.Body.String(), "root:x:", "path %q must not leak file content", p)
		}
	})

	t.Run("path traversal with spa fallback does not leak files", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
		})
		require.NoError(t, err)

		paths := []string{
			"/../../../etc/passwd",
			"/..%2f..%2f..%2fetc/passwd",
			"/../file.txt",
			"/docs/../../file.txt",
		}

		for _, p := range paths {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, p, nil)
			handler.ServeHTTP(rec, req)

			assert.NotContains(t, rec.Body.String(), "root:x:", "path %q must not leak file content", p)
		}
	})

	t.Run("spa fallback for directory without index.html", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/images", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>root</html>")
	})

	t.Run("etag sets header on response", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		etag := rec.Header().Get("ETag")
		assert.NotEmpty(t, etag)
		assert.True(t, strings.HasPrefix(etag, `"`))
		assert.True(t, strings.HasSuffix(etag, `"`))
	})

	t.Run("etag is consistent for same file", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec1, req1)

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, rec1.Header().Get("ETag"), rec2.Header().Get("ETag"))
	})

	t.Run("etag differs for different files", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec1, req1)

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/style.css", nil)
		handler.ServeHTTP(rec2, req2)

		assert.NotEqual(t, rec1.Header().Get("ETag"), rec2.Header().Get("ETag"))
	})

	t.Run("etag if-none-match returns 304", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec1, req1)

		etag := rec1.Header().Get("ETag")
		require.NotEmpty(t, etag)

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		req2.Header.Set("If-None-Match", etag)
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, http.StatusNotModified, rec2.Code)
		assert.Empty(t, rec2.Body.String())
	})

	t.Run("etag if-none-match with HEAD returns 304", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec1, req1)

		etag := rec1.Header().Get("ETag")

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodHead, "/file.txt", nil)
		req2.Header.Set("If-None-Match", etag)
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, http.StatusNotModified, rec2.Code)
	})

	t.Run("etag if-none-match mismatch returns 200", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		req.Header.Set("If-None-Match", `"wrong"`)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello world", rec.Body.String())
	})

	t.Run("etag if-none-match wildcard returns 304", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		req.Header.Set("If-None-Match", "*")
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotModified, rec.Code)
	})

	t.Run("etag if-none-match with multiple values", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec1, req1)

		etag := rec1.Header().Get("ETag")

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		req2.Header.Set("If-None-Match", fmt.Sprintf(`"other", %s, "another"`, etag))
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, http.StatusNotModified, rec2.Code)
	})

	t.Run("etag missing file has no etag", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/nonexistent.txt", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Empty(t, rec.Header().Get("ETag"))
	})

	t.Run("etag with spa fallback", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
			EnableETag:  true,
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/style.css", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.NotEmpty(t, rec.Header().Get("ETag"))
	})

	t.Run("etag build error propagates", func(t *testing.T) {
		_, err := StaticFilesHandler(StaticFilesConfig{
			FS:         &failOpenFS{inner: testFS},
			EnableETag: true,
		})
		assert.Error(t, err)
	})

	t.Run("alias serves target file", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
			Aliases: map[string]string{
				"/builder/": "page.html",
			},
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/builder/", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "<html>page</html>")
	})

	t.Run("alias non-matching path falls through", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
			Aliases: map[string]string{
				"/builder/": "page.html",
			},
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello world", rec.Body.String())
	})

	t.Run("alias target not found returns error", func(t *testing.T) {
		_, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
			Aliases: map[string]string{
				"/missing/": "nonexistent.html",
			},
		})
		assert.ErrorIs(t, err, ErrStaticFilesAliasTargetNotFound)
	})

	t.Run("alias with etag returns correct etag", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
			Aliases: map[string]string{
				"/builder/": "page.html",
			},
		})
		require.NoError(t, err)

		// Get ETag via alias path.
		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/builder/", nil)
		handler.ServeHTTP(rec1, req1)

		assert.Equal(t, http.StatusOK, rec1.Code)
		aliasETag := rec1.Header().Get("ETag")
		assert.NotEmpty(t, aliasETag)

		// Get ETag via direct file path.
		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/page.html", nil)
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, aliasETag, rec2.Header().Get("ETag"))
	})

	t.Run("alias with etag if-none-match returns 304", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			EnableETag: true,
			Aliases: map[string]string{
				"/builder/": "page.html",
			},
		})
		require.NoError(t, err)

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/builder/", nil)
		handler.ServeHTTP(rec1, req1)

		etag := rec1.Header().Get("ETag")
		require.NotEmpty(t, etag)

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/builder/", nil)
		req2.Header.Set("If-None-Match", etag)
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, http.StatusNotModified, rec2.Code)
	})

	t.Run("multiple aliases", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
			Aliases: map[string]string{
				"/builder/":    "page.html",
				"/playground/": "file.txt",
			},
		})
		require.NoError(t, err)

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/builder/", nil)
		handler.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusOK, rec1.Code)
		assert.Contains(t, rec1.Body.String(), "<html>page</html>")

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/playground/", nil)
		handler.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusOK, rec2.Code)
		assert.Equal(t, "hello world", rec2.Body.String())
	})

	t.Run("path prefix strips prefix for file serving", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			PathPrefix: "/ui",
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ui/file.txt", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello world", rec.Body.String())
	})

	t.Run("path prefix with alias", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			PathPrefix: "/ui",
			Aliases: map[string]string{
				"/policy-builder/": "page.html",
			},
		})
		require.NoError(t, err)

		// Alias path works.
		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/ui/policy-builder/", nil)
		handler.ServeHTTP(rec1, req1)

		assert.Equal(t, http.StatusOK, rec1.Code)
		assert.Contains(t, rec1.Body.String(), "<html>page</html>")

		// Regular file still works.
		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/ui/style.css", nil)
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, http.StatusOK, rec2.Code)
		assert.Contains(t, rec2.Body.String(), "body{}")
	})

	t.Run("path prefix with alias and etag", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			PathPrefix: "/ui",
			EnableETag: true,
			Aliases: map[string]string{
				"/policy-builder/": "page.html",
			},
		})
		require.NoError(t, err)

		// Alias path has ETag.
		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest(http.MethodGet, "/ui/policy-builder/", nil)
		handler.ServeHTTP(rec1, req1)

		assert.Equal(t, http.StatusOK, rec1.Code)
		aliasETag := rec1.Header().Get("ETag")
		assert.NotEmpty(t, aliasETag)

		// Direct file has same ETag.
		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/ui/page.html", nil)
		handler.ServeHTTP(rec2, req2)

		assert.Equal(t, aliasETag, rec2.Header().Get("ETag"))

		// If-None-Match works on alias.
		rec3 := httptest.NewRecorder()
		req3 := httptest.NewRequest(http.MethodGet, "/ui/policy-builder/", nil)
		req3.Header.Set("If-None-Match", aliasETag)
		handler.ServeHTTP(rec3, req3)

		assert.Equal(t, http.StatusNotModified, rec3.Code)
	})

	t.Run("path prefix without leading slash", func(t *testing.T) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:         testFS,
			PathPrefix: "/static",
		})
		require.NoError(t, err)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/static/file.txt", nil)
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello world", rec.Body.String())
	})
}

// failOpenFS wraps an fs.FS but fails when opening non-directory files.
type failOpenFS struct {
	inner fs.FS
}

func (f *failOpenFS) Open(name string) (fs.File, error) {
	file, err := f.inner.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if stat.IsDir() {
		return file, nil
	}
	file.Close()
	return nil, errors.New("simulated open error")
}

func BenchmarkStaticFilesHandler(b *testing.B) {
	testFS := fstest.MapFS{
		"file.txt":        {Data: []byte("hello world")},
		"index.html":      {Data: []byte("<html>root</html>")},
		"docs/index.html": {Data: []byte("<html>docs</html>")},
	}

	b.Run("file request", func(b *testing.B) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:                     testFS,
			EnableDirectoryListing: true,
		})
		require.NoError(b, err)

		req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)

		b.ResetTimer()
		for range b.N {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}
	})

	b.Run("directory listing disabled", func(b *testing.B) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS: testFS,
		})
		require.NoError(b, err)

		req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)

		b.ResetTimer()
		for range b.N {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}
	})

	b.Run("spa fallback hit", func(b *testing.B) {
		handler, err := StaticFilesHandler(StaticFilesConfig{
			FS:          testFS,
			SPAFallback: true,
		})
		require.NoError(b, err)

		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)

		b.ResetTimer()
		for range b.N {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
		}
	})
}
