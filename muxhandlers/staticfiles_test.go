package muxhandlers

import (
	"net/http"
	"net/http/httptest"
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
