// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build freebsd || netbsd || openbsd

package execsession

// ParentPID is not implemented on the BSDs. It always reports ok=false so that
// callers gracefully degrade rather than fail to build on these platforms.
func ParentPID(pid int) (int, bool) {
	return 0, false
}
