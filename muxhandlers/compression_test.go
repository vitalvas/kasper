package muxhandlers

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.vitalvas.com/kasper/mux"
)

func TestCompressionMiddleware(t *testing.T) {
	t.Run("config validation", func(t *testing.T) {
		tests := []struct {
			name    string
			config  CompressionConfig
			wantErr error
		}{
			{"level too low", CompressionConfig{Level: -3}, ErrInvalidCompressionLevel},
			{"level too high", CompressionConfig{Level: 10}, ErrInvalidCompressionLevel},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := CompressionMiddleware(tt.config)
				assert.ErrorIs(t, err, tt.wantErr)
			})
		}

		t.Run("valid configs", func(t *testing.T) {
			validConfigs := []CompressionConfig{
				{},
				{Level: flate.BestSpeed},
				{Level: flate.BestCompression},
				{Level: flate.HuffmanOnly},
				{Level: 0, MinLength: 1024},
			}

			for _, cfg := range validConfigs {
				_, err := CompressionMiddleware(cfg)
				assert.NoError(t, err)
			}
		})
	})

	t.Run("compresses response with gzip", func(t *testing.T) {
		body := "hello world, this is a test response body"

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(body))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
		assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))
		assert.Empty(t, w.Header().Get("Content-Length"))

		gr, err := gzip.NewReader(w.Body)
		require.NoError(t, err)
		defer gr.Close()

		decompressed, err := io.ReadAll(gr)
		require.NoError(t, err)
		assert.Equal(t, body, string(decompressed))
	})

	t.Run("compresses response with deflate", func(t *testing.T) {
		body := "hello world, this is a test response body"

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(body))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "deflate")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "deflate", w.Header().Get("Content-Encoding"))
		assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))

		dr := flate.NewReader(w.Body)
		defer dr.Close()

		decompressed, err := io.ReadAll(dr)
		require.NoError(t, err)
		assert.Equal(t, body, string(decompressed))
	})

	t.Run("prefers gzip over deflate", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("data"))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "deflate, gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	})

	t.Run("deflate preferred when higher quality", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("data"))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip;q=0.5, deflate;q=0.9")
		r.ServeHTTP(w, req)

		assert.Equal(t, "deflate", w.Header().Get("Content-Encoding"))
	})

	t.Run("skips when no Accept-Encoding", func(t *testing.T) {
		body := "uncompressed response"

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(body))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Header().Get("Content-Encoding"))
		assert.Equal(t, body, w.Body.String())
	})

	t.Run("skips when Content-Encoding already set", func(t *testing.T) {
		body := "already encoded"

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Encoding", "br")
			w.Write([]byte(body))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, "br", w.Header().Get("Content-Encoding"))
		assert.Equal(t, body, w.Body.String())
	})

	t.Run("skips compressed content types", func(t *testing.T) {
		tests := []struct {
			name        string
			contentType string
		}{
			{"image/png", "image/png"},
			{"image/jpeg", "image/jpeg"},
			{"video/mp4", "video/mp4"},
			{"audio/mpeg", "audio/mpeg"},
			{"application/zip", "application/zip"},
			{"application/gzip", "application/gzip"},
			{"application/x-gzip", "application/x-gzip"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				r := mux.NewRouter()
				r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", tt.contentType)
					w.Write([]byte("binary data"))
				}).Methods(http.MethodGet)

				mw, err := CompressionMiddleware(CompressionConfig{})
				require.NoError(t, err)
				r.Use(mw)

				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Accept-Encoding", "gzip")
				r.ServeHTTP(w, req)

				assert.NotEqual(t, "gzip", w.Header().Get("Content-Encoding"))
			})
		}
	})

	t.Run("MinLength skips small responses", func(t *testing.T) {
		body := "short"

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(body))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{MinLength: 1024})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEqual(t, "gzip", w.Header().Get("Content-Encoding"))
		assert.Equal(t, body, w.Body.String())
	})

	t.Run("MinLength compresses large responses", func(t *testing.T) {
		body := strings.Repeat("a", 2048)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(body))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{MinLength: 1024})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

		gr, err := gzip.NewReader(w.Body)
		require.NoError(t, err)
		defer gr.Close()

		decompressed, err := io.ReadAll(gr)
		require.NoError(t, err)
		assert.Equal(t, body, string(decompressed))
	})

	t.Run("preserves status code", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}).Methods(http.MethodPost)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	})

	t.Run("multiple writes", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("first "))
			w.Write([]byte("second "))
			w.Write([]byte("third"))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

		gr, err := gzip.NewReader(w.Body)
		require.NoError(t, err)
		defer gr.Close()

		decompressed, err := io.ReadAll(gr)
		require.NoError(t, err)
		assert.Equal(t, "first second third", string(decompressed))
	})

	t.Run("Flush delegates to underlying flusher", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("flushed data"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	})

	t.Run("Unwrap returns underlying ResponseWriter", func(t *testing.T) {
		w := httptest.NewRecorder()
		cw := &compressedResponseWriter{
			ResponseWriter: w,
		}
		assert.Equal(t, w, cw.Unwrap())
	})

	t.Run("compression levels produce valid output", func(t *testing.T) {
		levels := []int{
			flate.HuffmanOnly,
			flate.BestSpeed,
			flate.DefaultCompression,
			flate.BestCompression,
		}

		body := strings.Repeat("test data ", 100)

		for _, level := range levels {
			r := mux.NewRouter()
			r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.Write([]byte(body))
			}).Methods(http.MethodGet)

			mw, err := CompressionMiddleware(CompressionConfig{Level: level})
			require.NoError(t, err)
			r.Use(mw)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			r.ServeHTTP(w, req)

			gr, err := gzip.NewReader(w.Body)
			require.NoError(t, err)

			decompressed, err := io.ReadAll(gr)
			require.NoError(t, err)
			gr.Close()

			assert.Equal(t, body, string(decompressed))
		}
	})

	t.Run("deflate compression levels produce valid output", func(t *testing.T) {
		body := strings.Repeat("test data ", 100)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(body))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{Level: flate.BestSpeed})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "deflate")
		r.ServeHTTP(w, req)

		assert.Equal(t, "deflate", w.Header().Get("Content-Encoding"))

		dr := flate.NewReader(w.Body)
		decompressed, err := io.ReadAll(dr)
		require.NoError(t, err)
		dr.Close()

		assert.Equal(t, body, string(decompressed))
	})

	t.Run("empty body with WriteHeader only", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("preserves existing Vary values", func(t *testing.T) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Add("Vary", "Origin")
			_, _ = w.Write([]byte("compress me"))
		})

		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		r.Use(mw)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		r.ServeHTTP(w, req)

		assert.ElementsMatch(t, []string{"Origin", "Accept-Encoding"}, w.Header().Values("Vary"))
	})

	t.Run("preserves informational response handling", func(t *testing.T) {
		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		rec := newInformationalRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			_, err := w.Write([]byte("final body"))
			require.NoError(t, err)
		})).ServeHTTP(rec, req)

		require.Len(t, rec.informational, 1)
		assert.Equal(t, http.StatusEarlyHints, rec.informational[0].code)
		assert.Equal(t, http.StatusOK, rec.finalCode)
	})

	t.Run("rejects bodies forbidden by response status", func(t *testing.T) {
		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				n, err := w.Write([]byte("not allowed"))
				require.ErrorIs(t, err, http.ErrBodyNotAllowed)
				assert.Zero(t, n)
			})).ServeHTTP(rec, req)
			assert.Equal(t, status, rec.Code)
			assert.Empty(t, rec.Body.String())
			assert.Empty(t, rec.Header().Get("Content-Encoding"))
		}
	})

	t.Run("suppresses HEAD response body", func(t *testing.T) {
		mw, err := CompressionMiddleware(CompressionConfig{})
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodHead, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n, err := w.Write([]byte("representation"))
			require.NoError(t, err)
			assert.Equal(t, len("representation"), n)
		})).ServeHTTP(rec, req)
		assert.Empty(t, rec.Body.String())
	})
}

func TestSelectEncoding(t *testing.T) {
	tests := []struct {
		name           string
		acceptEncoding string
		want           string
	}{
		{"gzip only", "gzip", "gzip"},
		{"deflate only", "deflate", "deflate"},
		{"gzip and deflate prefers gzip", "deflate, gzip", "gzip"},
		{"deflate higher quality", "gzip;q=0.5, deflate;q=0.9", "deflate"},
		{"gzip higher quality", "gzip;q=0.9, deflate;q=0.5", "gzip"},
		{"equal quality prefers gzip", "gzip;q=0.8, deflate;q=0.8", "gzip"},
		{"wildcard selects gzip", "*", "gzip"},
		{"wildcard with quality", "*;q=0.5", "gzip"},
		{"wildcard q=0 rejects all", "*;q=0", ""},
		{"gzip q=0 falls back to deflate", "gzip;q=0, deflate", "deflate"},
		{"both q=0", "gzip;q=0, deflate;q=0", ""},
		{"no supported encoding", "br", ""},
		{"empty", "", ""},
		{"gzip with quality values", "gzip;q=0.8", "gzip"},
		{"deflate with quality values", "deflate;q=0.8", "deflate"},
		{"identity only", "identity", ""},
		{"mixed with unsupported", "br, gzip, zstd", "gzip"},
		{"wildcard overridden by explicit gzip q=0", "gzip;q=0, *", "deflate"},
		{"content coding is case insensitive", "GZip", "gzip"},
		{"quality above one is rejected", "gzip;q=2", ""},
		{"negative quality is rejected", "gzip;q=-0.1", ""},
		{"quality parameter after extension is parsed", "gzip;level=5;Q=0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			assert.Equal(t, tt.want, selectEncoding(req))
		})
	}
}

func TestParseEncoding(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantEnc     string
		wantQuality string
	}{
		{"simple encoding", "gzip", "gzip", ""},
		{"encoding with quality", "gzip;q=0.8", "gzip", "0.8"},
		{"encoding with quality 0", "gzip;q=0", "gzip", "0"},
		{"encoding with spaces", " gzip ; q=0.8 ", "gzip", "0.8"},
		{"wildcard", "*", "*", ""},
		{"wildcard with quality", "*;q=0.5", "*", "0.5"},
		{"no quality param", "gzip;level=5", "gzip", ""},
		{"case insensitive encoding and quality", "GZip;Q=0.7", "gzip", "0.7"},
		{"quality after another parameter", "gzip;level=5;q=0.4", "gzip", "0.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, quality := parseEncoding(tt.input)
			assert.Equal(t, tt.wantEnc, enc)
			assert.Equal(t, tt.wantQuality, quality)
		})
	}
}

func TestIsCompressedContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"text/plain", "text/plain", false},
		{"text/html", "text/html", false},
		{"application/json", "application/json", false},
		{"image/png", "image/png", true},
		{"image/jpeg", "image/jpeg", true},
		{"video/mp4", "video/mp4", true},
		{"audio/mpeg", "audio/mpeg", true},
		{"application/zip", "application/zip", true},
		{"application/gzip", "application/gzip", true},
		{"application/x-gzip", "application/x-gzip", true},
		{"application/x-bzip2", "application/x-bzip2", true},
		{"uppercase Image/PNG", "Image/PNG", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCompressedContentType(tt.contentType))
		})
	}
}

func BenchmarkCompressionMiddleware(b *testing.B) {
	b.Run("gzip compress", func(b *testing.B) {
		body := bytes.Repeat([]byte("benchmark data "), 100)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write(body)
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("deflate compress", func(b *testing.B) {
		body := bytes.Repeat([]byte("benchmark data "), 100)

		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write(body)
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "deflate")

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("skip no accept-encoding", func(b *testing.B) {
		r := mux.NewRouter()
		r.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("not compressed"))
		}).Methods(http.MethodGet)

		mw, err := CompressionMiddleware(CompressionConfig{})
		if err != nil {
			b.Fatal(err)
		}
		r.Use(mw)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		b.ResetTimer()
		for b.Loop() {
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
