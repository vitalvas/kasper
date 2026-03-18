package muxhandlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vitalvas/kasper/mux"
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

// memoryLocker is an in-memory IdempotencyLocker for testing.
type memoryLocker struct {
	mu    sync.Mutex
	locks map[string]struct{}
}

func newMemoryLocker() *memoryLocker {
	return &memoryLocker{locks: make(map[string]struct{})}
}

func (l *memoryLocker) Lock(_ context.Context, key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.locks[key]; ok {
		return false
	}
	l.locks[key] = struct{}{}
	return true
}

func (l *memoryLocker) Unlock(_ context.Context, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.locks, key)
}

// testDelegatingStore is a flexible IdempotencyStore for testing custom
// get/set behaviors.
type testDelegatingStore struct {
	getFn func(ctx context.Context, key string) ([]byte, bool)
	setFn func(ctx context.Context, key string, value []byte, ttl time.Duration)
}

func (s *testDelegatingStore) Get(ctx context.Context, key string) ([]byte, bool) {
	return s.getFn(ctx, key)
}

func (s *testDelegatingStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) {
	s.setFn(ctx, key, value, ttl)
}

// testBlockingLocker is a flexible IdempotencyLocker for testing custom
// lock/unlock behaviors.
type testBlockingLocker struct {
	lockFn   func(ctx context.Context, key string) bool
	unlockFn func(ctx context.Context, key string)
}

func (l *testBlockingLocker) Lock(ctx context.Context, key string) bool {
	return l.lockFn(ctx, key)
}

func (l *testBlockingLocker) Unlock(ctx context.Context, key string) {
	l.unlockFn(ctx, key)
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

	t.Run("same key on different paths does not replay", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(r.URL.Path))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		req2 := httptest.NewRequest(http.MethodPost, "/orders", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 2, callCount, "same key on different paths should invoke handler independently")
		assert.Equal(t, "/users", w1.Body.String())
		assert.Equal(t, "/orders", w2.Body.String())
	})

	t.Run("same key on different methods does not replay", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:   store,
			Methods: []string{http.MethodPost, http.MethodPut},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(r.Method))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		req2 := httptest.NewRequest(http.MethodPut, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 2, callCount, "same key on different methods should invoke handler independently")
		assert.Equal(t, "POST", w1.Body.String())
		assert.Equal(t, "PUT", w2.Body.String())
	})

	t.Run("CacheKeyFunc scopes per user", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			CacheKeyFunc: func(r *http.Request, key string) string {
				return fmt.Sprintf("%s:%s", r.Header.Get("X-User-ID"), key)
			},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(r.Header.Get("X-User-ID")))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		req1.Header.Set("X-User-ID", "alice")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		req2.Header.Set("X-User-ID", "bob")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 2, callCount, "different users with same key should invoke handler independently")
		assert.Equal(t, "alice", w1.Body.String())
		assert.Equal(t, "bob", w2.Body.String())
	})

	t.Run("CacheKeyFunc same user same key replays", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			CacheKeyFunc: func(r *http.Request, key string) string {
				return fmt.Sprintf("%s:%s", r.Header.Get("X-User-ID"), key)
			},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		req1.Header.Set("X-User-ID", "alice")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		req2.Header.Set("X-User-ID", "alice")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount, "same user with same key should replay")
		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Equal(t, "created", w2.Body.String())
	})

	t.Run("ValidateKeyFunc rejects invalid key", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			ValidateKeyFunc: func(_ *http.Request, key string) bool {
				return len(key) == 36 // simple UUID length check
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", "short")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ValidateKeyFunc accepts valid key", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			ValidateKeyFunc: func(_ *http.Request, key string) bool {
				return len(key) == 36
			},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", "550e8400-e29b-41d4-a716-446655440000")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("key exceeding KeyMaxLength returns 400", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:        store,
			KeyMaxLength: 10,
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", "this-key-is-way-too-long")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("default KeyMaxLength rejects long key", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", strings.Repeat("a", 65))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("KeyMaxLength negative disables limit", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:        store,
			KeyMaxLength: -1,
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", strings.Repeat("a", 1000))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("CanCache false skips caching", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			CanCache: func(r *http.Request) bool {
				return r.Header.Get("X-Skip-Cache") == ""
			},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		req1.Header.Set("X-Skip-Cache", "true")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		req2.Header.Set("X-Skip-Cache", "true")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, 2, callCount, "handler should be called twice when CanCache returns false")
	})

	t.Run("CanCache true enables caching", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			CanCache: func(_ *http.Request) bool {
				return true
			},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount, "handler should be called once when CanCache returns true")
		assert.Equal(t, http.StatusCreated, w2.Code)
	})

	t.Run("OnCacheHit called on cache hit", func(t *testing.T) {
		store := newMemoryStore()

		var hitKey string
		var hitCount int
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			OnCacheHit: func(_ *http.Request, key string) {
				hitCount++
				hitKey = key
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)
		assert.Equal(t, 0, hitCount)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, 1, hitCount)
		assert.Equal(t, "key-1", hitKey)
	})

	t.Run("OnCacheMiss called on cache miss", func(t *testing.T) {
		store := newMemoryStore()

		var missKeys []string
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			OnCacheMiss: func(_ *http.Request, key string) {
				missKeys = append(missKeys, key)
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, []string{"key-1"}, missKeys, "OnCacheMiss should only be called on the first request")
	})

	t.Run("OnCacheHit and OnCacheMiss both set", func(t *testing.T) {
		store := newMemoryStore()

		var hits, misses int
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			OnCacheHit: func(_ *http.Request, _ string) {
				hits++
			},
			OnCacheMiss: func(_ *http.Request, _ string) {
				misses++
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		req3 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req3.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req3)

		assert.Equal(t, 1, misses)
		assert.Equal(t, 2, hits)
	})

	t.Run("locker returns 409 when key is in-flight", func(t *testing.T) {
		store := newMemoryStore()
		locker := newMemoryLocker()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:  store,
			Locker: locker,
		})
		require.NoError(t, err)

		started := make(chan struct{})
		proceed := make(chan struct{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			started <- struct{}{}
			<-proceed
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		// First request: blocks in handler
		w1 := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/users", nil)
			req.Header.Set("Idempotency-Key", "key-1")
			handler.ServeHTTP(w1, req)
			close(done)
		}()
		<-started

		// Second request: same key while first is in-flight
		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusConflict, w2.Code)

		// Unblock first request and wait for it to complete
		proceed <- struct{}{}
		<-done

		assert.Equal(t, http.StatusCreated, w1.Code)
	})

	t.Run("locker releases lock after response", func(t *testing.T) {
		store := newMemoryStore()
		locker := newMemoryLocker()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:  store,
			Locker: locker,
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		// After first completes, lock should be released; second replays from cache
		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w2.Code)
	})

	t.Run("fingerprint mismatch returns 422", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			FingerprintFunc: func(r *http.Request) string {
				return r.Header.Get("X-Body-Hash")
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		req1.Header.Set("X-Body-Hash", "hash-a")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusCreated, w1.Code)

		// Same key, different fingerprint
		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		req2.Header.Set("X-Body-Hash", "hash-b")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusUnprocessableEntity, w2.Code)
	})

	t.Run("fingerprint match replays cached response", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			FingerprintFunc: func(r *http.Request) string {
				return r.Header.Get("X-Body-Hash")
			},
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		req1.Header.Set("X-Body-Hash", "hash-a")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		// Same key, same fingerprint
		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		req2.Header.Set("X-Body-Hash", "hash-a")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Equal(t, "created", w2.Body.String())
	})

	t.Run("fingerprint not set does not check", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		// Same key, no fingerprint func — should replay regardless
		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount)
		assert.Equal(t, http.StatusCreated, w2.Code)
	})

	t.Run("OnConflict called on lock failure", func(t *testing.T) {
		store := newMemoryStore()
		locker := newMemoryLocker()

		var conflictCount int
		var conflictKey string
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:  store,
			Locker: locker,
			OnConflict: func(_ *http.Request, key string) {
				conflictCount++
				conflictKey = key
			},
		})
		require.NoError(t, err)

		started := make(chan struct{})
		proceed := make(chan struct{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			started <- struct{}{}
			<-proceed
			w.WriteHeader(http.StatusCreated)
		}))

		done := make(chan struct{})
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/users", nil)
			req.Header.Set("Idempotency-Key", "key-1")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			close(done)
		}()
		<-started

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, 1, conflictCount)
		assert.Equal(t, "key-1", conflictKey)

		proceed <- struct{}{}
		<-done
	})

	t.Run("OnFingerprintMismatch called on mismatch", func(t *testing.T) {
		store := newMemoryStore()

		var mismatchCount int
		var mismatchKey string
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			FingerprintFunc: func(r *http.Request) string {
				return r.Header.Get("X-Body-Hash")
			},
			OnFingerprintMismatch: func(_ *http.Request, key string) {
				mismatchCount++
				mismatchKey = key
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		req1.Header.Set("X-Body-Hash", "hash-a")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		req2.Header.Set("X-Body-Hash", "hash-b")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusUnprocessableEntity, w2.Code)
		assert.Equal(t, 1, mismatchCount)
		assert.Equal(t, "key-1", mismatchKey)
	})

	t.Run("RetryAfter header on 409", func(t *testing.T) {
		store := newMemoryStore()
		locker := newMemoryLocker()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:      store,
			Locker:     locker,
			RetryAfter: 5 * time.Second,
		})
		require.NoError(t, err)

		started := make(chan struct{})
		proceed := make(chan struct{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			started <- struct{}{}
			<-proceed
			w.WriteHeader(http.StatusCreated)
		}))

		done := make(chan struct{})
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/users", nil)
			req.Header.Set("Idempotency-Key", "key-1")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			close(done)
		}()
		<-started

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusConflict, w2.Code)
		assert.Equal(t, "5", w2.Header().Get("Retry-After"))

		proceed <- struct{}{}
		<-done
	})

	t.Run("RetryAfter not set when zero", func(t *testing.T) {
		store := newMemoryStore()
		locker := newMemoryLocker()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:  store,
			Locker: locker,
		})
		require.NoError(t, err)

		started := make(chan struct{})
		proceed := make(chan struct{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			started <- struct{}{}
			<-proceed
			w.WriteHeader(http.StatusCreated)
		}))

		done := make(chan struct{})
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/users", nil)
			req.Header.Set("Idempotency-Key", "key-1")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			close(done)
		}()
		<-started

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusConflict, w2.Code)
		assert.Empty(t, w2.Header().Get("Retry-After"))

		proceed <- struct{}{}
		<-done
	})

	t.Run("metadata skip", func(t *testing.T) {
		tests := []struct {
			name      string
			skip      bool
			wantCalls int
		}{
			{"true bypasses idempotency", true, 2},
			{"false does not bypass", false, 1},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := newMemoryStore()
				mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
				require.NoError(t, err)

				callCount := 0
				router := mux.NewRouter()
				router.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
					callCount++
					w.WriteHeader(http.StatusCreated)
				}).Methods(http.MethodPost).Metadata(IdempotencySkipMetadataKey, tt.skip)

				router.Use(mw)

				req1 := httptest.NewRequest(http.MethodPost, "/test", nil)
				req1.Header.Set("Idempotency-Key", "key-1")
				router.ServeHTTP(httptest.NewRecorder(), req1)

				req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
				req2.Header.Set("Idempotency-Key", "key-1")
				router.ServeHTTP(httptest.NewRecorder(), req2)

				assert.Equal(t, tt.wantCalls, callCount)
			})
		}
	})

	t.Run("ReplayedHeaderName set on replayed response", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:              store,
			ReplayedHeaderName: "X-Idempotency-Replayed",
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		assert.Equal(t, http.StatusCreated, w1.Code)
		assert.Empty(t, w1.Header().Get("X-Idempotency-Replayed"), "first response should not have replayed header")

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Equal(t, "true", w2.Header().Get("X-Idempotency-Replayed"))
		assert.Equal(t, "created", w2.Body.String())
	})

	t.Run("ReplayedHeaderName not set when empty", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Empty(t, w2.Header().Get("X-Idempotency-Replayed"))
	})

	t.Run("ErrorHandler replaces default error responses", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:      store,
			EnforceKey: true,
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, statusCode int) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				fmt.Fprintf(w, `{"error":"%s"}`, http.StatusText(statusCode))
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Equal(t, `{"error":"Bad Request"}`, w.Body.String())
	})

	t.Run("ErrorHandler used for 409 Conflict", func(t *testing.T) {
		store := newMemoryStore()
		locker := newMemoryLocker()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:  store,
			Locker: locker,
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, statusCode int) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				fmt.Fprintf(w, `{"error":"%s"}`, http.StatusText(statusCode))
			},
		})
		require.NoError(t, err)

		started := make(chan struct{})
		proceed := make(chan struct{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			started <- struct{}{}
			<-proceed
			w.WriteHeader(http.StatusCreated)
		}))

		done := make(chan struct{})
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/users", nil)
			req.Header.Set("Idempotency-Key", "key-1")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			close(done)
		}()
		<-started

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusConflict, w2.Code)
		assert.Equal(t, "application/json", w2.Header().Get("Content-Type"))
		assert.Equal(t, `{"error":"Conflict"}`, w2.Body.String())

		proceed <- struct{}{}
		<-done
	})

	t.Run("ErrorHandler used for 422 fingerprint mismatch", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			FingerprintFunc: func(r *http.Request) string {
				return r.Header.Get("X-Body-Hash")
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, statusCode int) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				fmt.Fprintf(w, `{"error":"%s"}`, http.StatusText(statusCode))
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		req1.Header.Set("X-Body-Hash", "hash-a")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		req2.Header.Set("X-Body-Hash", "hash-b")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusUnprocessableEntity, w2.Code)
		assert.Equal(t, "application/json", w2.Header().Get("Content-Type"))
		assert.Equal(t, `{"error":"Unprocessable Entity"}`, w2.Body.String())
	})

	t.Run("OnStore called when response is cached", func(t *testing.T) {
		store := newMemoryStore()

		var storeEvents []int
		var storeKey string
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			OnStore: func(_ *http.Request, key string, statusCode int) {
				storeKey = key
				storeEvents = append(storeEvents, statusCode)
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		// Second request replays — should NOT trigger OnStore
		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, []int{http.StatusCreated}, storeEvents)
		assert.Equal(t, "key-1", storeKey)
	})

	t.Run("OnStore not called for non-cacheable status", func(t *testing.T) {
		store := newMemoryStore()

		var storeCalled bool
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:                store,
			CacheableStatusCodes: []int{http.StatusCreated},
			OnStore: func(_ *http.Request, _ string, _ int) {
				storeCalled = true
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))

		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		req.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		assert.False(t, storeCalled)
	})

	t.Run("ResponseHeadersFunc called on replay", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: store,
			ResponseHeadersFunc: func(h http.Header, _ *http.Request, replayed bool) {
				if replayed {
					h.Set("X-Cache", "HIT")
				} else {
					h.Set("X-Cache", "MISS")
				}
			},
		})
		require.NoError(t, err)

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		assert.Equal(t, http.StatusCreated, w1.Code)
		assert.Equal(t, "MISS", w1.Header().Get("X-Cache"))

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Equal(t, "HIT", w2.Header().Get("X-Cache"))
		assert.Equal(t, "created", w2.Body.String())
	})

	t.Run("re-check cache after lock acquisition", func(t *testing.T) {
		// Verify that after acquiring the lock, the middleware re-checks
		// the cache. We use a store that forces a miss on B's initial
		// Get but returns real data on the re-check Get (after lock).
		//
		// Flow: A does Get(miss) → Lock → re-check Get(miss) → handler → Set → Unlock
		//       B does Get(forced miss) → Lock(blocks) → re-check Get(hit) → replay
		realStore := newMemoryStore()

		var getMu sync.Mutex
		getCalls := 0
		// A: Get(1)=miss, re-check Get(2)=miss (real, nothing stored yet)
		// A: stores response
		// B: Get(3)=forced miss, re-check Get(4)=hit (real, A stored)
		missStore := &testDelegatingStore{
			getFn: func(ctx context.Context, key string) ([]byte, bool) {
				getMu.Lock()
				getCalls++
				call := getCalls
				getMu.Unlock()

				if call == 3 {
					return nil, false
				}
				return realStore.Get(ctx, key)
			},
			setFn: func(ctx context.Context, key string, value []byte, ttl time.Duration) {
				realStore.Set(ctx, key, value, ttl)
			},
		}

		blocked := make(chan struct{})
		unblock := make(chan struct{})
		var lockMu sync.Mutex
		lockCalls := 0

		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store: missStore,
			Locker: &testBlockingLocker{
				lockFn: func(_ context.Context, _ string) bool {
					lockMu.Lock()
					lockCalls++
					call := lockCalls
					lockMu.Unlock()

					if call == 2 {
						close(blocked)
						<-unblock
					}
					return true
				},
				unlockFn: func(_ context.Context, _ string) {},
			},
		})
		require.NoError(t, err)

		var handlerCalls int32
		var handlerMu sync.Mutex

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			handlerMu.Lock()
			handlerCalls++
			handlerMu.Unlock()
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		}))

		// Request A: Get(1)=miss → Lock(1) → re-check Get(2)=miss → handler → store
		reqA := httptest.NewRequest(http.MethodPost, "/users", nil)
		reqA.Header.Set("Idempotency-Key", "key-1")
		wA := httptest.NewRecorder()
		handler.ServeHTTP(wA, reqA)
		assert.Equal(t, http.StatusCreated, wA.Code)

		// Request B: Get(3)=forced miss → Lock(2) blocks
		doneB := make(chan struct{})
		wB := httptest.NewRecorder()
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/users", nil)
			req.Header.Set("Idempotency-Key", "key-1")
			handler.ServeHTTP(wB, req)
			close(doneB)
		}()

		<-blocked
		// B is blocked on Lock(2). A has already stored. Unblock B.
		close(unblock)
		// B acquires lock → re-check Get(4)=hit → replay
		<-doneB

		handlerMu.Lock()
		assert.Equal(t, int32(1), handlerCalls, "handler called once; B replayed via re-check after lock")
		handlerMu.Unlock()
		assert.Equal(t, http.StatusCreated, wB.Code)
		assert.Equal(t, "created", wB.Body.String())
	})

	t.Run("MaxCacheBodySize prevents caching large responses", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:            store,
			MaxCacheBodySize: 10,
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("this response body is larger than 10 bytes"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		w1 := httptest.NewRecorder()
		handler.ServeHTTP(w1, req1)

		assert.Equal(t, http.StatusOK, w1.Code)
		assert.Equal(t, "this response body is larger than 10 bytes", w1.Body.String())

		// Second request: not cached, handler called again
		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 2, callCount, "handler should be called twice since body exceeded MaxCacheBodySize")
	})

	t.Run("MaxCacheBodySize allows caching small responses", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{
			Store:            store,
			MaxCacheBodySize: 100,
		})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("small"))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		w2 := httptest.NewRecorder()
		handler.ServeHTTP(w2, req2)

		assert.Equal(t, 1, callCount, "handler should be called once since body fits MaxCacheBodySize")
		assert.Equal(t, http.StatusCreated, w2.Code)
		assert.Equal(t, "small", w2.Body.String())
	})

	t.Run("MaxCacheBodySize zero means no limit", func(t *testing.T) {
		store := newMemoryStore()
		mw, err := IdempotencyMiddleware(IdempotencyConfig{Store: store})
		require.NoError(t, err)

		callCount := 0
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(strings.Repeat("a", 10000)))
		}))

		req1 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req1.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req1)

		req2 := httptest.NewRequest(http.MethodPost, "/users", nil)
		req2.Header.Set("Idempotency-Key", "key-1")
		handler.ServeHTTP(httptest.NewRecorder(), req2)

		assert.Equal(t, 1, callCount, "handler should be called once when MaxCacheBodySize is zero")
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
