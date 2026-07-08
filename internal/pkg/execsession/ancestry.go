// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package execsession

import "os"

// AncestryFn returns the parent PID of pid, with ok=false when it cannot be
// determined or pid is invalid/dead. It is the seam that lets consumer logic be
// tested with a fake process tree.
type AncestryFn func(pid int) (ppid int, ok bool)

// maxAncestryDepth bounds the parent walk to guard against cycles and
// pathological trees.
const maxAncestryDepth = 64

// IsAncestor walks parents starting at self up to a bounded depth, returning
// true if target is encountered. It fails closed (returns false) when ancestry
// cannot be resolved on this platform, when parentOf is nil, or when a cycle is
// detected. Because a dead PID will not appear in the walk, this also serves as
// a liveness check on target.
func IsAncestor(target, self int, parentOf AncestryFn) bool {
	if parentOf == nil {
		return false
	}

	pid := self
	for i := 0; i < maxAncestryDepth && pid > 1; i++ {
		if pid == target {
			return true
		}
		ppid, ok := parentOf(pid)
		if !ok {
			return false
		}
		if ppid == pid {
			return false // self-cycle
		}
		pid = ppid
	}
	return pid == target
}

// selfPID returns the current process id. It exists as a helper so tests can
// reference the process without importing os.
func selfPID() int {
	return os.Getpid()
}
