package muxhandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vitalvas/kasper/mux"
)

// gateHandler returns an http.Handler that signals it has entered via
// `entered`, waits for `release` before completing, and increments
// `seen` once it finishes. Used by tests to control in-flight timing.
func gateHandler(entered chan<- struct{}, release <-chan struct{}, seen *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		seen.Add(1)
		w.WriteHeader(http.StatusOK)
	})
}

func TestGracefulShutdownMiddleware(t *testing.T) {
	t.Run("before Drain: requests pass through", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		assert.False(t, drainer.IsDraining())

		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusTeapot, w.Code)
	})

	t.Run("after Drain: new requests get 503 with default headers", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		drainer.Drain()
		assert.True(t, drainer.IsDraining())

		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("handler must not run after Drain")
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "close", w.Header().Get("Connection"))
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, "text/plain; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "Service Unavailable", w.Body.String())
		assert.Empty(t, w.Header().Get("Retry-After"))
	})

	t.Run("RetryAfter Duration emits delta-seconds", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{
			RetryAfter: 45 * time.Second,
		})
		drainer.Drain()
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "45", w.Header().Get("Retry-After"))
	})

	t.Run("sub-second RetryAfter rounds up to 1", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{
			RetryAfter: 500 * time.Millisecond,
		})
		drainer.Drain()
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, "1", w.Header().Get("Retry-After"))
	})

	t.Run("custom StatusCode is honored", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{
			StatusCode: http.StatusGone,
		})
		drainer.Drain()
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusGone, w.Code)
		assert.Equal(t, "Gone", w.Body.String())
	})

	t.Run("unknown StatusCode falls back to Service Unavailable text", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{
			StatusCode: 599,
		})
		drainer.Drain()
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, 599, w.Code)
		assert.Equal(t, "Service Unavailable", w.Body.String())
	})

	t.Run("custom Response handler fully owns the body", func(t *testing.T) {
		response := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"draining":true}`))
		})

		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{
			Response: response,
			// Connection: close and Cache-Control: no-store should be
			// applied before the custom handler runs.
		})
		drainer.Drain()
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Equal(t, `{"draining":true}`, w.Body.String())
		assert.Equal(t, "close", w.Header().Get("Connection"))
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})

	t.Run("Bypass forwards selected requests during drain", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{
			Bypass: func(_ *mux.Router, r *http.Request) bool { return r.URL.Path == "/healthz" },
		})
		drainer.Drain()

		var seen int32
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&seen, 1)
			w.WriteHeader(http.StatusOK)
		}))

		bypassed := httptest.NewRecorder()
		h.ServeHTTP(bypassed, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, http.StatusOK, bypassed.Code)

		blocked := httptest.NewRecorder()
		h.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/api/users", nil))
		assert.Equal(t, http.StatusServiceUnavailable, blocked.Code)

		assert.Equal(t, int32(1), atomic.LoadInt32(&seen), "only the bypassed request should reach the handler")
	})

	t.Run("in-flight requests started before Drain run to completion", func(t *testing.T) {
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		var seen atomic.Int32

		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		h := mw(gateHandler(entered, release, &seen))

		// Start one in-flight request and wait until it has entered
		// the handler.
		var wg sync.WaitGroup
		w := httptest.NewRecorder()
		wg.Go(func() {
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		})
		<-entered
		assert.Equal(t, int64(1), drainer.InFlight())

		// Drain now; the in-flight request should not be affected.
		drainer.Drain()
		assert.Equal(t, int64(1), drainer.InFlight())

		// A second request arriving during drain must be rejected
		// without touching the gate.
		rejected := httptest.NewRecorder()
		h.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rejected.Code)
		assert.Equal(t, int32(0), seen.Load(), "rejected request must not enter the handler")

		// Release the in-flight handler; it must complete cleanly.
		close(release)
		wg.Wait()
		assert.Equal(t, int32(1), seen.Load())
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, int64(0), drainer.InFlight())
	})

	t.Run("Wait returns immediately when nothing is in flight", func(t *testing.T) {
		_, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		assert.NoError(t, drainer.Wait(ctx))
	})

	t.Run("Wait blocks until InFlight reaches zero", func(t *testing.T) {
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		var seen atomic.Int32

		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		h := mw(gateHandler(entered, release, &seen))

		var wg sync.WaitGroup
		w := httptest.NewRecorder()
		wg.Go(func() {
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		})
		<-entered

		drainer.Drain()

		// Release the gate from a goroutine so Wait observes the
		// transition rather than returning early.
		go func() {
			time.Sleep(30 * time.Millisecond)
			close(release)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		assert.NoError(t, drainer.Wait(ctx))
		assert.Equal(t, int64(0), drainer.InFlight())
		wg.Wait()
	})

	t.Run("Wait honours context deadline", func(t *testing.T) {
		entered := make(chan struct{}, 1)
		release := make(chan struct{})
		var seen atomic.Int32

		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		h := mw(gateHandler(entered, release, &seen))

		var wg sync.WaitGroup
		wg.Go(func() {
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		})
		<-entered

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := drainer.Wait(ctx)
		assert.ErrorIs(t, err, context.DeadlineExceeded)

		// Cleanup: let the in-flight request finish.
		close(release)
		wg.Wait()
	})

	t.Run("Drain is idempotent", func(t *testing.T) {
		_, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		drainer.Drain()
		drainer.Drain()
		assert.True(t, drainer.IsDraining())
	})

	t.Run("Wait does not return while a concurrent handler is still running", func(t *testing.T) {
		// Regression for the race where IsDraining() was checked
		// before inFlight.Add: a request could pass the drain check,
		// stall, and let Wait observe inFlight=0 even though the
		// handler was about to run. The fix increments inFlight
		// first, so Wait either sees the request counted or never
		// sees it execute. We exercise the contention with many
		// short-lived requests racing against Drain.
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})

		var handlerEntries atomic.Int64
		var handlerExits atomic.Int64
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			handlerEntries.Add(1)
			// Tiny artificial yield so the handler's body straddles
			// any concurrent Drain/Wait the test triggers.
			time.Sleep(50 * time.Microsecond)
			w.WriteHeader(http.StatusOK)
			handlerExits.Add(1)
		}))

		var wg sync.WaitGroup
		const N = 200
		for range N {
			wg.Go(func() {
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			})
		}

		// Sprinkle Drain across the request burst so some requests
		// see draining=false and some see draining=true.
		time.Sleep(time.Millisecond)
		drainer.Drain()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		assert.NoError(t, drainer.Wait(ctx))

		// After Wait returns, every handler that started must have
		// exited. If Wait had returned while a handler was mid-flight,
		// entries would exceed exits at this point.
		assert.Equal(t, handlerEntries.Load(), handlerExits.Load(),
			"Wait returned before all in-flight handlers completed")

		wg.Wait()
		assert.Equal(t, int64(0), drainer.InFlight())
	})

	t.Run("concurrent requests do not race InFlight", func(t *testing.T) {
		mw, drainer := GracefulShutdownMiddleware(mux.NewRouter(), GracefulShutdownConfig{})
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		const N = 100
		var wg sync.WaitGroup
		for range N {
			wg.Go(func() {
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			})
		}
		wg.Wait()
		assert.Equal(t, int64(0), drainer.InFlight())
	})
}
