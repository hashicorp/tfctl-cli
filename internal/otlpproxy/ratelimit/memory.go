// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Default bounds for the in-memory limiter. They guard a public endpoint
// against unbounded memory growth from a flood of unique client IPs.
const (
	// DefaultMaxEntries caps the number of distinct per-IP buckets retained.
	DefaultMaxEntries = 50_000

	// DefaultTTL is how long an idle per-IP bucket is kept before it becomes
	// eligible for eviction.
	DefaultTTL = 10 * time.Minute
)

// MemoryConfig configures an in-memory RateLimiter.
type MemoryConfig struct {
	// PerMinute is the sustained per-key request rate.
	PerMinute float64

	// Burst is the per-key token-bucket burst size.
	Burst int

	// MaxEntries bounds the number of per-key buckets. Values <= 0 use
	// DefaultMaxEntries.
	MaxEntries int

	// TTL is the idle lifetime of a per-key bucket. Values <= 0 use
	// DefaultTTL.
	TTL time.Duration

	// Now is the clock used for token accounting and eviction. Nil uses
	// time.Now. Injected for deterministic tests.
	Now func() time.Time
}

// entry is a per-key bucket plus its last-seen timestamp for eviction.
type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Memory is an in-memory, per-key token-bucket RateLimiter. It bounds memory
// via a max-entry cap and idle eviction so a flood of unique keys (the typical
// public-endpoint risk) cannot grow the map without limit.
type Memory struct {
	limit      rate.Limit
	burst      int
	maxEntries int
	ttl        time.Duration
	now        func() time.Time

	mu      sync.Mutex
	entries map[string]*entry
}

// NewMemory builds an in-memory limiter, applying defaults for unset bounds.
func NewMemory(cfg MemoryConfig) *Memory {
	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &Memory{
		limit:      rate.Limit(cfg.PerMinute / 60.0), // per-minute -> per-second.
		burst:      cfg.Burst,
		maxEntries: maxEntries,
		ttl:        ttl,
		now:        now,
		entries:    make(map[string]*entry),
	}
}

// Allow reports whether a request from key is permitted now. It never returns
// an error: the in-memory backend cannot fail.
func (m *Memory) Allow(key string) (bool, error) {
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[key]
	if !ok {
		if len(m.entries) >= m.maxEntries {
			m.evictLocked(now)
		}
		e = &entry{limiter: rate.NewLimiter(m.limit, m.burst)}
		m.entries[key] = e
	}
	e.lastSeen = now

	return e.limiter.AllowN(now, 1), nil
}

// Len returns the current number of tracked keys. Primarily for tests.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

// evictLocked makes room for a new key. In a single pass it deletes all stale
// entries and tracks the oldest survivor; if the map is still at capacity it
// removes that oldest survivor. Callers must hold m.mu.
func (m *Memory) evictLocked(now time.Time) {
	var (
		oldestKey  string
		oldestSeen time.Time
	)
	for k, e := range m.entries {
		// Stale when lastSeen + ttl <= now.
		if !e.lastSeen.Add(m.ttl).After(now) {
			delete(m.entries, k)
			continue
		}
		if oldestKey == "" || e.lastSeen.Before(oldestSeen) {
			oldestKey = k
			oldestSeen = e.lastSeen
		}
	}

	if oldestKey != "" && len(m.entries) >= m.maxEntries {
		delete(m.entries, oldestKey)
	}
}
