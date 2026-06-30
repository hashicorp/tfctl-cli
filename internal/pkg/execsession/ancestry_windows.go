// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build windows

package execsession

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ParentPID returns the parent pid of pid by walking a Toolhelp32 process
// snapshot. Returns ok=false if the snapshot fails or the process is not found.
func ParentPID(pid int) (int, bool) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0, false
	}

	for {
		if int(entry.ProcessID) == pid {
			return int(entry.ParentProcessID), true
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			return 0, false
		}
	}
}
