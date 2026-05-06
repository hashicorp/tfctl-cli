// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import "github.com/cli/browser"

// openBrowser attempts to open the given URL in the default browser.
func openBrowser(url string) error {
	return browser.OpenURL(url)
}
