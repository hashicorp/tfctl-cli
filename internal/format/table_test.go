// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"bufio"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/tfcloud/internal/format"
	"github.com/hashicorp/tfcloud/internal/iostreams"
	"github.com/stretchr/testify/require"
)

func TestTable_KV_Slice(t *testing.T) {
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
		Default: format.Table,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Key", "Value"},
		{"Hello", "World!"},
		{"Another", "Test"},
	}
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))
	}

	// There should be no more text
	r.False(scanner.Scan())
}

func TestTable_KV_Struct(t *testing.T) {
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
		Default: format.Table,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Key", "Value"},
		{"Hello", "World!"},
	}
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))
	}

	// There should be no more text
	r.False(scanner.Scan())
}

func TestTable_Complex_Slice(t *testing.T) {
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
		Default: format.Table,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Name", "Description", "Version", "Created", "At", "Updated", "At"},
		{"Test", "Test", "description", "v12", "5", "seconds", "ago", "1", "second", "ago"},
		{"Other", "Other", "description", "v15", "10", "minutes", "ago", "3", "seconds", "ago"},
	}
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))
	}

	// There should be no more text
	r.False(scanner.Scan())
}

func TestTable_Complex_Struct(t *testing.T) {
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
		Default: format.Table,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"Name", "Description", "Version", "Created", "At", "Updated", "At"},
		{"Test", "Test", "description", "v12", "5", "seconds", "ago", "1", "second", "ago"},
	}
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))
	}

	// There should be no more text
	r.False(scanner.Scan())
}

type KVTableFormatter struct {
	KVDisplayer
}

func (f *KVTableFormatter) HeaderFormatter(input string) string {
	return strings.ToUpper(input)
}

func (f *KVTableFormatter) FirstColumnFormatter(input string) string {
	return strings.ToLower(input)
}

func TestTable_TableFormatter(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer
	d := &KVTableFormatter{
		KVDisplayer{
			KVs: []*KV{
				{
					Key:   "HELLO",
					Value: "World!",
				},
				{
					Key:   "ANOTHER",
					Value: "Test",
				},
			},
			Default: format.Table,
		},
	}

	// Display the table
	r.NoError(out.Display(d))

	// Create a scanner to check the output
	scanner := bufio.NewScanner(io.Output)

	// Check the output is expected
	expected := [][]string{
		{"KEY", "VALUE"},
		{"hello", "World!"},
		{"another", "Test"},
	}
	for _, row := range expected {
		r.True(scanner.Scan())
		r.Equal(row, strings.Fields(scanner.Text()))
	}

	// There should be no more text
	r.False(scanner.Scan())
}
