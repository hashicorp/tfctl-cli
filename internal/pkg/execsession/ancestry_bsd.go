// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build freebsd || netbsd || openbsd

package execsession

import "os"

// ParentPID is only partially implemented on the BSDs: it can resolve the
// parent of the current process via os.Getppid, but not of an arbitrary pid.

func ParentPID(pid int) (int, bool) {
	if pid == os.Getpid() {
		return os.Getppid(), true
	}
	return 0, false
}
