// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package execsession

import (
	"os"

	"golang.org/x/sys/windows"
)

// acquireLock takes an exclusive, non-blocking lock on f via LockFileEx. It
// returns an error if another process already holds the lock.
func acquireLock(f *os.File) error {
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
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
