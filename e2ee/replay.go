package e2ee

import (
	"sync"
	"time"
)

// ReplayCache tracks per-message replay identifiers (nid) to detect and
// reject replayed requests. Implementations must be safe for concurrent use.
//
// StoreUnique atomically records nid for the given (kid, epk) scope: it
// returns true if the nid was newly inserted (not seen before) and false if
// it already existed. Entries may be evicted after ttl has elapsed.
//
// The server inserts the nid only after AES-GCM authentication succeeds and
// before applying any application side effects, so a duplicate indicates a
// replayed authenticated message.
type ReplayCache interface {
	StoreUnique(kid string, epk []byte, nid string, ttl time.Duration) bool
}

// MemoryReplayCache is an in-memory ReplayCache with time-based eviction.
// It is safe for concurrent use. A background-free design is used: expired
// entries are pruned lazily on access and opportunistically on insert.
type MemoryReplayCache struct {
	mu      sync.Mutex
	entries map[string]time.Time // composite key -> expiry
	now     func() time.Time
}

// NewMemoryReplayCache creates an empty in-memory replay cache.
func NewMemoryReplayCache() *MemoryReplayCache {
	return &MemoryReplayCache{
		entries: make(map[string]time.Time),
		now:     time.Now,
	}
}

// StoreUnique records the (kid, epk, nid) tuple if not already present.
// It returns true when the tuple was newly inserted.
func (c *MemoryReplayCache) StoreUnique(kid string, epk []byte, nid string, ttl time.Duration) bool {
	key := replayKey(kid, epk, nid)
	now := c.now()
	expiry := now.Add(ttl)

	c.mu.Lock()
	defer c.mu.Unlock()

	if exp, ok := c.entries[key]; ok && exp.After(now) {
		return false
	}

	c.entries[key] = expiry
	c.pruneLocked(now)

	return true
}

// pruneLocked removes expired entries. Caller must hold the mutex.
func (c *MemoryReplayCache) pruneLocked(now time.Time) {
	for k, exp := range c.entries {
		if !exp.After(now) {
			delete(c.entries, k)
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
