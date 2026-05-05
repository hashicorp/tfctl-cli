// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"os/exec"
	"runtime"
)

// browserCmd returns the command and arguments for opening a URL in the default browser.
func browserCmd(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}

// openBrowser attempts to open the given URL in the default browser.
func openBrowser(url string) error {
	bin, args := browserCmd(url)
	return exec.Command(bin, args...).Start()
}
