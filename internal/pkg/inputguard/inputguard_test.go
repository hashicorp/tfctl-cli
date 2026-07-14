// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package inputguard

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "plain identifier", input: "ws-abc123", wantErr: false},
		{name: "empty string", input: "", wantErr: false},
		{name: "path-like value", input: "organizations/my-org", wantErr: false},
		{name: "dotdot is allowed (not a security boundary)", input: "../etc/passwd", wantErr: false},
		{name: "unicode letters", input: "café-naïve", wantErr: false},
		{name: "spaces are fine", input: "my org name", wantErr: false},
		{name: "ansi escape rejected", input: "red\x1b[31mtext", wantErr: true},
		{name: "newline rejected", input: "line1\nline2", wantErr: true},
		{name: "tab rejected", input: "a\tb", wantErr: true},
		{name: "null byte rejected", input: "a\x00b", wantErr: true},
		{name: "carriage return rejected", input: "a\rb", wantErr: true},
		{name: "invalid utf8 rejected", input: "a\xffb", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("Validate(%q) = nil, want error", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", tc.input, err)
			}
		})
	}
}

func TestValidateReturnsInvalidInputError(t *testing.T) {
	err := Validate("bad\x1b[0m")
	var invErr *InvalidInputError
	if !errors.As(err, &invErr) {
		t.Fatalf("Validate returned %T, want *InvalidInputError", err)
	}
}

func TestInvalidInputErrorEscapesAndTruncates(t *testing.T) {
	// Control characters must be escaped (%q), never written raw.
	err := &InvalidInputError{Value: "x\x1b[31m", Reason: "contains a control character"}
	if strings.ContainsRune(err.Error(), '\x1b') {
		t.Fatalf("error message leaked a raw escape character: %q", err.Error())
	}

	// Long values are truncated.
	long := strings.Repeat("a", maxErrorValueLen*2)
	msg := (&InvalidInputError{Value: long, Reason: "too long"}).Error()
	if !strings.Contains(msg, "...") {
		t.Fatalf("expected truncated value in %q", msg)
	}
}
