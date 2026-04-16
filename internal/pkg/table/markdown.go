// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package table outputs data in a table format. The package is heavily inspired by
// github.com/gosuri/uitable.
package table

import (
	"strings"
)

// MarkdownTable represents a decorator that renders the data in formatted in a table.
type MarkdownTable struct {
	// rows is the collection of rows in the table
	rows []*row

	// maxColWidth is the maximum allowed width for cells in the table
	maxColWidth uint

	// separator is the separator for columns in the table. Default is "\t"
	separator string
}

// AddRow adds a new row to the table.
func (t *MarkdownTable) AddRow(data ...interface{}) *MarkdownTable {
	r := newRow(data...)
	t.rows = append(t.rows, r)
	return t
}

// String returns the string value of table.
func (t *MarkdownTable) String() string {
	if len(t.rows) == 0 {
		return ""
	}

	// Set the separator string
	t.separator = " | "

	// No width maximum in markdown
	t.maxColWidth = 0

	// determine the width for each column (cell in a row)
	var colWidths []uint
	var rawColWidths []uint
	for _, row := range t.rows {
		for i, cell := range row.cells {
			// resize colwidth array
			if i+1 > len(colWidths) {
				colWidths = append(colWidths, 0)
				rawColWidths = append(rawColWidths, 0)
			}
			cellwidth := cell.lineWidth()
			if cellwidth > rawColWidths[i] {
				rawColWidths[i] = cellwidth
			}

			if t.maxColWidth != 0 && cellwidth > t.maxColWidth {
				cellwidth = t.maxColWidth
			}
			if cellwidth > colWidths[i] {
				colWidths[i] = cellwidth
			}
		}
	}

	var lines []string
	for j, row := range t.rows {
		row.separator = t.separator
		for i, cell := range row.cells {
			cell.width = colWidths[i]
		}
		lines = append(lines, "| "+row.string()+" |")
		if j == 0 {
			// Add a separator after the header row
			lines = append(lines, t.separatorRow(colWidths))
		}
	}
	return strings.Join(lines, "\n")
}

func (t *MarkdownTable) separatorRow(colWidths []uint) string {
	data := make([]any, len(colWidths))
	for i, w := range colWidths {
		data[i] = " " + strings.Repeat("-", int(w)) + " "
	}
	separatorRow := newRow(data...)
	separatorRow.separator = "|"
	return "|" + separatorRow.string() + "|"
}
