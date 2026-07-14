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
	"unicode"
	"unicode/utf8"
)

// maxErrorValueLen bounds how much of an offending value is echoed in an error
// so a pathological input cannot flood the terminal.
const maxErrorValueLen = 80

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
