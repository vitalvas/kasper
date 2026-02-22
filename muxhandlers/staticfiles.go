package muxhandlers

import (
	"errors"
	"io/fs"
	"net/http"
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

	indexPath := name + "/index.html"
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

	return http.FileServerFS(fileSystem), nil
}
