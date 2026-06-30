// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package execsession

import (
	"os"
	"strconv"
	"strings"
)

// ParentPID returns the parent pid of pid by reading /proc/<pid>/stat. The
// second field (comm) is wrapped in parentheses and may itself contain spaces
// and parentheses, so the state and ppid fields are parsed relative to the last
// ')'. Returns ok=false if the process does not exist or cannot be parsed.
func ParentPID(pid int) (int, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}

	contents := string(data)
	rparen := strings.LastIndexByte(contents, ')')
	if rparen < 0 || rparen+1 >= len(contents) {
		return 0, false
	}

	// After ") " the fields are: state ppid pgrp ...
	fields := strings.Fields(contents[rparen+1:])
	if len(fields) < 2 {
		return 0, false
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	return ppid, true
}
