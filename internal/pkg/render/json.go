// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package render formats API responses for human-readable CLI output.
package render

import (
	"bytes"
	"encoding/json"
)

// PrettyJSON indents JSON for human-readable output.
func PrettyJSON(raw []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return string(raw)
	}
	return out.String()
}
