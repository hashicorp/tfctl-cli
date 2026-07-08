// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !windows

package execsession

import (
	"os"

	"golang.org/x/sys/unix"
)

// acquireLock takes an exclusive, non-blocking advisory lock on f. It returns
// an error (unix.EWOULDBLOCK) if another process already holds the lock.
func acquireLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

// releaseLock releases the advisory lock held on f.
func releaseLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
