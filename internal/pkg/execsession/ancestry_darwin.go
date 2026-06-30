// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build darwin

package execsession

import "golang.org/x/sys/unix"

// ParentPID returns the parent pid of pid using the kern.proc.pid sysctl.
// Returns ok=false if the process does not exist or the lookup fails.
func ParentPID(pid int) (int, bool) {
	kproc, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kproc == nil {
		return 0, false
	}
	return int(kproc.Eproc.Ppid), true
}
