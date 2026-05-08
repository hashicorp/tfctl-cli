// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import "github.com/cli/browser"

// openBrowserFn is the function used to open a URL in the default browser.
// Tests can replace this to avoid launching a real browser.
var openBrowserFn = browser.OpenURL

// openBrowser attempts to open the given URL in the default browser.
func openBrowser(url string) error {
	return openBrowserFn(url)
}
