// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package inputguard provides lightweight input-hygiene checks for user-supplied
// CLI values.
//
// This is deliberately NOT a security boundary. Authorization is enforced
// server-side by the API token; these checks only keep obviously-malformed
// values (invalid UTF-8, control characters such as ANSI escape sequences) out
// of requests and out of any text that tfctl echoes back to a terminal or an
// audit log, where they could corrupt output or spoof messages.
package inputguard

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxErrorValueLen bounds how much of an offending value is echoed in an error
// so a pathological input cannot flood the terminal.
const maxErrorValueLen = 80

var (
	// Redactions should use capture groups to identify sensitive segments of a path, which
	// will be replaced with a redaction placeholder.
	redactions = []*regexp.Regexp{
		// API paths:
		regexp.MustCompile("/organizations/([^/]+)/workspaces/([^/]+)"),
		regexp.MustCompile("/organizations/([^/]+)"),

		// Archivist Paths:
		regexp.MustCompile("/v1/object/([a-zA-Z0-9]+)"),

		// Registry paths:
		regexp.MustCompile("/v1/modules/([^/]+)/([^/]+)/([^/]+)"),
		regexp.MustCompile("/registry-providers/private/([^/]+)/([^/]+)"),
	}
)

// InvalidInputError describes why a value failed validation. The offending value
// is always rendered with %q so control characters are escaped rather than
// written raw to a terminal.
type InvalidInputError struct {
	// Value is the offending input.
	Value string

	// Reason is a short, human-readable explanation.
	Reason string
}

func (e *InvalidInputError) Error() string {
	v := e.Value
	if len(v) > maxErrorValueLen {
		v = v[:maxErrorValueLen] + "..."
	}
	return fmt.Sprintf("invalid input %q: %s", v, e.Reason)
}

// Validate reports whether s is safe to use as a CLI input value. It rejects:
//
//   - invalid UTF-8, and
//   - control characters (including ANSI escape sequences),
//
// which have no legitimate place in a command-line value and can corrupt
// terminal output or audit logs. It returns an *InvalidInputError on failure.
//
// This is input hygiene, not authorization: it does not attempt to judge
// whether a value is "allowed", only that it is well-formed printable text.
func Validate(s string) error {
	if !utf8.ValidString(s) {
		return &InvalidInputError{Value: s, Reason: "contains invalid UTF-8"}
	}

	for _, r := range s {
		if unicode.IsControl(r) {
			return &InvalidInputError{Value: s, Reason: "contains a control character"}
		}
	}

	return nil
}

// RedactPath returns a version of an HTTP path suitable for logging or error messages, with
// sensitive segments replaced with a redaction placeholder. It is
// intended for use in telemetry and audit logs, not for security-critical
// redaction of secrets.
func RedactPath(path string) string {
	for _, re := range redactions {
		// Find the start/end indexes of the full match AND all capture groups
		indices := re.FindStringSubmatchIndex(path)
		if len(indices) == 0 {
			continue
		}

		var result strings.Builder
		lastIndex := 0

		// indices[0] and indices[1] are the start/end of the FULL match.
		// Capture groups start at index 2 (indices[2] to indices[3] is Group 1, etc.)
		for i := 1; i < len(indices)/2; i++ {
			groupStart := indices[2*i]
			groupEnd := indices[2*i+1]

			// Handle cases where an optional group didn't match
			if groupStart < 0 || groupEnd < 0 {
				continue
			}

			result.WriteString(path[lastIndex:groupStart])
			result.WriteString("<redacted>")
			lastIndex = groupEnd
		}

		// Write whatever remains of the path after the last redacted group
		result.WriteString(path[lastIndex:])

		// Update path and continue to next regex
		path = result.String()
	}

	return path
}
