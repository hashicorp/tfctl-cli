// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

// table outputs data in a table format. The package is heavily inspired by
// github.com/gosuri/uitable.
package table

import (
	"fmt"
	"strings"

	"github.com/muesli/ansi"
	"github.com/muesli/reflow/padding"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/reflow/wrap"
)

// Table represents a decorator that renders the data in formatted in a table
type Table struct {
	// LineLength is the maximum allowed length of a line in the Table
	LineLength uint

	// Wrap when set to true wraps the contents of the columns when the length exceeds the MaxColWidth
	Wrap bool

	// SeparatorSpaces is the number of spaces between columns
	SeparatorSpaces uint

	// HeaderFormatter is a function that formats the header of the table
	HeaderFormatter func(input string) string

	// FirstColumnFormatter is a function that formats the first column of the table
	FirstColumnFormatter func(input string) string

	// rows is the collection of rows in the table
	rows []*row

	// maxColWidth is the maximum allowed width for cells in the table
	maxColWidth uint

	// separator is the separator for columns in the table. Default is "\t"
	separator string
}

// AddRow adds a new row to the table
func (t *Table) AddRow(data ...interface{}) *Table {
	r := newRow(data...)
	t.rows = append(t.rows, r)
	return t
}

// String returns the string value of table
func (t *Table) String() string {
	if len(t.rows) == 0 {
		return ""
	}

	// Set the separator string
	t.separator = strings.Repeat(" ", int(t.SeparatorSpaces))

	// Determine the maximum column width by subtracting from the total line LineLength
	// the width of column separators and then dividing by the number of columns.
	numHeaders := len(t.rows[0].cells)
	headerSpacing := uint((numHeaders - 1)) * t.SeparatorSpaces
	t.maxColWidth = (t.LineLength - uint(headerSpacing)) / uint(numHeaders)

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

	// If the total width of the table is less than the LineLength, distribute
	// the remaining width to the columns whose colwidth is less than the
	// rawColWidths.
	if t.LineLength > 0 {
		totalWidth := uint(int(t.SeparatorSpaces) * (len(colWidths) - 1))
		for _, w := range colWidths {
			totalWidth += w
		}

		// Determine the remaining width to distribute.
		remainingWidth := t.LineLength - totalWidth
		if remainingWidth > 0 {
			for i, w := range colWidths {
				if desiredWidth := rawColWidths[i]; w < desiredWidth {
					add := desiredWidth - w
					if add > remainingWidth {
						add = remainingWidth
					}
					colWidths[i] += add
					remainingWidth -= add
				}
			}
		}
	}

	var lines []string
	for i, row := range t.rows {
		row.separator = t.separator
		if i == 0 {
			row.headerFormatter = t.HeaderFormatter
		} else {
			row.firstColumnFormatter = t.FirstColumnFormatter
		}
		for i, cell := range row.cells {
			cell.width = colWidths[i]
			cell.wrap = t.Wrap
		}
		lines = append(lines, row.string())
	}
	return strings.Join(lines, "\n")
}

// row represents a row in a table
type row struct {
	// cells is the group of cell for the row
	cells []*cell

	// separator for tabular columns
	separator string

	// headerFormatter is a function that formats the header of the table
	headerFormatter func(input string) string

	// firstColumnFormatter is a function that formats the first column of the table
	firstColumnFormatter func(input string) string
}

// newRow returns a new Row and adds the data to the row
func newRow(data ...interface{}) *row {
	r := &row{cells: make([]*cell, len(data))}
	for i, d := range data {
		r.cells[i] = &cell{data: d}
	}
	return r
}

// string returns the string representation of the row
func (r *row) string() string {
	// get the max number of lines for each cell
	var lc int // line count
	for _, cell := range r.cells {
		if clc := len(strings.Split(cell.string(), "\n")); clc > lc {
			lc = clc
		}
	}

	// allocate a two-dimensional array of cells for each line and add size them
	cells := make([][]*cell, lc)
	for x := 0; x < lc; x++ {
		cells[x] = make([]*cell, len(r.cells))
		for y := 0; y < len(r.cells); y++ {
			cells[x][y] = &cell{width: r.cells[y].width, wrap: r.cells[y].wrap}
		}
	}

	// insert each line in a cell as new cell in the cells array
	for y, cell := range r.cells {
		lines := strings.Split(cell.string(), "\n")
		for x, line := range lines {
			cells[x][y].data = line
		}
	}

	// format each line
	lines := make([]string, lc)
	for x := range lines {
		line := make([]string, len(cells[x]))
		for y := range cells[x] {
			val := cells[x][y].string()
			if r.headerFormatter != nil {
				val = r.headerFormatter(val)
			}
			if y == 0 && r.firstColumnFormatter != nil {
				val = r.firstColumnFormatter(val)
			}

			line[y] = val
		}
		lines[x] = strings.Join(line, r.separator)
	}
	return strings.Join(lines, "\n")
}

// cell represents a column in a row
type cell struct {
	// width is the width of the cell
	width uint

	// wrap when true wraps the contents of the cell when the length exceeds the width
	wrap bool

	// data is the cell data
	data interface{}
}

// lineWidth returns the max width of all the lines in a cell
func (c *cell) lineWidth() uint {
	width := 0
	for _, s := range strings.Split(c.string(), "\n") {
		w := ansi.PrintableRuneWidth(s)
		if w > width {
			width = w
		}
	}
	return uint(width)
}

// string returns the string formated representation of the cell
func (c *cell) string() string {
	if c.data == nil {
		return padding.String(" ", c.width)
	}
	s := fmt.Sprint(c.data)
	if c.width > 0 {
		if c.wrap && uint(ansi.PrintableRuneWidth(s)) > c.width {
			return wrap.String(s, int(c.width))
		} else if !c.wrap && uint(ansi.PrintableRuneWidth(s)) > c.width {
			return truncate.StringWithTail(s, c.width, "...")
		} else if len(s) != 0 {
			return padding.String(s, c.width)
		} else {
			return strings.Repeat(" ", int(c.width))
		}
	}
	return s
}
