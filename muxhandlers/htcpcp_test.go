package muxhandlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fixedAprilFirst is an arbitrary instant on April 1 used to make the
// HTCPCP tests deterministic regardless of when they run.
var fixedAprilFirst = time.Date(2024, time.April, 1, 12, 0, 0, 0, time.UTC)

func newHTCPCPHandler(cfg HTCPCPConfig) http.Handler {
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return fixedAprilFirst }
	}
	mw := HTCPCPMiddleware(cfg)
	return mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Pass", "through")
		w.WriteHeader(http.StatusOK)
	}))
}

func TestHTCPCPMiddleware(t *testing.T) {
	t.Run("teapot returns 418 for coffee BREW", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotTeapot})
		req := httptest.NewRequest(MethodBrew, "/", strings.NewReader("start"))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusTeapot, w.Code)
		assert.Contains(t, w.Body.String(), "teapot")
	})

	t.Run("teapot brews supported tea variety", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotTeapot})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "tea-earl-grey")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, ContentTypeMessageTeapot, w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "earl-grey")
	})

	t.Run("teapot rejects unsupported tea variety with 406", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{
			PotType: PotTeapot,
			Teas:    []string{"earl-grey"},
		})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "tea-rooibos")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotAcceptable, w.Code)
	})

	t.Run("coffee pot brews coffee", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotCoffee})
		req := httptest.NewRequest(MethodBrew, "/", strings.NewReader("start"))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, ContentTypeMessageCoffeePot, w.Header().Get("Content-Type"))
	})

	t.Run("coffee pot refuses tea with 406", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotCoffee})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "tea-earl-grey")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotAcceptable, w.Code)
	})

	t.Run("empty pot returns 503 with Retry-After", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{
			PotType:    PotTeapot,
			Empty:      true,
			RetryAfter: 30,
		})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "tea-green")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "30", w.Header().Get("Retry-After"))
	})

	t.Run("empty pot defaults Retry-After to 60", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotTeapot, Empty: true})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "tea-green")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "60", w.Header().Get("Retry-After"))
	})

	t.Run("WHEN method is acknowledged with 200", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotCoffee})
		req := httptest.NewRequest(MethodWhen, "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, ContentTypeMessageCoffeePot, w.Header().Get("Content-Type"))
	})

	t.Run("addition not in AvailableAdditions returns 406", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{
			PotType:            PotCoffee,
			AvailableAdditions: []string{"milk-whole"},
		})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "milk-skim")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotAcceptable, w.Code)
	})

	t.Run("addition in AvailableAdditions brews successfully", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{
			PotType:            PotCoffee,
			AvailableAdditions: []string{"milk-whole", "sugar"},
		})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "milk-whole, sugar")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("non-HTCPCP methods pass through", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotCoffee})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "through", w.Header().Get("X-Pass"))
	})

	t.Run("teapot defaults to RFC 7168 tea registry", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotTeapot})
		for _, variety := range DefaultTeaVarieties {
			req := httptest.NewRequest(MethodBrew, "/", nil)
			req.Header.Set("Accept-Additions", fmt.Sprintf("tea-%s", variety))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code, "variety %q should be accepted", variety)
		}
	})

	t.Run("case-insensitive matching for teas and additions", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{
			PotType:            PotTeapot,
			AvailableAdditions: []string{"Milk-Whole"},
		})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "TEA-EARL-GREY, MILK-WHOLE")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("first tea variety wins when multiple are requested", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{
			PotType: PotTeapot,
			Teas:    []string{"earl-grey"},
		})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		// rooibos is not in Teas, but earl-grey is first.
		req.Header.Set("Accept-Additions", "tea-earl-grey, tea-rooibos")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "earl-grey")
	})

	t.Run("Accept-Additions parameters are stripped", func(t *testing.T) {
		h := newHTCPCPHandler(HTCPCPConfig{PotType: PotTeapot})
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "tea-earl-grey;q=0.9")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHTCPCPDateFilter(t *testing.T) {
	t.Run("BREW falls through to next on non-April-1", func(t *testing.T) {
		cfg := HTCPCPConfig{
			PotType: PotTeapot,
			Now:     func() time.Time { return time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC) },
		}
		h := newHTCPCPHandler(cfg)
		req := httptest.NewRequest(MethodBrew, "/", nil)
		req.Header.Set("Accept-Additions", "tea-earl-grey")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		// Middleware is a no-op; next handler runs and stamps X-Pass.
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "through", w.Header().Get("X-Pass"))
	})

	t.Run("BREW is active on April 1", func(t *testing.T) {
		cfg := HTCPCPConfig{
			PotType: PotTeapot,
			Now:     func() time.Time { return fixedAprilFirst },
		}
		h := newHTCPCPHandler(cfg)
		req := httptest.NewRequest(MethodBrew, "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("custom ActiveOn predicate is honored", func(t *testing.T) {
		cfg := HTCPCPConfig{
			PotType:  PotTeapot,
			ActiveOn: func(_ time.Time) bool { return true },
			Now:      func() time.Time { return time.Date(2024, time.December, 25, 0, 0, 0, 0, time.UTC) },
		}
		h := newHTCPCPHandler(cfg)
		req := httptest.NewRequest(MethodBrew, "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("non-HTCPCP methods pass through regardless of date", func(t *testing.T) {
		cfg := HTCPCPConfig{
			PotType: PotCoffee,
			Now:     func() time.Time { return time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC) },
		}
		h := newHTCPCPHandler(cfg)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "through", w.Header().Get("X-Pass"))
	})
}

func TestHTCPCPDefaultClock(t *testing.T) {
	// Build the middleware without overriding Now so the default
	// time.Now path is exercised. ActiveOn is forced to false so the
	// test is deterministic regardless of the wall-clock date.
	mw := HTCPCPMiddleware(HTCPCPConfig{
		PotType:  PotTeapot,
		ActiveOn: func(_ time.Time) bool { return false },
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot - 1) // 417, distinct from any HTCPCP code
	}))
	req := httptest.NewRequest(MethodBrew, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTeapot-1, w.Code)
}

func TestIsAprilFirst(t *testing.T) {
	t.Run("matches April 1", func(t *testing.T) {
		assert.True(t, IsAprilFirst(time.Date(2024, time.April, 1, 0, 0, 0, 0, time.UTC)))
		assert.True(t, IsAprilFirst(time.Date(2099, time.April, 1, 23, 59, 59, 0, time.UTC)))
	})

	t.Run("rejects other days", func(t *testing.T) {
		assert.False(t, IsAprilFirst(time.Date(2024, time.April, 2, 0, 0, 0, 0, time.UTC)))
		assert.False(t, IsAprilFirst(time.Date(2024, time.March, 31, 23, 59, 59, 0, time.UTC)))
		assert.False(t, IsAprilFirst(time.Date(2024, time.May, 1, 0, 0, 0, 0, time.UTC)))
	})
}

func TestHTCPCPParseAcceptAdditions(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		assert.Nil(t, parseAcceptAdditions(""))
	})

	t.Run("whitespace-only input returns nil", func(t *testing.T) {
		assert.Nil(t, parseAcceptAdditions("   "))
	})

	t.Run("trims and lowercases entries", func(t *testing.T) {
		got := parseAcceptAdditions("  Milk-Whole , SUGAR ")
		assert.Equal(t, []string{"milk-whole", "sugar"}, got)
	})

	t.Run("skips empty entries", func(t *testing.T) {
		got := parseAcceptAdditions("milk-whole,,sugar")
		assert.Equal(t, []string{"milk-whole", "sugar"}, got)
	})
}
