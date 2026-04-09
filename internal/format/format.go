// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"fmt"
	"strings"

	"golang.org/x/exp/maps"
)

// Format captures the output format to use.
type Format int

const (
	Unset Format = iota

	// Pretty is used to output the payload in a key/value format where each
	// pair is outputted on a new line.
	Pretty Format = iota

	// Table outputs the payload as a table.
	Table Format = iota

	// JSON outputs the values in raw JSON.
	JSON Format = iota
)

var (
	// formatStrings is used to convert from the canonical string representation
	// to the Format enum.
	formatStrings = map[string]Format{
		"pretty": Pretty,
		"table":  Table,
		"json":   JSON,
	}
)

// FromString converts a string representation of a format to a Format.
func FromString(s string) (Format, error) {
	s = strings.ToLower(s)
	f, ok := formatStrings[s]
	if !ok {
		return Pretty, fmt.Errorf("invalid format %q. Must be one of %q", s, maps.Keys(formatStrings))
	}

	return f, nil
}
