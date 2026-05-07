// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"fmt"
	"io"
	"net/http"
)

// fetchLog fetches the raw log content from an archivist log-read-url.
// The URL is self-authenticating so no additional auth headers are needed.
func fetchLog(logURL string) (string, error) {
	resp, err := http.Get(logURL) //nolint:gosec // log-read-url is self-authenticating
	if err != nil {
		return "", fmt.Errorf("fetching log: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching log: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading log body: %w", err)
	}

	return string(body), nil
}
