package e2ee

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemoryReplayCacheStoreUnique(t *testing.T) {
	c := NewMemoryReplayCache()

	epk := []byte{1, 2, 3}

	assert.True(t, c.StoreUnique("kid", epk, "nid1", time.Minute), "first insert is unique")
	assert.False(t, c.StoreUnique("kid", epk, "nid1", time.Minute), "duplicate rejected")
	assert.True(t, c.StoreUnique("kid", epk, "nid2", time.Minute), "different nid is unique")
	assert.True(t, c.StoreUnique("kid2", epk, "nid1", time.Minute), "different kid is unique")
	assert.True(t, c.StoreUnique("kid", []byte{9}, "nid1", time.Minute), "different epk is unique")
}

func TestMemoryReplayCacheZeroValueAndNilReceiver(t *testing.T) {
	var zero MemoryReplayCache
	assert.True(t, zero.StoreUnique("kid", nil, "nid", time.Minute))
	assert.False(t, zero.StoreUnique("kid", nil, "nid", time.Minute))

	var cache *MemoryReplayCache
	assert.False(t, cache.StoreUnique("kid", nil, "nid", time.Minute))
}

func TestMemoryReplayCacheExpiry(t *testing.T) {
	c := NewMemoryReplayCache()

	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }

	assert.True(t, c.StoreUnique("kid", nil, "nid", 10*time.Second))
	assert.False(t, c.StoreUnique("kid", nil, "nid", 10*time.Second))

	// Advance past expiry; the nid should be acceptable again.
	now = now.Add(11 * time.Second)
	assert.True(t, c.StoreUnique("kid", nil, "nid", 10*time.Second))
}

func TestMemoryReplayCacheConcurrent(t *testing.T) {
	c := NewMemoryReplayCache()

	const n = 100

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		uniques int
	)

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()

			if c.StoreUnique("kid", nil, "same-nid", time.Minute) {
				mu.Lock()
				uniques++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, 1, uniques, "exactly one insert wins the race")
}

func TestReplayPruneRemovesExpired(t *testing.T) {
	c := NewMemoryReplayCache()

	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }

	// Insert an entry with a short TTL, then advance and insert another, which
	// triggers pruning of the expired entry.
	assert.True(t, c.StoreUnique("k", nil, "old", time.Second))

	now = now.Add(2 * time.Second)
	assert.True(t, c.StoreUnique("k", nil, "new", time.Minute))

	// The old entry was pruned; it can be inserted again as unique.
	assert.True(t, c.StoreUnique("k", nil, "old", time.Minute))
}

func TestMemoryReplayCacheNonPositiveTTLRetainsEntry(t *testing.T) {
	c := NewMemoryReplayCache()
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }

	assert.True(t, c.StoreUnique("k", nil, "zero", 0))
	now = now.Add(100 * 365 * 24 * time.Hour)
	assert.False(t, c.StoreUnique("k", nil, "zero", -time.Second))
}

func TestMemoryReplayCachePrunesThroughExpiryHeap(t *testing.T) {
	c := NewMemoryReplayCache()
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }

	for i := range 1000 {
		assert.True(t, c.StoreUnique("k", nil, string(rune(i)), time.Second))
	}
	assert.Len(t, c.entries, 1000)
	assert.Len(t, c.expiries, 1000)

	now = now.Add(2 * time.Second)
	assert.True(t, c.StoreUnique("k", nil, "fresh", time.Minute))
	assert.Len(t, c.entries, 1)
	assert.Len(t, c.expiries, 1)
}

func BenchmarkMemoryReplayCachePruning(b *testing.B) {
	for _, count := range []int{1024, 16384} {
		b.Run(fmt.Sprintf("expired=%d", count), func(b *testing.B) {
			now := time.Unix(1000, 0)
			for b.Loop() {
				b.StopTimer()
				cache := NewMemoryReplayCache()
				cache.now = func() time.Time { return now }
				for i := range count {
					cache.StoreUnique("kid", nil, fmt.Sprintf("nid-%d", i), time.Second)
				}
				now = now.Add(2 * time.Second)
				b.StartTimer()

				if !cache.StoreUnique("kid", nil, "fresh", time.Minute) {
					b.Fatal("fresh replay identifier was rejected")
				}

				b.StopTimer()
				now = now.Add(-2 * time.Second)
				b.StartTimer()
			}
			b.ReportMetric(float64(count), "expired/op")
		})
	}
}
