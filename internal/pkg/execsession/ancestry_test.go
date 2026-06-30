// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package execsession

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeAncestry builds an AncestryFn from a child->parent map. A pid that is
// absent from the map is treated as dead/unknown (ok=false).
func fakeAncestry(parents map[int]int) AncestryFn {
	return func(pid int) (int, bool) {
		ppid, ok := parents[pid]
		return ppid, ok
	}
}

func TestIsAncestor(t *testing.T) {
	t.Parallel()

	t.Run("direct parent", func(t *testing.T) {
		t.Parallel()
		parents := map[int]int{100: 50, 50: 1}
		assert.True(t, IsAncestor(50, 100, fakeAncestry(parents)))
	})

	t.Run("deep chain", func(t *testing.T) {
		t.Parallel()
		parents := map[int]int{100: 90, 90: 80, 80: 70, 70: 1}
		assert.True(t, IsAncestor(80, 100, fakeAncestry(parents)))
	})

	t.Run("self is ancestor", func(t *testing.T) {
		t.Parallel()
		parents := map[int]int{100: 50}
		assert.True(t, IsAncestor(100, 100, fakeAncestry(parents)))
	})

	t.Run("not an ancestor", func(t *testing.T) {
		t.Parallel()
		parents := map[int]int{100: 50, 50: 1}
		assert.False(t, IsAncestor(999, 100, fakeAncestry(parents)))
	})

	t.Run("dead target pid not in chain", func(t *testing.T) {
		t.Parallel()
		// 100 -> 50 -> 1, target 777 is not in the walk.
		parents := map[int]int{100: 50, 50: 1}
		assert.False(t, IsAncestor(777, 100, fakeAncestry(parents)))
	})

	t.Run("unknown self pid fails closed", func(t *testing.T) {
		t.Parallel()
		parents := map[int]int{} // self 100 not resolvable
		assert.False(t, IsAncestor(50, 100, fakeAncestry(parents)))
	})

	t.Run("cycle guard", func(t *testing.T) {
		t.Parallel()
		// 100 -> 50 -> 100 (cycle, neither is target 7)
		parents := map[int]int{100: 50, 50: 100}
		assert.False(t, IsAncestor(7, 100, fakeAncestry(parents)))
	})

	t.Run("self-cycle parent equals pid", func(t *testing.T) {
		t.Parallel()
		parents := map[int]int{100: 100}
		assert.False(t, IsAncestor(7, 100, fakeAncestry(parents)))
	})

	t.Run("depth cap", func(t *testing.T) {
		t.Parallel()
		// Build a long chain where target sits beyond the depth cap.
		parents := make(map[int]int)
		for i := 1000; i > 100; i-- {
			parents[i] = i - 1
		}
		// target 100 is ~900 hops from 1000, beyond the 64 cap.
		assert.False(t, IsAncestor(100, 1000, fakeAncestry(parents)))
	})

	t.Run("nil parentOf fails closed", func(t *testing.T) {
		t.Parallel()
		assert.False(t, IsAncestor(50, 100, nil))
	})

	t.Run("reaching init without target", func(t *testing.T) {
		t.Parallel()
		parents := map[int]int{100: 1}
		assert.False(t, IsAncestor(2, 100, fakeAncestry(parents)))
	})
}

func TestParentPID_Self(t *testing.T) {
	t.Parallel()

	// The platform ParentPID for our own process should resolve to our actual
	// parent (the test runner / shell). We can't assert the exact value but it
	// should be resolvable and positive on supported platforms.
	self := selfPID()
	ppid, ok := ParentPID(self)
	if !ok {
		t.Skip("ParentPID not supported on this platform")
	}
	assert.Positive(t, ppid)
	assert.NotEqual(t, self, ppid)
}
