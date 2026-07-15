// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package execsession

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// acquireLock takes a shared, non-blocking lock on f via LockFileEx. The
// granting process holds this lock for its lifetime; authorizers detect
// liveness by trying to take an exclusive lock (see probeLiveness).
func acquireLock(f *os.File) error {
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_FAIL_IMMEDIATELY, // shared lock (no EXCLUSIVE flag)
		0,
		1, 0,
		&windows.Overlapped{},
	)
}

// releaseLock releases the lock held on f.
func releaseLock(f *os.File) error {
	return windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0,
		1, 0,
		&windows.Overlapped{},
	)
}

// probeLiveness reports whether the process that created the session file at
// path is still alive. The holder keeps a shared lock for its lifetime, so an
// exclusive lock can only be acquired once the holder has exited. A successful
// exclusive acquire therefore means the holder is dead; a lock violation means
// it is still alive.
func probeLiveness(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = f.Close() }()

	h := windows.Handle(f.Fd())
	ol := &windows.Overlapped{}
	if err := windows.LockFileEx(
		h,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, 0,
		ol,
	); err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return true, nil // holder still alive
		}
		return false, err
	}
	// Acquired an exclusive lock: no live holder. Release immediately so we do
	// not falsely signal liveness to a concurrent probe.
	_ = windows.UnlockFileEx(h, 0, 1, 0, ol)
	return false, nil
}
