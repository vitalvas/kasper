package e2ee

import (
	"container/heap"
	"sync"
	"time"
)

// ReplayCache tracks per-message replay identifiers (nid) to detect and
// reject replayed requests, implementing the nonce (replay ID) validation of
// draft-vasylenko-e2ee-http Section 11.5 (Replay Protection). Implementations
// must be safe for concurrent use.
//
// StoreUnique atomically records nid for the given (kid, epk) scope: it
// returns true if the nid was newly inserted (not seen before) and false if
// it already existed. Per Section 11.5 the cache is keyed by (kid, epk) and
// entries are retained for at least max_skew plus a tolerance, expressed here
// via ttl. A non-positive ttl requires indefinite retention so arithmetic or
// configuration errors cannot silently disable replay protection.
//
// The server inserts the nid only after AES-GCM authentication succeeds and
// before applying any application side effects, so a duplicate indicates a
// replayed authenticated message (Section 8.5 steps 9 and 12, Section 11.5).
type ReplayCache interface {
	StoreUnique(kid string, epk []byte, nid string, ttl time.Duration) bool
}

// MemoryReplayCache is an in-memory ReplayCache with time-based eviction.
// It is safe for concurrent use. A background-free design is used: expired
// entries are pruned lazily on access and opportunistically on insert.
type MemoryReplayCache struct {
	mu       sync.Mutex
	entries  map[string]time.Time // zero expiry means retain indefinitely
	expiries replayExpiryHeap
	now      func() time.Time
}

type replayExpiry struct {
	key string
	at  time.Time
}

type replayExpiryHeap []replayExpiry

func (h replayExpiryHeap) Len() int           { return len(h) }
func (h replayExpiryHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h replayExpiryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *replayExpiryHeap) Push(value any)    { *h = append(*h, value.(replayExpiry)) }
func (h *replayExpiryHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	old[last] = replayExpiry{}
	*h = old[:last]
	return value
}

// NewMemoryReplayCache creates an empty in-memory replay cache.
func NewMemoryReplayCache() *MemoryReplayCache {
	return &MemoryReplayCache{
		entries: make(map[string]time.Time),
		now:     time.Now,
	}
}

// StoreUnique records the (kid, epk, nid) tuple if not already present.
// It returns true when the tuple was newly inserted. Non-positive ttl values
// retain the entry indefinitely.
func (c *MemoryReplayCache) StoreUnique(kid string, epk []byte, nid string, ttl time.Duration) bool {
	if c == nil {
		return false
	}

	key := replayKey(kid, epk, nid)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]time.Time)
	}
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	c.pruneLocked(now)

	if expiry, ok := c.entries[key]; ok && (expiry.IsZero() || expiry.After(now)) {
		return false
	}

	var expiry time.Time
	if ttl > 0 {
		expiry = now.Add(ttl)
		heap.Push(&c.expiries, replayExpiry{key: key, at: expiry})
	}
	c.entries[key] = expiry

	return true
}

// pruneLocked removes expired entries. Caller must hold the mutex.
func (c *MemoryReplayCache) pruneLocked(now time.Time) {
	for c.expiries.Len() > 0 && !c.expiries[0].at.After(now) {
		expired := heap.Pop(&c.expiries).(replayExpiry)
		if current, ok := c.entries[expired.key]; ok && current.Equal(expired.at) {
			delete(c.entries, expired.key)
		}
	}
}

// replayKey builds a composite cache key from kid, epk, and nid. The epk is
// length-prefixed implicitly by the fixed delimiters; a NUL separator that
// cannot appear in an sf-string keeps the components unambiguous.
func replayKey(kid string, epk []byte, nid string) string {
	const sep = "\x00"

	return kid + sep + string(epk) + sep + nid
}
