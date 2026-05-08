// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"bufio"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestPretty_StringPayload(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	d := &StringPayloadDisplayer{
		payload:    map[string]string{"key": "value"},
		prettyText: "formatted pretty output",
	}

	r.NoError(out.Display(d))
	r.Equal("formatted pretty output\n", io.Output.String())
}

func TestPretty_KV_Slice_Empty(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer
	d := &KVDisplayer{
		KVs:     []*KV{},
		Default: format.Pretty,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	r.Equal("Listed 0 items.\n", io.Output.String())
}

func TestPretty_KV_Slice(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer
	d := &KVDisplayer{
		KVs: []*KV{
			{
				Key:   "Hello",
				Value: "World!",
			},
			{
				Key:   "Another",
				Value: "Test",
			},
		},
		Default: format.Pretty,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Key:", "Hello"},
		{"Value:", "World!"},
		{"---"},
		{"Key:", "Another"},
		{"Value:", "Test"},
	}

	previousAlignment := -1
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))

		// Skip blank lines when checking for alignment
		if len(row) == 1 {
			continue
		}

		// Determine whether we are aligning
		alignment := charactersToValue(scanner.Text())
		if previousAlignment == -1 {
			previousAlignment = alignment
		}

		r.Equal(previousAlignment, alignment)
	}

	// There should be no more text
	r.False(scanner.Scan())
}

func TestPretty_KV_Struct(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer
	d := &KVDisplayer{
		KVs: []*KV{
			{
				Key:   "Hello",
				Value: "World!",
			},
		},
		Default: format.Pretty,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Key:", "Hello"},
		{"Value:", "World!"},
	}

	previousAlignment := -1
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))

		// Skip blank lines when checking for alignment
		if len(row) == 1 {
			continue
		}

		// Determine whether we are aligning
		alignment := charactersToValue(scanner.Text())
		if previousAlignment == -1 {
			previousAlignment = alignment
		}

		r.Equal(previousAlignment, alignment)
	}

	// There should be no more text
	r.False(scanner.Scan())
}

func TestPretty_Complex_Slice(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer
	d := &ComplexDisplayer{
		Data: []*Complex{
			{
				Name:        "Test",
				Description: "Test description",
				Version:     12,
				CreatedAt:   time.Now().Add(-5 * time.Second),
				UpdatedAt:   time.Now().Add(-1 * time.Second),
			},
			{
				Name:        "Other",
				Description: "Other description",
				Version:     15,
				CreatedAt:   time.Now().Add(-10 * time.Minute),
				UpdatedAt:   time.Now().Add(-3 * time.Second),
			},
		},
		Default: format.Pretty,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Name:", "Test"},
		{"Description:", "Test", "description"},
		{"Version:", "v12"},
		{"Created", "At:", "5", "seconds", "ago"},
		{"Updated", "At:", "1", "second", "ago"},
		{"---"},
		{"Name:", "Other"},
		{"Description:", "Other", "description"},
		{"Version:", "v15"},
		{"Created", "At:", "10", "minutes", "ago"},
		{"Updated", "At:", "3", "seconds", "ago"},
	}

	previousAlignment := -1
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))

		// Skip blank lines when checking for alignment
		if len(row) == 1 {
			continue
		}

		// Determine whether we are aligning
		alignment := charactersToValue(scanner.Text())
		if previousAlignment == -1 {
			previousAlignment = alignment
		}

		r.Equal(previousAlignment, alignment)
	}

	// There should be no more text
	r.False(scanner.Scan())
}

func TestPretty_Complex_Struct(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer
	d := &ComplexDisplayer{
		Data: []*Complex{
			{
				Name:        "Test",
				Description: "Test description",
				Version:     12,
				CreatedAt:   time.Now().Add(-5 * time.Second),
				UpdatedAt:   time.Now().Add(-1 * time.Second),
			},
		},
		Default: format.Pretty,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Name:", "Test"},
		{"Description:", "Test", "description"},
		{"Version:", "v12"},
		{"Created", "At:", "5", "seconds", "ago"},
		{"Updated", "At:", "1", "second", "ago"},
	}

	previousAlignment := -1
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))

		// Skip blank lines when checking for alignment
		if len(row) == 1 {
			continue
		}

		// Determine whether we are aligning
		alignment := charactersToValue(scanner.Text())
		if previousAlignment == -1 {
			previousAlignment = alignment
		}

		r.Equal(previousAlignment, alignment)
	}

	// There should be no more text
	r.False(scanner.Scan())
}

// charactersToValue returns the number of characters to the first value.
// For an input of "My Key:   My Value" the returned value will be 10.
// "My Key" (6) + ":" (1) + "   " (3) = 10.
func charactersToValue(line string) int {
	// Split on the colon
	parts := strings.Split(line, ":")
	label := parts[0]
	prefixedValue := parts[1]

	// Determine the number of spaces to the first non-space character
	spaces := 0
	for i, c := range prefixedValue {
		spaces = i
		if c != ' ' {
			break
		}
	}

	// Determine whether we are aligning
	return len(label) + 1 + spaces
}
