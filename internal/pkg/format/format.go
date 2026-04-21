// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package format provides utilities for handling output formats.
package format

import (
	"fmt"
	"strings"

	"golang.org/x/exp/maps"
)

// Format captures the output format to use.
type Format int

const (
	// Unset is the default value for Format and indicates that no format has been set.
	Unset Format = iota

	// Pretty outputs the payload in vertical records.
	Pretty Format = iota

	// Table outputs the payload in a human readable table format. This is the default unless the
	// output is not a table or is too wide.
	Table Format = iota

	// JSON outputs the values in raw JSON.
	JSON Format = iota

	// Markdown outputs the payload in markdown format.
	Markdown Format = iota

	// Agent outputs as JSON, except edited for relevance.
	Agent Format = iota
)

var (
	// formatStrings is used to convert from the canonical string representation
	// to the Format enum.
	formatStrings = map[string]Format{
		"pretty":   Pretty,
		"table":    Table,
		"markdown": Markdown,
		"json":     JSON,
		"agent":    Agent,
	}
)

// IsJSONOrAgent returns true if the format is machine-readable output.
func (f Format) IsJSONOrAgent() bool {
	return f == JSON || f == Agent
}

// FromString converts a string representation of a format to a Format.
func FromString(s string) (Format, error) {
	s = strings.ToLower(s)
	f, ok := formatStrings[s]
	if !ok {
		return Unset, fmt.Errorf("invalid format %q. Must be one of %q", s, maps.Keys(formatStrings))
	}

	return f, nil
}
