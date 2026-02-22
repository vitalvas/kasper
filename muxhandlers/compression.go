package muxhandlers

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/vitalvas/kasper/mux"
)

// ErrInvalidCompressionLevel is returned when CompressionConfig.Level is
// outside the valid compression level range.
var ErrInvalidCompressionLevel = errors.New("compression: invalid compression level")

// CompressionConfig configures the Compression middleware behaviour.
type CompressionConfig struct {
	// Level is the compression level for both gzip and deflate. When zero,
	// flate.DefaultCompression is used. Must be in
	// [flate.HuffmanOnly, flate.BestCompression] or zero.
	Level int

	// MinLength is the minimum response body size in bytes before compression
	// is applied. When zero, all responses are compressed.
	MinLength int
}

// compressor is the common interface implemented by both gzip.Writer and
// flate.Writer.
type compressor interface {
	io.WriteCloser
	Flush() error
	Reset(w io.Writer)
}

// CompressionMiddleware returns a middleware that compresses response bodies
// using gzip or deflate when the client advertises support via the
// Accept-Encoding header. Gzip is preferred over deflate when the client
// accepts both. It uses sync.Pool instances to reuse writers for performance.
//
// Compression is skipped when:
//   - The request does not include "gzip" or "deflate" in Accept-Encoding
//   - The response already has a Content-Encoding header
//   - The response Content-Type is an inherently compressed format
//     (image/*, video/*, audio/*, or common archive types)
//
// It returns ErrInvalidCompressionLevel if Level is outside the valid range.
func CompressionMiddleware(cfg CompressionConfig) (mux.MiddlewareFunc, error) {
	level := cfg.Level
	if level == 0 {
		level = flate.DefaultCompression
	}

	if level < flate.HuffmanOnly || level > flate.BestCompression {
		return nil, ErrInvalidCompressionLevel
	}

	minLength := cfg.MinLength

	gzipPool := &sync.Pool{
		New: func() any {
			w, _ := gzip.NewWriterLevel(io.Discard, level)
			return w
		},
	}

	deflatePool := &sync.Pool{
		New: func() any {
			w, _ := flate.NewWriter(io.Discard, level)
			return w
		},
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			encoding := selectEncoding(r)
			if encoding == "" {
				next.ServeHTTP(w, r)
				return
			}

			var pool *sync.Pool
			if encoding == "gzip" {
				pool = gzipPool
			} else {
				pool = deflatePool
			}

			cw := &compressedResponseWriter{
				ResponseWriter: w,
				pool:           pool,
				minLength:      minLength,
				encoding:       encoding,
			}
			defer cw.close()

			next.ServeHTTP(cw, r)
		})
	}, nil
}

// selectEncoding returns the best supported encoding from the Accept-Encoding
// header. It returns "gzip", "deflate", or "" if neither is accepted. When
// both are accepted with equal quality, gzip is preferred.
func selectEncoding(r *http.Request) string {
	var (
		gzipQ    float64 = -1
		deflateQ float64 = -1
		wildQ    float64 = -1
	)

	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		name, quality := parseEncoding(strings.TrimSpace(part))
		q := parseQuality(quality)

		switch name {
		case "gzip":
			gzipQ = q
		case "deflate":
			deflateQ = q
		case "*":
			wildQ = q
		}
	}

	// Apply wildcard to unspecified encodings.
	if gzipQ < 0 && wildQ >= 0 {
		gzipQ = wildQ
	}

	if deflateQ < 0 && wildQ >= 0 {
		deflateQ = wildQ
	}

	// Prefer gzip when quality is equal or higher.
	if gzipQ > 0 && gzipQ >= deflateQ {
		return "gzip"
	}

	if deflateQ > 0 {
		return "deflate"
	}

	return ""
}

// parseQuality converts a quality string to a float64.
// An empty string defaults to 1.0 (implicit full quality per HTTP spec).
func parseQuality(s string) float64 {
	if s == "" {
		return 1.0
	}

	q, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	return q
}

// parseEncoding splits an encoding token into the encoding name and quality
// value. For "gzip;q=0.8" it returns ("gzip", "0.8"). When no quality value
// is present it returns the encoding and an empty string.
func parseEncoding(s string) (encoding, quality string) {
	encoding, params, ok := strings.Cut(s, ";")
	if !ok {
		return strings.TrimSpace(encoding), ""
	}

	params = strings.TrimSpace(params)
	if key, val, found := strings.Cut(params, "="); found && strings.TrimSpace(key) == "q" {
		return strings.TrimSpace(encoding), strings.TrimSpace(val)
	}

	return strings.TrimSpace(encoding), ""
}

// compressedContentTypes contains content type prefixes and exact types that
// are already compressed and should not be double-compressed.
var compressedContentTypes = []string{
	"image/",
	"video/",
	"audio/",
	"application/zip",
	"application/gzip",
	"application/x-gzip",
	"application/x-bzip2",
	"application/x-xz",
	"application/zstd",
	"application/x-7z-compressed",
	"application/x-rar-compressed",
}

// isCompressedContentType reports whether the content type is an inherently
// compressed format.
func isCompressedContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))

	for _, prefix := range compressedContentTypes {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}

	return false
}

// compressedResponseWriter wraps http.ResponseWriter to conditionally apply
// gzip or deflate compression. It buffers writes until MinLength bytes are
// collected to decide whether compression should be applied.
type compressedResponseWriter struct {
	http.ResponseWriter
	pool      *sync.Pool
	minLength int
	encoding  string

	writer      compressor
	buf         []byte
	wroteHeader bool
	decided     bool
	compressed  bool
	statusCode  int
}

func (cw *compressedResponseWriter) WriteHeader(statusCode int) {
	if cw.wroteHeader {
		return
	}

	cw.statusCode = statusCode
	cw.wroteHeader = true

	if cw.decided {
		cw.ResponseWriter.WriteHeader(statusCode)
	}
}

func (cw *compressedResponseWriter) Write(b []byte) (int, error) {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}

	if cw.decided {
		if cw.compressed {
			return cw.writer.Write(b)
		}

		return cw.ResponseWriter.Write(b)
	}

	cw.buf = append(cw.buf, b...)

	if len(cw.buf) >= cw.minLength {
		cw.finalize()
	}

	return len(b), nil
}

// finalize decides whether to compress, writes headers, and flushes buffered
// data.
func (cw *compressedResponseWriter) finalize() {
	cw.decided = true

	h := cw.Header()
	if h.Get("Content-Encoding") != "" || isCompressedContentType(h.Get("Content-Type")) {
		cw.compressed = false
		cw.ResponseWriter.WriteHeader(cw.statusCode)
		cw.ResponseWriter.Write(cw.buf)
		cw.buf = nil

		return
	}

	cw.compressed = true
	h.Set("Content-Encoding", cw.encoding)
	h.Set("Vary", "Accept-Encoding")
	h.Del("Content-Length")

	cw.writer = cw.pool.Get().(compressor)
	cw.writer.Reset(cw.ResponseWriter)

	cw.ResponseWriter.WriteHeader(cw.statusCode)
	cw.writer.Write(cw.buf)
	cw.buf = nil
}

// close finalizes outstanding writes. If compression was not yet decided
// (response smaller than MinLength), it flushes the buffer uncompressed.
func (cw *compressedResponseWriter) close() {
	if !cw.decided {
		cw.decided = true

		if len(cw.buf) > 0 && len(cw.buf) < cw.minLength {
			// Buffer is below MinLength threshold; flush uncompressed.
			cw.ResponseWriter.WriteHeader(cw.statusCode)
			cw.ResponseWriter.Write(cw.buf)
			cw.buf = nil

			return
		}

		if len(cw.buf) > 0 {
			cw.decided = false
			cw.finalize()
		} else if cw.wroteHeader {
			cw.ResponseWriter.WriteHeader(cw.statusCode)
		}
	}

	if cw.writer != nil {
		cw.writer.Close()
		cw.pool.Put(cw.writer)
		cw.writer = nil
	}
}

// Flush implements http.Flusher by flushing the compression writer and the
// underlying ResponseWriter if it supports flushing.
func (cw *compressedResponseWriter) Flush() {
	if !cw.decided && len(cw.buf) > 0 {
		cw.finalize()
	}

	if cw.writer != nil {
		cw.writer.Flush()
	}

	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for middleware chaining.
func (cw *compressedResponseWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}
