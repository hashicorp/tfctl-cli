// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package table

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarkdownTable(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	tbl := &MarkdownTable{}

	tbl.AddRow("header1", "header2")
	tbl.AddRow("short", strings.Repeat("a long value ", 10))
	tbl.AddRow("medium", "medium")

	out := tbl.String()

	expected := []string{
		"| header1 | header2                                                                                                                            |",
		"| ------- | ---------------------------------------------------------------------------------------------------------------------------------- |",
		"| short   | a long value a long value a long value a long value a long value a long value a long value a long value a long value a long value  |",
		"| medium  | medium                                                                                                                             |",
	}

	r.Equal(expected, strings.Split(out, "\n"))
}
