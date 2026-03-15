package muxhandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryStore is an in-memory IdempotencyStore for testing.
type memoryStore struct {
	mu    sync.RWMutex
	items map[string]memoryEntry
}

type memoryEntry struct {
	data      []byte
	expiresAt time.Time
}

func newMemoryStore() *memoryStore {
	return &memoryStore{items: make(map[string]memoryEntry)}
}

func (s *memoryStore) Get(_ context.Context, key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.items[key]
	if !ok {
		return nil, false
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

func (s *memoryStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	s.items[key] = memoryEntry{data: value, expiresAt: expiresAt}
}

func TestIdempotencyMiddleware(t *testing.T) {
	t.Run("config error no store", func(t *testing.T) {
		_, err := IdempotencyMiddleware(IdempotencyConfig{})
		assert.ErrorIs(t, err, ErrNoIdempotencyStore)
	})

	t.Run("first request passes through and caches", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("X-Custom", "value")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"123"}`))
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", "key-1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "value", w.Header().Get("X-Custom"))
		assert.Equal(t, "key-1", w.Header().Get("Idempotency-Key"))
		assert.Equal(t, `{"id":"123"}`, w.Body.String())
	})

	t.Run("duplicate request replays cached response", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.Header().Set("X-Custom", "value")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"123"}`))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Equal(t, "value", w2.Header().Get("X-Custom"))
		assert.Equal(t, "key-1", w2.Header().Get("Idempotency-Key"))
		assert.Equal(t, `{"id":"123"}`, w2.Body.String())
	})

	t.Run("different keys are independent", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-a")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-b")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, 2, callCount)
	})

	t.Run("GET requests pass through without key", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/users", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing key passes through by default", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing key returns 400 when enforced", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:      store,
			EnforceKey: true,
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("custom header name", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:      store,
			HeaderName: "X-Request-Id",
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("X-Request-Id", "custom-key")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("X-Request-Id", "custom-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req2)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "custom-key", w.Header().Get("X-Request-Id"))
	})

	t.Run("custom methods", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:   store,
			Methods: []string{http.MethodPut, http.MethodPatch},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
		}))

		// POST should pass through (not in custom methods)
		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		assert.Equal(t, 2, callCount)

		// PUT should be idempotent
		callCount = 0
		req2 := httptest.NewRequest(http.MethodPut, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-2")
		handler.ServeHTTP(httptest.NewRecorder(), req2)
		handler.ServeHTTP(httptest.NewRecorder(), req2)
		assert.Equal(t, 1, callCount)
	})

	t.Run("does not cache excluded status codes", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:                store,
			CacheableStatusCodes: []int{http.StatusOK, http.StatusCreated},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-err")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-err")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 2, callCount, "handler should be called twice since 500 is not cacheable")
		assert.Equal(t, http.StatusInternalServerError, w1.Code)
		assert.Equal(t, http.StatusInternalServerError, w2.Code)
	})

	t.Run("caches allowed status codes", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:                store,
			CacheableStatusCodes: []int{http.StatusOK, http.StatusCreated},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"1"}`))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-ok")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-ok")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount, "handler should be called once since 201 is cacheable")
		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Equal(t, `{"id":"1"}`, w2.Body.String())
	})

	t.Run("nil CacheableStatusCodes caches all", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusInternalServerError)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/", nil)
		req1.Header.Set("Idempotency-Key", "key-all")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/", nil)
		req2.Header.Set("Idempotency-Key", "key-all")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, 1, callCount, "handler should be called once since all status codes are cached by default")
	})

	t.Run("handler that writes body without explicit WriteHeader", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("hello"))
		}))

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Idempotency-Key", "key-1")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "hello", w.Body.String())

		// Replay
		req2 := httptest.NewRequest(http.MethodPost, "/", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusOK, w2.Code)
		assert.Equal(t, "hello", w2.Body.String())
	})
}

func BenchmarkIdempotencyMiddleware(b *testing.B) {
	b.Run("cache miss", func(b *testing.B) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		if err != nil {
			b.Fatal(err)
		}

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"123"}`))
		}))

		b.ResetTimer()
		for i := range b.N {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("Idempotency-Key", string(rune(i)))
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})

	b.Run("cache hit", func(b *testing.B) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		if err != nil {
			b.Fatal(err)
		}

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"123"}`))
		}))

		// Seed cache
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Idempotency-Key", "bench-key")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		b.ResetTimer()
		for b.Loop() {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("Idempotency-Key", "bench-key")
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
