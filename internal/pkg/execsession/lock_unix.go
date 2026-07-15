// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !windows

package execsession

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// acquireLock takes a shared, non-blocking advisory lock on f. The granting
// process holds this lock for its lifetime; authorizers detect liveness by
// trying to take an exclusive lock (see probeLiveness). It returns an error if
// an incompatible lock is already held.
func acquireLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_SH|unix.LOCK_NB)
}

// releaseLock releases the advisory lock held on f.
func releaseLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

// probeLiveness reports whether the process that created the session file at
// path is still alive. The holder keeps a shared lock for its lifetime, so an
// exclusive lock can only be acquired once the holder has exited. A successful
// exclusive acquire therefore means the holder is dead; EWOULDBLOCK means it is
// still alive.
func probeLiveness(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = f.Close() }()

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return true, nil // holder still alive
		}
		return false, err
	}
	// Acquired an exclusive lock: no live holder. Release immediately so we do
	// not falsely signal liveness to a concurrent probe.
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return false, nil
}
