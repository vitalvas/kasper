package csrf

import (
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
	"github.com/vitalvas/kasper/securecookie"
)

func newTestKey(t *testing.T) []byte {
	t.Helper()
	k, err := securecookie.GenerateKey(32)
	require.NoError(t, err)
	return k
}

func newRouter(t *testing.T, cfg Config, h http.HandlerFunc) http.Handler {
	t.Helper()
	mw := Middleware(cfg)
	return mw(h)
}

func extractCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	resp := w.Result()
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// issueTokenCookie runs a GET through mw to obtain a valid CSRF cookie and
// the masked token captured during that request.
func issueTokenCookie(t *testing.T, mw mux.MiddlewareFunc, getURL string) (*http.Cookie, string) {
	t.Helper()
	var captured string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, getURL, nil))
	cookie := extractCookie(t, w, "csrf_token")
	require.NotNil(t, cookie)
	require.NotEmpty(t, captured)
	return cookie, captured
}

func TestSafeMethodsBypass(t *testing.T) {
	key := newTestKey(t)
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
		t.Run(m, func(t *testing.T) {
			h := newRouter(t, Config{Key: key}, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			r := httptest.NewRequest(m, "/", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.NotNil(t, extractCookie(t, w, "csrf_token"), "cookie should be issued on safe methods")
		})
	}
}

func TestEagerCookieIssuance(t *testing.T) {
	key := newTestKey(t)
	h := newRouter(t, Config{Key: key}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	c := extractCookie(t, w, "csrf_token")
	require.NotNil(t, c)
	assert.NotEmpty(t, c.Value)
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
}

func TestLazyCookieIssuance(t *testing.T) {
	key := newTestKey(t)

	t.Run("no cookie when Token unused", func(t *testing.T) {
		h := newRouter(t, Config{Key: key, Lazy: true}, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Nil(t, extractCookie(t, w, "csrf_token"))
	})

	t.Run("cookie issued when Token called", func(t *testing.T) {
		h := newRouter(t, Config{Key: key, Lazy: true}, func(w http.ResponseWriter, r *http.Request) {
			_ = Token(r)
			w.WriteHeader(http.StatusOK)
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.NotNil(t, extractCookie(t, w, "csrf_token"))
	})
}

func TestUnsafeMethodWithoutCookie(t *testing.T) {
	key := newTestKey(t)
	h := newRouter(t, Config{Key: key}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "cookie not present")
}

func TestUnsafeMethodTokenRoundTrip(t *testing.T) {
	key := newTestKey(t)

	var captured string
	getHandler := newRouter(t, Config{Key: key}, func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	})
	getReq := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	getW := httptest.NewRecorder()
	getHandler.ServeHTTP(getW, getReq)
	cookie := extractCookie(t, getW, "csrf_token")
	require.NotNil(t, cookie)
	require.NotEmpty(t, captured)

	postHandler := newRouter(t, Config{Key: key}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("Origin", "https://example.com")
	postReq.Header.Set("X-CSRF-Token", captured)
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postHandler.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusOK, postW.Code)
}

func TestMaskedTokenRotates(t *testing.T) {
	key := newTestKey(t)
	var t1, t2 string
	h := newRouter(t, Config{Key: key}, func(w http.ResponseWriter, r *http.Request) {
		t1 = Token(r)
		t2 = Token(r)
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.NotEmpty(t, t1)
	assert.NotEmpty(t, t2)
	assert.NotEqual(t, t1, t2, "masked tokens should rotate per call")
	assert.Equal(t, unmask(t1), unmask(t2), "but unmask to the same raw token")
}

func TestFormFieldSubmission(t *testing.T) {
	key := newTestKey(t)

	var captured string
	var capturedCookie *http.Cookie
	mw := Middleware(Config{Key: key})
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	capturedCookie = extractCookie(t, getW, "csrf_token")
	require.NotNil(t, capturedCookie)
	require.NotEmpty(t, captured)

	form := url.Values{}
	form.Set("csrf_token", captured)
	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("Origin", "https://example.com")
	postReq.AddCookie(capturedCookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusOK, postW.Code)
}

func TestOriginValidation(t *testing.T) {
	key := newTestKey(t)
	cfg := Config{
		Key:            key,
		TrustedOrigins: []string{"https://trusted.example.com", "https://*.partner.com"},
	}

	tests := []struct {
		name       string
		host       string
		origin     string
		wantStatus int
	}{
		{"same-origin allowed", "example.com", "https://example.com", http.StatusOK},
		{"trusted origin allowed", "example.com", "https://trusted.example.com", http.StatusOK},
		{"wildcard subdomain allowed", "example.com", "https://api.partner.com", http.StatusOK},
		{"wildcard parent host rejected", "example.com", "https://partner.com", http.StatusForbidden},
		{"untrusted origin rejected", "example.com", "https://evil.com", http.StatusForbidden},
		{"null origin rejected", "example.com", "null", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := Middleware(cfg)
			var captured string
			var cookie *http.Cookie
			getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = Token(r)
				w.WriteHeader(http.StatusOK)
			}))
			getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/", tt.host), nil)
			getW := httptest.NewRecorder()
			getH.ServeHTTP(getW, getReq)
			cookie = extractCookie(t, getW, "csrf_token")
			require.NotNil(t, cookie)

			postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			postReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("https://%s/", tt.host), nil)
			postReq.Header.Set("X-CSRF-Token", captured)
			postReq.Header.Set("Origin", tt.origin)
			postReq.AddCookie(cookie)
			postW := httptest.NewRecorder()
			postH.ServeHTTP(postW, postReq)
			assert.Equal(t, tt.wantStatus, postW.Code)
		})
	}
}

func TestRefererFallbackHTTPS(t *testing.T) {
	key := newTestKey(t)
	cfg := Config{Key: key}

	t.Run("https without origin requires referer", func(t *testing.T) {
		mw := Middleware(cfg)
		var captured string
		var cookie *http.Cookie
		getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = Token(r)
			w.WriteHeader(http.StatusOK)
		}))
		getW := httptest.NewRecorder()
		getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
		cookie = extractCookie(t, getW, "csrf_token")
		require.NotNil(t, cookie)

		postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
		postReq.Header.Set("X-CSRF-Token", captured)
		postReq.AddCookie(cookie)
		postW := httptest.NewRecorder()
		postH.ServeHTTP(postW, postReq)
		assert.Equal(t, http.StatusForbidden, postW.Code)
		assert.Contains(t, postW.Body.String(), "referer missing")
	})

	t.Run("https with valid referer allowed", func(t *testing.T) {
		mw := Middleware(cfg)
		var captured string
		var cookie *http.Cookie
		getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = Token(r)
			w.WriteHeader(http.StatusOK)
		}))
		getW := httptest.NewRecorder()
		getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
		cookie = extractCookie(t, getW, "csrf_token")

		postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
		postReq.Header.Set("X-CSRF-Token", captured)
		postReq.Header.Set("Referer", "https://example.com/page")
		postReq.AddCookie(cookie)
		postW := httptest.NewRecorder()
		postH.ServeHTTP(postW, postReq)
		assert.Equal(t, http.StatusOK, postW.Code)
	})

	t.Run("http without origin/referer is allowed", func(t *testing.T) {
		mw := Middleware(cfg)
		var captured string
		var cookie *http.Cookie
		getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = Token(r)
			w.WriteHeader(http.StatusOK)
		}))
		getW := httptest.NewRecorder()
		getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
		cookie = extractCookie(t, getW, "csrf_token")

		postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		postReq := httptest.NewRequest(http.MethodPost, "http://example.com/", nil)
		postReq.Header.Set("X-CSRF-Token", captured)
		postReq.AddCookie(cookie)
		postW := httptest.NewRecorder()
		postH.ServeHTTP(postW, postReq)
		assert.Equal(t, http.StatusOK, postW.Code)
	})
}

func TestTokenMismatch(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie := extractCookie(t, getW, "csrf_token")

	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("Origin", "https://example.com")
	postReq.Header.Set("X-CSRF-Token", "tampered-token-value")
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
}

func TestRotate(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var first, second string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first = unmask(Token(r))
		Rotate(w, r)
		second = unmask(Token(r))
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	assert.NotEmpty(t, first)
	assert.NotEmpty(t, second)
	assert.NotEqual(t, first, second, "Rotate should change the cookie token")
}

func TestTemplateField(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var html string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html = string(TemplateField(r))
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Contains(t, html, `<input type="hidden"`)
	assert.Contains(t, html, `name="csrf_token"`)
	assert.Contains(t, html, `value="`)
}

func TestCustomErrorHandler(t *testing.T) {
	key := newTestKey(t)
	var capturedErr error
	mw := Middleware(Config{
		Key: key,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, reason error) {
			capturedErr = reason
			w.WriteHeader(http.StatusTeapot)
		},
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusTeapot, w.Code)
	assert.True(t, errors.Is(capturedErr, ErrNoCookie))
}

func TestInvalidConfig(t *testing.T) {
	t.Run("invalid trusted origin", func(t *testing.T) {
		mw := Middleware(Config{Key: newTestKey(t), TrustedOrigins: []string{"not-a-url"}})
		w := httptest.NewRecorder()
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("invalid key size", func(t *testing.T) {
		mw := Middleware(Config{Key: []byte("too-short")})
		w := httptest.NewRecorder()
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestTokenOutsideMiddleware(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Empty(t, Token(r))
	assert.Empty(t, TemplateField(r))
}

func TestMaskUnmaskRoundTrip(t *testing.T) {
	raw, err := newRawToken()
	require.NoError(t, err)
	masked := mask(raw)
	assert.NotEqual(t, raw, masked)
	assert.Equal(t, raw, unmask(masked))
}

func TestUnmaskRejectsMalformed(t *testing.T) {
	assert.Empty(t, unmask("!!!not-base64"))
	assert.Empty(t, unmask("AA"))
}

func TestRotateOutsideMiddleware(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	Rotate(w, r) // must not panic and must do nothing
	assert.Empty(t, w.Header().Get("Set-Cookie"))
}

func TestExplicitCookieSecureFalse(t *testing.T) {
	key := newTestKey(t)
	insecure := false
	mw := Middleware(Config{Key: key, CookieSecure: &insecure})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	c := extractCookie(t, w, "csrf_token")
	require.NotNil(t, c)
	assert.False(t, c.Secure, "Secure should be false when explicitly disabled")
}

func TestRequestSchemeViaTLS(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// httptest.NewRequest("https://...", ...) sets r.TLS via the URL scheme
	// only when explicitly built; to exercise the r.TLS branch in
	// requestScheme we craft a request with empty URL.Scheme but TLS set.
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.URL.Scheme = ""
	r.TLS = &tls.ConnectionState{}
	r.Host = "example.com"
	r.Header.Set("X-CSRF-Token", "x")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	// No Origin/Referer over HTTPS -> should fail with referer missing.
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "referer missing")
}

func TestRequestSchemePlainHTTPDefault(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var captured string
	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getReq.URL.Scheme = ""
	getReq.TLS = nil
	getReq.Host = "example.com"
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, getReq)
	cookie = extractCookie(t, getW, "csrf_token")
	require.NotNil(t, cookie)

	// Plain HTTP without Origin/Referer: should pass.
	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.URL.Scheme = ""
	postReq.TLS = nil
	postReq.Host = "example.com"
	postReq.Header.Set("X-CSRF-Token", captured)
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusOK, postW.Code)
}

func TestRefererRejected(t *testing.T) {
	mw := Middleware(Config{Key: newTestKey(t)})
	cookie, captured := issueTokenCookie(t, mw, "https://example.com/")

	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("X-CSRF-Token", captured)
	postReq.Header.Set("Referer", "https://evil.com/page")
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
	assert.Contains(t, postW.Body.String(), "referer not trusted")
}

func TestMalformedRefererURL(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var captured string
	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, getW, "csrf_token")

	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("X-CSRF-Token", captured)
	postReq.Header.Set("Referer", "::not-a-url::")
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
}

func TestSchemeMismatchInOriginPattern(t *testing.T) {
	// http://trusted.example.com is NOT trusted when only https is listed.
	key := newTestKey(t)
	mw := Middleware(Config{
		Key:            key,
		TrustedOrigins: []string{"https://trusted.example.com"},
	})

	var captured string
	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, getW, "csrf_token")

	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("X-CSRF-Token", captured)
	postReq.Header.Set("Origin", "http://trusted.example.com") // wrong scheme
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
}

func TestCorruptedCookieIsTreatedAsMissing(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Send a request with a corrupted CSRF cookie. The middleware should
	// treat it as missing and either issue a fresh one (safe method) or
	// reject the request (unsafe method).
	r := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	r.Header.Set("Origin", "https://example.com")
	r.Header.Set("X-CSRF-Token", "anything")
	r.AddCookie(&http.Cookie{Name: "csrf_token", Value: "garbage-value-not-a-securecookie"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "cookie not present")
}

func TestCookieFromDifferentKeyRejected(t *testing.T) {
	keyA := newTestKey(t)
	keyB := newTestKey(t)

	// Issue a cookie under key A, then try to use it with middleware
	// configured with key B. Decode should fail and the cookie is treated
	// as missing.
	mwA := Middleware(Config{Key: keyA})
	getH := mwA(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/", nil))
	cookie := extractCookie(t, getW, "csrf_token")
	require.NotNil(t, cookie)

	mwB := Middleware(Config{Key: keyB})
	postH := mwB(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("Origin", "https://example.com")
	postReq.Header.Set("X-CSRF-Token", "x")
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
}

func TestRequestSchemeRawTLS(t *testing.T) {
	// Construct a raw http.Request to bypass httptest auto-population of
	// URL.Scheme, exercising the r.TLS branch in requestScheme.
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := &http.Request{
		Method: http.MethodPost,
		Host:   "example.com",
		URL:    &url.URL{Path: "/"},
		Header: http.Header{},
		TLS:    &tls.ConnectionState{},
	}
	r.Header.Set("X-CSRF-Token", "x")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	// HTTPS without Origin and without Referer -> rejected.
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "referer missing")
}

func TestRequestSchemeRawNoTLS(t *testing.T) {
	// Plain HTTP (no URL.Scheme, no TLS) -> referer missing is allowed.
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var captured string
	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getR := &http.Request{
		Method: http.MethodGet,
		Host:   "example.com",
		URL:    &url.URL{Path: "/"},
		Header: http.Header{},
	}
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, getR)
	cookie = extractCookie(t, getW, "csrf_token")
	require.NotNil(t, cookie)

	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postR := &http.Request{
		Method: http.MethodPost,
		Host:   "example.com",
		URL:    &url.URL{Path: "/"},
		Header: http.Header{},
	}
	postR.Header.Set("X-CSRF-Token", captured)
	postR.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postR)
	assert.Equal(t, http.StatusOK, postW.Code)
}

func TestRandFailureOnIssue(t *testing.T) {
	original := randRead
	randRead = func(_ []byte) (int, error) { return 0, errors.New("rng failed") }
	t.Cleanup(func() { randRead = original })

	key := newTestKey(t)
	mw := Middleware(Config{Key: key})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	// On safe method with rng failure during eager issuance, the error
	// handler is invoked with a 403.
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRandFailureOnRotate(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	original := randRead
	t.Cleanup(func() { randRead = original })

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Now break randomness and call Rotate -- should silently no-op.
		randRead = func(_ []byte) (int, error) { return 0, errors.New("rng failed") }
		Rotate(w, r)
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRandFailureOnLazyToken(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key, Lazy: true})

	original := randRead
	t.Cleanup(func() { randRead = original })

	var got string
	var gotField template.HTML
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		randRead = func(_ []byte) (int, error) { return 0, errors.New("rng failed") }
		got = Token(r)
		gotField = TemplateField(r)
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Empty(t, got)
	assert.Empty(t, gotField)
}

func TestRandFailureOnMask(t *testing.T) {
	// First generate a real cookie under normal randomness, then break
	// randomness and trigger mask via Token.
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	original := randRead
	t.Cleanup(func() { randRead = original })

	var got string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Cookie already issued by middleware (eager). Break rand for mask.
		randRead = func(_ []byte) (int, error) { return 0, errors.New("rng failed") }
		got = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Empty(t, got, "Token should return empty when masking RNG fails")
}

func TestNoTokenSubmittedWithValidCookie(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, getW, "csrf_token")
	require.NotNil(t, cookie)

	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("Origin", "https://example.com")
	postReq.AddCookie(cookie) // valid cookie but no token submitted
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
	assert.Contains(t, postW.Body.String(), "token not submitted")
}

func TestSubmittedTokenWithUnmaskableButCorrectLength(t *testing.T) {
	// Build a header value that decodes successfully but yields a raw
	// token that does NOT match the cookie. This exercises the
	// constant-time compare branch (currently masked off because
	// unmaskOrEmpty short-circuits on bad input).
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, getW, "csrf_token")

	// Generate a different valid masked token (different raw token).
	otherRaw, err := newRawToken()
	require.NoError(t, err)
	otherMasked := mask(otherRaw)

	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("Origin", "https://example.com")
	postReq.Header.Set("X-CSRF-Token", otherMasked)
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
	assert.Contains(t, postW.Body.String(), "token mismatch")
}

// TestTrustedOriginSchemeStrict is a regression test for CVE-2025-47909,
// where a trusted-origin allowlist that matches only on host and ignores
// the scheme allows a network attacker on http:// to forge a valid Origin
// for an https:// site. The matcher here must compare both scheme and host.
func TestTrustedOriginSchemeStrict(t *testing.T) {
	key := newTestKey(t)
	mw := Middleware(Config{
		Key:            key,
		TrustedOrigins: []string{"https://trusted.example.com"},
	})

	var captured string
	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, getW, "csrf_token")

	tests := []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{"https trusted accepted", "https://trusted.example.com", http.StatusOK},
		{"http trusted rejected (CVE-2025-47909)", "http://trusted.example.com", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
			postReq.Header.Set("X-CSRF-Token", captured)
			postReq.Header.Set("Origin", tt.origin)
			postReq.AddCookie(cookie)
			postW := httptest.NewRecorder()
			postH.ServeHTTP(postW, postReq)
			assert.Equal(t, tt.wantStatus, postW.Code)
		})
	}
}

// TestOriginNullRejected is a regression test for the upstream review
// finding: RFC 6454 §7.3 specifies "null" as a privacy-sensitive origin
// (sandboxed iframes, redirected POSTs). When present, kasper/csrf rejects
// it explicitly regardless of scheme. Previously the value was treated as
// "missing", which let HTTP requests pass without any check.
func TestOriginNullRejected(t *testing.T) {
	mw := Middleware(Config{Key: newTestKey(t)})

	tests := []struct {
		name string
		url  string
	}{
		{"https request", "https://example.com/"},
		{"http request", "http://example.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cookie, captured := issueTokenCookie(t, mw, tt.url)
			postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			postReq := httptest.NewRequest(http.MethodPost, tt.url, nil)
			postReq.Header.Set("X-CSRF-Token", captured)
			postReq.Header.Set("Origin", "null")
			postReq.AddCookie(cookie)
			postW := httptest.NewRecorder()
			postH.ServeHTTP(postW, postReq)
			assert.Equal(t, http.StatusForbidden, postW.Code)
			assert.Contains(t, postW.Body.String(), "origin not trusted")
		})
	}
}

// TestWildcardSingleLabelOnly is a regression test for the upstream review
// finding: "https://*.example.com" must match exactly one DNS label
// (api.example.com), not deeper paths (a.b.example.com). Previously the
// matcher used a plain HasSuffix check and over-matched.
//
// The serving host is unrelated to the trusted-origin pattern so the
// same-origin check does not interfere; only the wildcard matcher decides.
func TestWildcardSingleLabelOnly(t *testing.T) {
	mw := Middleware(Config{
		Key:            newTestKey(t),
		TrustedOrigins: []string{"https://*.example.com"},
	})
	cookie, captured := issueTokenCookie(t, mw, "https://app.local/")

	tests := []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{"single label allowed", "https://api.example.com", http.StatusOK},
		{"two labels rejected", "https://a.b.example.com", http.StatusForbidden},
		{"three labels rejected", "https://x.y.z.example.com", http.StatusForbidden},
		{"apex rejected", "https://example.com", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			postReq := httptest.NewRequest(http.MethodPost, "https://app.local/", nil)
			postReq.Header.Set("X-CSRF-Token", captured)
			postReq.Header.Set("Origin", tt.origin)
			postReq.AddCookie(cookie)
			postW := httptest.NewRecorder()
			postH.ServeHTTP(postW, postReq)
			assert.Equal(t, tt.wantStatus, postW.Code, "origin=%s", tt.origin)
		})
	}
}

func TestTrustedOriginFunc(t *testing.T) {
	key := newTestKey(t)
	called := 0
	mw := Middleware(Config{
		Key: key,
		// Allow any *.preview.app.dev (Vercel-style) hostname.
		TrustedOriginFunc: func(u *url.URL) bool {
			called++
			return u.Scheme == "https" && strings.HasSuffix(u.Host, ".preview.app.dev")
		},
	})

	var captured string
	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, getW, "csrf_token")

	tests := []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{"matching dynamic origin allowed", "https://app-git-feat-team.preview.app.dev", http.StatusOK},
		{"non-matching origin rejected", "https://evil.com", http.StatusForbidden},
		{"http variant rejected by func", "http://app-git-feat.preview.app.dev", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
			postReq.Header.Set("X-CSRF-Token", captured)
			postReq.Header.Set("Origin", tt.origin)
			postReq.AddCookie(cookie)
			postW := httptest.NewRecorder()
			postH.ServeHTTP(postW, postReq)
			assert.Equal(t, tt.wantStatus, postW.Code)
		})
	}
	assert.Greater(t, called, 0, "TrustedOriginFunc should be consulted")
}

func TestValidate(t *testing.T) {
	// Validate is for use outside the middleware chain (e.g., WebSocket
	// upgrades). The middleware itself rejects invalid requests before
	// the handler runs, so inside a normal handler Validate always
	// returns nil. To exercise the error paths we use ErrorHandler to
	// allow the request through and observe what Validate would say if
	// it were invoked from a hijacked / upgrade context.
	key := newTestKey(t)

	// Issue a cookie under a permissive middleware so we have material
	// to validate against.
	issuer := Middleware(Config{Key: key})
	var captured string
	var cookie *http.Cookie
	issuerH := issuer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	issuerW := httptest.NewRecorder()
	issuerH.ServeHTTP(issuerW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, issuerW, "csrf_token")
	require.NotNil(t, cookie)

	// Permissive middleware: log validation failures into a captured
	// error but always continue to the handler. This simulates a flow
	// where the caller wants to drive their own validation via Validate.
	var observed error
	permissive := Middleware(Config{
		Key: key,
		ErrorHandler: func(_ http.ResponseWriter, r *http.Request, _ error) {
			observed = Validate(r)
		},
	})

	t.Run("valid request passes", func(t *testing.T) {
		observed = errors.New("not called")
		var got error
		h := permissive(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = Validate(r)
		}))
		req := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-CSRF-Token", captured)
		req.AddCookie(cookie)
		h.ServeHTTP(httptest.NewRecorder(), req)
		assert.NoError(t, got)
	})

	t.Run("missing token returns ErrNoToken", func(t *testing.T) {
		observed = nil
		h := permissive(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
		req.Header.Set("Origin", "https://example.com")
		req.AddCookie(cookie)
		h.ServeHTTP(httptest.NewRecorder(), req)
		assert.ErrorIs(t, observed, ErrNoToken)
	})

	t.Run("untrusted origin returns ErrOriginRejected", func(t *testing.T) {
		observed = nil
		h := permissive(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
		req.Header.Set("Origin", "https://evil.com")
		req.Header.Set("X-CSRF-Token", captured)
		req.AddCookie(cookie)
		h.ServeHTTP(httptest.NewRecorder(), req)
		assert.ErrorIs(t, observed, ErrOriginRejected)
	})

	t.Run("outside middleware returns ErrNoCookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		err := Validate(r)
		assert.ErrorIs(t, err, ErrNoCookie)
	})
}

func TestTokenAvailableInErrorHandler(t *testing.T) {
	// SPA bootstrap pattern: the error handler must be able to surface a
	// token (e.g., return JSON with the new token) so the client can
	// recover from a 403.
	key := newTestKey(t)
	var tokenInHandler string
	mw := Middleware(Config{
		Key: key,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			tokenInHandler = Token(r)
			w.WriteHeader(http.StatusForbidden)
		},
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotEmpty(t, tokenInHandler, "Token should be retrievable in ErrorHandler for SPA bootstrap")
}

func TestProxyHeadersIntegration(t *testing.T) {
	// Simulate what muxhandlers.ProxyHeadersMiddleware does: rewrite
	// r.URL.Scheme and r.Host from X-Forwarded-* before the CSRF
	// middleware runs. Verify same-origin checks honor the rewritten
	// values.
	key := newTestKey(t)
	mw := Middleware(Config{Key: key})

	var captured string
	var cookie *http.Cookie
	getH := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	getW := httptest.NewRecorder()
	getH.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	cookie = extractCookie(t, getW, "csrf_token")

	// Backend receives an HTTP request from the proxy; ProxyHeaders
	// rewrites scheme/host to match the public-facing values.
	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.URL.Scheme = "https"
	postReq.Host = "example.com"
	postReq.Header.Set("Origin", "https://example.com")
	postReq.Header.Set("X-CSRF-Token", captured)
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusOK, postW.Code, "same-origin check should honor rewritten URL.Scheme")
}

func TestParseOriginPatternErrors(t *testing.T) {
	tests := []string{
		"://nohost",   // no scheme/host
		"justastring", // missing scheme
		"http://",     // empty host
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			_, err := parseOriginPattern(in)
			assert.Error(t, err)
		})
	}
}

func TestSecFetchSite(t *testing.T) {
	mw := Middleware(Config{Key: newTestKey(t)})
	cookie, captured := issueTokenCookie(t, mw, "https://example.com/")

	tests := []struct {
		name       string
		fetchSite  string
		origin     string
		wantStatus int
	}{
		{"same-origin accepted (token still required)", "same-origin", "", http.StatusOK},
		{"none accepted (user-initiated)", "none", "", http.StatusOK},
		{"cross-site rejected", "cross-site", "https://example.com", http.StatusForbidden},
		{"same-site rejected (subdomain takeover risk)", "same-site", "https://example.com", http.StatusForbidden},
		{"missing falls back to Origin (allowed)", "", "https://example.com", http.StatusOK},
		{"missing falls back to Origin (rejected)", "", "https://evil.com", http.StatusForbidden},
		{"unknown value falls back to Origin", "future-value", "https://example.com", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
			postReq.Header.Set("X-CSRF-Token", captured)
			if tt.fetchSite != "" {
				postReq.Header.Set("Sec-Fetch-Site", tt.fetchSite)
			}
			if tt.origin != "" {
				postReq.Header.Set("Origin", tt.origin)
			}
			postReq.AddCookie(cookie)
			postW := httptest.NewRecorder()
			postH.ServeHTTP(postW, postReq)
			assert.Equal(t, tt.wantStatus, postW.Code)
		})
	}
}

func TestSecFetchSiteStillRequiresToken(t *testing.T) {
	mw := Middleware(Config{Key: newTestKey(t)})
	cookie, _ := issueTokenCookie(t, mw, "https://example.com/")

	// same-origin trusts the origin layer but the token is a separate
	// defense; missing token must still produce ErrNoToken.
	postH := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	postReq.Header.Set("Sec-Fetch-Site", "same-origin")
	postReq.AddCookie(cookie)
	postW := httptest.NewRecorder()
	postH.ServeHTTP(postW, postReq)
	assert.Equal(t, http.StatusForbidden, postW.Code)
	assert.Contains(t, postW.Body.String(), "token not submitted")
}

func TestVaryHeaderOnRejection(t *testing.T) {
	mw := Middleware(Config{Key: newTestKey(t)})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	r.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	require.Equal(t, http.StatusForbidden, w.Code)
	vary := w.Result().Header.Values("Vary")
	assert.Contains(t, vary, "Sec-Fetch-Site")
	assert.Contains(t, vary, "Origin")
	assert.Contains(t, vary, "Cookie")
}

func TestHostCookiePrefixEnforcement(t *testing.T) {
	key := newTestKey(t)

	tests := []struct {
		name      string
		cfg       Config
		shouldErr bool
	}{
		{
			name:      "valid __Host- config",
			cfg:       Config{Key: key, CookieName: "__Host-csrf"},
			shouldErr: false,
		},
		{
			name: "__Host- with explicit Secure=false rejected",
			cfg: func() Config {
				insecure := false
				return Config{Key: key, CookieName: "__Host-csrf", CookieSecure: &insecure}
			}(),
			shouldErr: true,
		},
		{
			name:      "__Host- with Domain rejected",
			cfg:       Config{Key: key, CookieName: "__Host-csrf", CookieDomain: "example.com"},
			shouldErr: true,
		},
		{
			name:      "__Host- with non-root Path rejected",
			cfg:       Config{Key: key, CookieName: "__Host-csrf", CookiePath: "/app"},
			shouldErr: true,
		},
		{
			name: "__Secure- with Secure=false rejected",
			cfg: func() Config {
				insecure := false
				return Config{Key: key, CookieName: "__Secure-csrf", CookieSecure: &insecure}
			}(),
			shouldErr: true,
		},
		{
			name:      "__Secure- with default Secure=true accepted",
			cfg:       Config{Key: key, CookieName: "__Secure-csrf"},
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := Middleware(tt.cfg)
			h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
			if tt.shouldErr {
				assert.Equal(t, http.StatusInternalServerError, w.Code)
			} else {
				assert.Equal(t, http.StatusOK, w.Code)
				c := extractCookie(t, w, tt.cfg.CookieName)
				require.NotNil(t, c)
				assert.True(t, c.Secure)
			}
		})
	}
}

// --- Fuzz tests ---

// FuzzValidate exercises Validate with arbitrary header inputs to confirm
// no panic path exists for malformed Origin/Referer/header values.
func FuzzValidate(f *testing.F) {
	key, _ := securecookie.GenerateKey(32)
	mw := Middleware(Config{
		Key:            key,
		TrustedOrigins: []string{"https://trusted.example.com", "https://*.partner.com"},
		ErrorHandler:   func(http.ResponseWriter, *http.Request, error) {},
	})

	// Pre-issue a cookie under permissive middleware so Validate has
	// material to work with. We capture both the cookie (for re-attach)
	// and a valid masked token.
	cookie, validToken := issueTokenCookieF(mw)

	f.Add("https://example.com", "https://example.com", "", validToken)
	f.Add("null", "", "", validToken)
	f.Add("", "https://example.com/page", "", validToken)
	f.Add("", "", "", "")
	f.Add("https://evil.com", "", "", validToken)
	f.Add("\x00\x00", "\x00\x00", "::not-a-url::", "garbage")
	f.Add("https://example.com:65536", "", "", validToken)

	f.Fuzz(func(_ *testing.T, origin, referer, host, token string) {
		req := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		if host != "" {
			req.Host = host
		}
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		req.AddCookie(cookie)

		// Run through middleware so request state is attached.
		var observed error
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			observed = Validate(r)
		}))
		h.ServeHTTP(httptest.NewRecorder(), req)
		_ = observed
	})
}

// FuzzMaskUnmask confirms unmask never panics on arbitrary input and
// round-trips correctly when given valid mask output.
func FuzzMaskUnmask(f *testing.F) {
	f.Add("")
	f.Add("AAAA")
	f.Add("not-base64-!!!")
	if raw, err := newRawToken(); err == nil {
		f.Add(mask(raw))
	}

	f.Fuzz(func(_ *testing.T, s string) {
		_ = unmask(s)
	})
}

// FuzzParseOriginPattern confirms parseOriginPattern never panics.
func FuzzParseOriginPattern(f *testing.F) {
	f.Add("https://example.com")
	f.Add("https://*.example.com")
	f.Add("")
	f.Add("not-a-url")
	f.Add("\x00\x00\x00")
	f.Add("https://[::1]:8080")

	f.Fuzz(func(_ *testing.T, s string) {
		p, err := parseOriginPattern(s)
		if err == nil && p != nil {
			_ = p.match("https", "example.com")
		}
	})
}

// issueTokenCookieF is a benchmark/fuzz helper (no *testing.T).
func issueTokenCookieF(mw mux.MiddlewareFunc) (*http.Cookie, string) {
	var captured string
	var cookie *http.Cookie
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = Token(r)
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://example.com/", nil))
	for _, c := range w.Result().Cookies() {
		if c.Name == "csrf_token" {
			cookie = c
			break
		}
	}
	return cookie, captured
}

// --- Benchmarks ---

func BenchmarkMiddlewareHappyPath(b *testing.B) {
	key, _ := securecookie.GenerateKey(32)
	mw := Middleware(Config{Key: key})
	cookie, token := issueTokenCookieF(mw)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-CSRF-Token", token)
		req.AddCookie(cookie)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkMiddlewareIssueCookie(b *testing.B) {
	key, _ := securecookie.GenerateKey(32)
	mw := Middleware(Config{Key: key})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkToken(b *testing.B) {
	key, _ := securecookie.GenerateKey(32)
	mw := Middleware(Config{Key: key})

	// Build a request with state attached once.
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	var capturedReq *http.Request
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), req)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Token(capturedReq)
	}
}

func BenchmarkValidate(b *testing.B) {
	key, _ := securecookie.GenerateKey(32)
	mw := Middleware(Config{
		Key:          key,
		ErrorHandler: func(http.ResponseWriter, *http.Request, error) {},
	})
	cookie, token := issueTokenCookieF(mw)

	var capturedReq *http.Request
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	preReq := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	preReq.Header.Set("Origin", "https://example.com")
	preReq.Header.Set("X-CSRF-Token", token)
	preReq.AddCookie(cookie)
	h.ServeHTTP(httptest.NewRecorder(), preReq)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Validate(capturedReq)
	}
}

func BenchmarkMask(b *testing.B) {
	raw, _ := newRawToken()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = mask(raw)
	}
}

func BenchmarkUnmask(b *testing.B) {
	raw, _ := newRawToken()
	masked := mask(raw)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = unmask(masked)
	}
}
