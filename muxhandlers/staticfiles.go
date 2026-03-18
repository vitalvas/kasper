package muxhandlers

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// ErrStaticFilesNoFS is returned when StaticFilesConfig.FS is nil.
var ErrStaticFilesNoFS = errors.New("static files: file system must not be nil")

// ErrStaticFilesNoIndexHTML is returned when SPAFallback is enabled
// but the file system does not contain an index.html at the root.
var ErrStaticFilesNoIndexHTML = errors.New("static files: index.html is required when SPA fallback is enabled")

// StaticFilesConfig configures the static file handler.
type StaticFilesConfig struct {
	// FS is the file system to serve files from. Required.
	// Works with os.DirFS, embed.FS, and any fs.FS implementation.
	FS fs.FS

	// EnableDirectoryListing allows directory contents to be listed
	// when no index.html is present. Disabled by default for security.
	EnableDirectoryListing bool

	// SPAFallback serves the root index.html for any path that does
	// not match an existing file. This allows client-side routers to
	// handle all routes. Requires index.html at the root of FS.
	SPAFallback bool

	// EnableETag precomputes strong ETags for all files by walking the
	// FS at init time. Designed for immutable file systems such as
	// embed.FS. The handler sets the ETag response header and handles
	// If-None-Match conditional requests (304 Not Modified).
	EnableETag bool
}

// noDirListingFS wraps an fs.FS to prevent directory listing.
// When a directory is opened that does not contain an index.html,
// it returns fs.ErrNotExist so http.FileServer responds with 404.
type noDirListingFS struct {
	fs fs.FS
}

func (n *noDirListingFS) Open(name string) (fs.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if !stat.IsDir() {
		return f, nil
	}

	indexPath := fmt.Sprintf("%s/index.html", name)
	if name == "." {
		indexPath = "index.html"
	}

	if _, err := fs.Stat(n.fs, indexPath); err != nil {
		f.Close()
		return nil, fs.ErrNotExist
	}

	return f, nil
}

// spaFallbackFS wraps an fs.FS to fall back to index.html when a
// requested path does not exist. This enables client-side routing
// for single-page applications.
type spaFallbackFS struct {
	fs fs.FS
}

func (s *spaFallbackFS) Open(name string) (fs.File, error) {
	f, err := s.fs.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return s.fs.Open("index.html")
	}

	return f, err
}

// StaticFilesHandler returns an http.Handler that serves static files from
// the provided file system. It is not middleware — it serves files directly
// without calling a next handler.
func StaticFilesHandler(cfg StaticFilesConfig) (http.Handler, error) {
	if cfg.FS == nil {
		return nil, ErrStaticFilesNoFS
	}

	if cfg.SPAFallback {
		if _, err := fs.Stat(cfg.FS, "index.html"); err != nil {
			return nil, ErrStaticFilesNoIndexHTML
		}
	}

	fileSystem := cfg.FS

	if !cfg.EnableDirectoryListing {
		fileSystem = &noDirListingFS{fs: fileSystem}
	}

	if cfg.SPAFallback {
		fileSystem = &spaFallbackFS{fs: fileSystem}
	}

	handler := http.FileServerFS(fileSystem)

	if cfg.EnableETag {
		etags, err := buildStaticETags(cfg.FS)
		if err != nil {
			return nil, err
		}

		handler = staticETagHandler(handler, etags)
	}

	return handler, nil
}

// buildStaticETags walks the FS and precomputes ETags for all files.
func buildStaticETags(fsys fs.FS) (map[string]string, error) {
	etags := make(map[string]string)

	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		f, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		h := fnv.New128a()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}

		etag := fmt.Sprintf(`"%x"`, h.Sum(nil))
		etags[fmt.Sprintf("/%s", path)] = etag

		return nil
	})
	if err != nil {
		return nil, err
	}

	return etags, nil
}

// staticETagHandler wraps a file server handler to set ETag headers and
// handle If-None-Match conditional requests.
func staticETagHandler(next http.Handler, etags map[string]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag, ok := etags[r.URL.Path]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("ETag", etag)

		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if match := r.Header.Get("If-None-Match"); match != "" {
				if etagMatches(match, etag) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

// etagMatches checks whether the client's If-None-Match header value
// contains the server ETag. Supports comma-separated lists and "*".
func etagMatches(clientHeader, serverETag string) bool {
	if clientHeader == "*" {
		return true
	}

	for val := range strings.SplitSeq(clientHeader, ",") {
		if strings.TrimSpace(val) == serverETag {
			return true
		}
	}

	return false
}
