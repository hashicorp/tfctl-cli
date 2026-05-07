// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"encoding/json"
	"strings"
)

// JSONLog represents a single line of Terraform's structured JSON log output.
type JSONLog struct {
	Level      string      `json:"@level"`
	Message    string      `json:"@message"`
	Type       string      `json:"type"`
	Diagnostic *Diagnostic `json:"diagnostic,omitempty"`
}

type Diagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
	Address  string `json:"address,omitempty"`
}

// parseDiagnostics attempts to extract diagnostics from log output.
// It detects structured logs (JSON lines after the first 3 lines) and parses them.
// Returns the diagnostics found, or nil if the log is not structured.
func parseDiagnostics(logContent string) []Diagnostic {
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	if len(lines) <= 3 {
		return nil
	}

	structured := false
	for _, line := range lines[3:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if json.Valid([]byte(line)) {
			structured = true
			break
		}
		break
	}

	if !structured {
		return nil
	}

	// Parse JSON lines and filter for diagnostics.
	var diags []Diagnostic
	for _, line := range lines[3:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry JSONLog
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.Type == "diagnostic" && entry.Diagnostic != nil {
			diags = append(diags, *entry.Diagnostic)
		}
	}

	return diags
}
