// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock is a deterministic, concurrency-safe clock for tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Unix(1_700_000_000, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// Memory must satisfy the RateLimiter interface.
var _ RateLimiter = (*Memory)(nil)

func TestMemory_AllowsUpToBurstThenLimits(t *testing.T) {
	t.Parallel()

	clk := newFakeClock()
	m := NewMemory(MemoryConfig{
		PerMinute:  60, // 1 token/sec.
		Burst:      2,
		MaxEntries: 10,
		TTL:        time.Hour,
		Now:        clk.Now,
	})

	allowed, err := m.Allow("1.1.1.1")
	require.NoError(t, err)
	assert.True(t, allowed, "first request within burst")

	allowed, err = m.Allow("1.1.1.1")
	require.NoError(t, err)
	assert.True(t, allowed, "second request within burst")

	allowed, err = m.Allow("1.1.1.1")
	require.NoError(t, err)
	assert.False(t, allowed, "third request exceeds burst")
}

func TestMemory_TokensRefillOverTime(t *testing.T) {
	t.Parallel()

	clk := newFakeClock()
	m := NewMemory(MemoryConfig{
		PerMinute:  60, // 1 token/sec.
		Burst:      1,
		MaxEntries: 10,
		TTL:        time.Hour,
		Now:        clk.Now,
	})

	allowed, _ := m.Allow("1.1.1.1")
	assert.True(t, allowed, "first request consumes the only token")

	allowed, _ = m.Allow("1.1.1.1")
	assert.False(t, allowed, "no tokens left")

	clk.Advance(time.Second)

	allowed, _ = m.Allow("1.1.1.1")
	assert.True(t, allowed, "token refilled after one second")
}

func TestMemory_KeysAreIndependent(t *testing.T) {
	t.Parallel()

	clk := newFakeClock()
	m := NewMemory(MemoryConfig{
		PerMinute:  60,
		Burst:      1,
		MaxEntries: 10,
		TTL:        time.Hour,
		Now:        clk.Now,
	})

	allowed, _ := m.Allow("1.1.1.1")
	assert.True(t, allowed)
	allowed, _ = m.Allow("1.1.1.1")
	assert.False(t, allowed, "first IP is now limited")

	allowed, _ = m.Allow("2.2.2.2")
	assert.True(t, allowed, "different IP has its own bucket")
}

func TestMemory_EvictsStaleEntries(t *testing.T) {
	t.Parallel()

	clk := newFakeClock()
	m := NewMemory(MemoryConfig{
		PerMinute:  60,
		Burst:      1,
		MaxEntries: 2,
		TTL:        time.Minute,
		Now:        clk.Now,
	})

	_, _ = m.Allow("1.1.1.1")
	_, _ = m.Allow("2.2.2.2")
	assert.Equal(t, 2, m.Len())

	// Advance well past the TTL so existing entries are stale, then insert a
	// new key which triggers an eviction sweep.
	clk.Advance(2 * time.Minute)
	_, _ = m.Allow("3.3.3.3")

	assert.Equal(t, 1, m.Len(), "stale entries swept, only the new key remains")
}

func TestMemory_EnforcesMaxEntryCap(t *testing.T) {
	t.Parallel()

	clk := newFakeClock()
	m := NewMemory(MemoryConfig{
		PerMinute:  60,
		Burst:      1,
		MaxEntries: 2,
		TTL:        time.Hour, // Nothing is stale; cap must still hold.
		Now:        clk.Now,
	})

	for i := 0; i < 50; i++ {
		_, err := m.Allow(fmt.Sprintf("10.0.0.%d", i))
		require.NoError(t, err)
		assert.LessOrEqual(t, m.Len(), 2, "entry count never exceeds the cap")
	}
	assert.Equal(t, 2, m.Len())
}

func TestMemory_DefaultsApplied(t *testing.T) {
	t.Parallel()

	// Zero values for MaxEntries/TTL/Now must be replaced by sane defaults
	// rather than producing a limiter that rejects everything or panics.
	m := NewMemory(MemoryConfig{PerMinute: 60, Burst: 5})
	allowed, err := m.Allow("1.1.1.1")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestMemory_ConcurrentAccessIsRaceFree(t *testing.T) {
	t.Parallel()

	clk := newFakeClock()
	const maxEntries = 64
	m := NewMemory(MemoryConfig{
		PerMinute:  6000,
		Burst:      100,
		MaxEntries: maxEntries,
		TTL:        time.Minute,
		Now:        clk.Now,
	})

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_, _ = m.Allow(fmt.Sprintf("ip-%d-%d", g, i%50))
			}
		}(g)
	}
	wg.Wait()

	assert.LessOrEqual(t, m.Len(), maxEntries, "cap holds under concurrency")
}
