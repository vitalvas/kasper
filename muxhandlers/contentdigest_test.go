package muxhandlers

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func digestHeaderFor(body string, alg DigestAlgorithm) string {
	var sum []byte
	if alg == DigestSHA512 {
		h := sha512.Sum512([]byte(body))
		sum = h[:]
	} else {
		h := sha256.Sum256([]byte(body))
		sum = h[:]
	}
	return fmt.Sprintf("%s=:%s:", alg, base64.StdEncoding.EncodeToString(sum))
}

func TestContentDigestMiddlewareConfig(t *testing.T) {
	t.Run("rejects missing algorithm", func(t *testing.T) {
		_, err := ContentDigestMiddleware(ContentDigestConfig{})
		require.ErrorIs(t, err, ErrNoDigestAlgorithm)
	})
	t.Run("rejects unsupported algorithm", func(t *testing.T) {
		_, err := ContentDigestMiddleware(ContentDigestConfig{Algorithm: "md5"})
		require.ErrorIs(t, err, ErrNoDigestAlgorithm)
	})
}

func TestContentDigestResponse(t *testing.T) {
	for _, alg := range []DigestAlgorithm{DigestSHA256, DigestSHA512} {
		t.Run(string(alg), func(t *testing.T) {
			mw, err := ContentDigestMiddleware(ContentDigestConfig{Algorithm: alg})
			require.NoError(t, err)

			const body = `{"ok":true}`
			h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(body))
			}))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			assert.Equal(t, http.StatusCreated, rec.Code)
			assert.Equal(t, body, rec.Body.String())
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Equal(t, digestHeaderFor(body, alg), rec.Header().Get("Content-Digest"))
		})
	}
}

func TestContentDigestResponseDefaultStatus(t *testing.T) {
	mw, err := ContentDigestMiddleware(ContentDigestConfig{Algorithm: DigestSHA256})
	require.NoError(t, err)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, digestHeaderFor("hi", DigestSHA256), rec.Header().Get("Content-Digest"))
}

func TestContentDigestVerifyRequests(t *testing.T) {
	const body = `payload-data`

	newMW := func(require bool) func(http.Handler) http.Handler {
		mw, err := ContentDigestMiddleware(ContentDigestConfig{
			Algorithm:            DigestSHA256,
			VerifyRequests:       true,
			RequireRequestDigest: require,
		})
		if err != nil {
			t.Fatal(err)
		}
		return mw
	}

	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_, _ = w.Write(b)
	})

	t.Run("valid digest passes and body preserved", func(t *testing.T) {
		h := newMW(true)(echo)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Digest", digestHeaderFor(body, DigestSHA256))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, body, rec.Body.String())
	})

	t.Run("mismatch rejected", func(t *testing.T) {
		h := newMW(true)(echo)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Digest", digestHeaderFor("different", DigestSHA256))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing header rejected when required", func(t *testing.T) {
		h := newMW(true)(echo)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing header allowed when not required", func(t *testing.T) {
		h := newMW(false)(echo)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, body, rec.Body.String())
	})

	t.Run("malformed header rejected", func(t *testing.T) {
		h := newMW(true)(echo)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Digest", "not a valid dictionary!!")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("well-formed unsupported algorithm rejected", func(t *testing.T) {
		h := newMW(true)(echo)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Digest", "md5=:1B2M2Y8AsgTpgAmY7PhCfg==:")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("inner-list value rejected as malformed", func(t *testing.T) {
		h := newMW(true)(echo)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Digest", "sha-256=(:aGk=:)")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("nil body with matching empty digest", func(t *testing.T) {
		noRead := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		h := newMW(true)(noRead)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.Body = nil // exercise the nil-body path in readAndRestoreRequestBody
		req.Header.Set("Content-Digest", digestHeaderFor("", DigestSHA256))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestSetAndVerifyContentDigest(t *testing.T) {
	t.Run("set then verify round-trips", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
		require.NoError(t, SetContentDigest(req, DigestSHA256))
		assert.Equal(t, digestHeaderFor("hello", DigestSHA256), req.Header.Get("Content-Digest"))
		require.NoError(t, VerifyContentDigest(req))
		// Body remains readable after both operations.
		b, _ := io.ReadAll(req.Body)
		assert.Equal(t, "hello", string(b))
	})

	t.Run("set rejects unsupported algorithm", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("x"))
		require.ErrorIs(t, SetContentDigest(req, DigestAlgorithm("sha-1")), ErrDigestUnsupported)
	})

	t.Run("verify distinguishes error kinds", func(t *testing.T) {
		mk := func(h string) *http.Request {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
			if h != "" {
				req.Header.Set("Content-Digest", h)
			}
			return req
		}
		require.ErrorIs(t, VerifyContentDigest(mk("")), ErrDigestMissing)
		require.ErrorIs(t, VerifyContentDigest(mk("sha-256=notbytes")), ErrDigestMalformed)
		require.ErrorIs(t, VerifyContentDigest(mk("md5=:1B2M2Y8AsgTpgAmY7PhCfg==:")), ErrDigestUnsupported)
		require.ErrorIs(t, VerifyContentDigest(mk(digestHeaderFor("other", DigestSHA256))), ErrDigestMismatch)
	})
}

func TestContentDigestCustomOnError(t *testing.T) {
	called := false
	mw, err := ContentDigestMiddleware(ContentDigestConfig{
		Algorithm:            DigestSHA256,
		VerifyRequests:       true,
		RequireRequestDigest: true,
		OnError: func(w http.ResponseWriter, _ *http.Request, err error) {
			called = true
			assert.ErrorIs(t, err, ErrDigestMissing)
			w.WriteHeader(http.StatusForbidden)
		},
	})
	require.NoError(t, err)

	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("x"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestDigestResponseHeaders(t *testing.T) {
	mw, err := ContentDigestMiddleware(ContentDigestConfig{Algorithm: DigestSHA256})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Existing", "old")
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.Equal(t, "old", w.Header().Get("X-Existing"))
		w.Header().Set("X-Existing", "new")
		_, err := w.Write([]byte("hello"))
		require.NoError(t, err)
		w.Header().Set("X-Late", "ignored")
	})).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	res := rec.Result()
	defer res.Body.Close()
	require.Equal(t, []string{"new"}, res.Header.Values("X-Existing"))
	require.Empty(t, res.Header.Get("X-Late"))
	require.Equal(t, "text/plain; charset=utf-8", res.Header.Get("Content-Type"))
}

func TestDigestResponsesWithoutContent(t *testing.T) {
	mw, err := ContentDigestMiddleware(ContentDigestConfig{Algorithm: DigestSHA256})
	require.NoError(t, err)
	for _, status := range []int{200, 204, 304} {
		method := "GET"
		if status == 200 {
			method = "HEAD"
		}
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			n, err := w.Write([]byte("not sent"))
			if status == 200 {
				require.NoError(t, err)
				require.Equal(t, 8, n)
			} else {
				require.ErrorIs(t, err, http.ErrBodyNotAllowed)
			}
		})).ServeHTTP(rec, httptest.NewRequest(method, "/", nil))
		require.Equal(t, status, rec.Code)
		require.Empty(t, rec.Body.String())
		require.Equal(t, digestHeaderFor("", DigestSHA256), rec.Header().Get("Content-Digest"))
	}
}

func TestDigestMultipleFieldLines(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader("hello"))
	req.Header.Add("Content-Digest", "unknown=:AA==:")
	req.Header.Add("Content-Digest", digestHeaderFor("hello", DigestSHA256))
	require.NoError(t, VerifyContentDigest(req))
	req.Header.Add("Content-Digest", "other=notbytes")
	require.ErrorIs(t, VerifyContentDigest(req), ErrDigestMalformed)
	req.Header = nil
	require.NoError(t, SetContentDigest(req, DigestSHA256))
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(body))
}

func TestDigestInformationalResponseAndTrailers(t *testing.T) {
	mw, err := ContentDigestMiddleware(ContentDigestConfig{Algorithm: DigestSHA256})
	require.NoError(t, err)
	server := httptest.NewServer(mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", "</style.css>; rel=preload")
		w.WriteHeader(http.StatusEarlyHints)
		w.Header().Del("Link")
		w.Header().Set("Trailer", "X-Checksum")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
		w.Header().Set("X-Checksum", "done")
	})))
	defer server.Close()
	res, err := server.Client().Get(server.URL)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, res.StatusCode)
	require.Equal(t, "hello", string(body))
	require.Equal(t, "text/plain; charset=utf-8", res.Header.Get("Content-Type"))
	require.Equal(t, digestHeaderFor("hello", DigestSHA256), res.Header.Get("Content-Digest"))
	require.Equal(t, "done", res.Trailer.Get("X-Checksum"))
	require.Empty(t, res.Header.Get("Link"))
}

func TestResponseContentDigest(t *testing.T) {
	newResponse := func(body string) *http.Response {
		return &http.Response{Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	}

	t.Run("set and verify round trip restores body", func(t *testing.T) {
		resp := newResponse("hello")

		require.NoError(t, SetResponseContentDigest(resp, DigestSHA256))
		assert.Equal(t, digestHeaderFor("hello", DigestSHA256), resp.Header.Get("Content-Digest"))
		require.NoError(t, VerifyResponseContentDigest(resp))

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(body))
	})

	t.Run("unsupported algorithm returns error", func(t *testing.T) {
		assert.ErrorIs(t, SetResponseContentDigest(newResponse(""), "md5"), ErrDigestUnsupported)
	})

	t.Run("tampered body returns mismatch", func(t *testing.T) {
		resp := newResponse("hello")
		require.NoError(t, SetResponseContentDigest(resp, DigestSHA512))
		resp.Body = io.NopCloser(strings.NewReader("tampered"))

		assert.ErrorIs(t, VerifyResponseContentDigest(resp), ErrDigestMismatch)
	})

	t.Run("missing header returns error", func(t *testing.T) {
		assert.ErrorIs(t, VerifyResponseContentDigest(newResponse("x")), ErrDigestMissing)
	})

	t.Run("nil body digests empty content", func(t *testing.T) {
		resp := &http.Response{Header: make(http.Header)}

		require.NoError(t, SetResponseContentDigest(resp, DigestSHA256))
		require.NoError(t, VerifyResponseContentDigest(resp))
	})

	t.Run("nil header map is created on set", func(t *testing.T) {
		resp := &http.Response{Body: io.NopCloser(strings.NewReader("x"))}

		require.NoError(t, SetResponseContentDigest(resp, DigestSHA256))
		assert.NotEmpty(t, resp.Header.Get("Content-Digest"))
	})
}
