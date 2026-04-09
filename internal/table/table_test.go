// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package table

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This test ensures that we expand columns to fill the width of the line if the
// total output is less than the line width.
func TestTable_LessThanLineWidth(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	tbl := &Table{
		LineLength:      22,
		Wrap:            true,
		SeparatorSpaces: 2,
	}

	tbl.AddRow("header1", "header2")    // 7 + 2 + 7  = 16
	tbl.AddRow("short", "alongervalue") // 5 + 2 + 12 = 19
	tbl.AddRow("medium", "medium")      // 6 + 2 + 6  = 14

	// Only have two rows and expect no extra lines.
	out := tbl.String()
	r.Len(strings.Split(out, "\n"), 3, out)
	r.Len(tbl.rows, 3)

	// Expect the cell width to be equal to the header for the first column and
	// the length of the longest value for the second.
	r.Equal(uint(7), tbl.rows[0].cells[0].width)
	r.Equal(uint(12), tbl.rows[1].cells[1].width)
}

// This test ensures that column width is evenly distributed when the overall
// output exceeds the line length.
func TestTable_MoreThanLineWidth(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	tbl := &Table{
		LineLength:      18,
		Wrap:            true,
		SeparatorSpaces: 2,
	}

	tbl.AddRow("header1", "header2")           // 7  + 2 + 7  = 16
	tbl.AddRow("short", "alongervalue")        // 5  + 2 + 12 = 19
	tbl.AddRow("alongervalue", "alongervalue") // 12 + 2 + 12 = 26

	// Expect both rows to be wrapped.
	// header1   header2
	// short     alongerv
	//           alue
	// alongerv  alongerv
	// alue      alue
	out := tbl.String()
	r.Len(strings.Split(out, "\n"), 5, out)
	r.Len(tbl.rows, 3)

	// Expect the cell width to be an equal distribution across the line width.
	// (18 - 2) / 2 = 8
	r.Equal(uint(8), tbl.rows[0].cells[0].width)
	r.Equal(uint(8), tbl.rows[1].cells[1].width)
}

func TestCell(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	c := &cell{
		data:  "foo bar",
		width: 5,
	}

	got := c.string()
	r.Equal("fo...", got)
	r.EqualValues(5, c.lineWidth())

	c.wrap = true
	got = c.string()
	r.Equal("foo b\nar", got)
	r.EqualValues(5, c.lineWidth())
}

func TestRow(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	row := &row{
		separator: "  ",
		cells: []*cell{
			{data: "foo", width: 3, wrap: true},
			{data: "bar baz", width: 3, wrap: true},
		},
	}
	need := "foo  bar\n     baz"
	r.Equal(need, row.string())
}
