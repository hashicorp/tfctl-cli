// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"github.com/hashicorp/tfcloud/internal/pkg/table"
)

// TableFormatter is an optional interface to implement to customize how a table
// is outputted.
type TableFormatter interface {
	HeaderFormatter(input string) string
	FirstColumnFormatter(input string) string
}

// outputTable outputs the payload as a table.
func (o *Outputter) outputTable(d Displayer) error {
	// Gather the headers and the row template
	fields := d.FieldTemplates()
	headers := make([]interface{}, len(fields))
	for i, f := range fields {
		headers[i] = f.Name
	}

	// Create the table outputter
	tbl := table.Table{
		LineLength:           uint(o.io.TerminalWidth()),
		Wrap:                 true,
		SeparatorSpaces:      3,
		HeaderFormatter:      o.defaultHeaderFormatter,
		FirstColumnFormatter: o.defaultFirstColumnFormatter,
	}

	// If the displayer has implemented the table formatter, then use it.
	if formatter, ok := d.(TableFormatter); ok {
		tbl.HeaderFormatter = formatter.HeaderFormatter
		tbl.FirstColumnFormatter = formatter.FirstColumnFormatter
	}

	// Add the headers
	tbl.AddRow(headers...)

	// Get the payload
	var p any
	if tp, ok := d.(TemplatedPayload); ok {
		p = tp.TemplatedPayload()
	} else {
		p = d.Payload()
	}

	// Build the rows
	var rows [][]interface{}
	rv := reflect.ValueOf(p)

	// If the payload is a slice, render each row and add it to the table.
	if rv.Kind() == reflect.Slice {
		for i := 0; i < rv.Len(); i++ {
			vf := rv.Index(i)
			row, err := renderRow(vf.Interface(), fields)
			if err != nil {
				return err
			}

			rows = append(rows, row)
		}
	} else {
		// Render the payload as a row and add it to the table.
		row, err := renderRow(p, fields)
		if err != nil {
			return err
		}

		rows = append(rows, row)
	}

	for _, row := range rows {
		tbl.AddRow(row...)
	}

	// Output the table
	fmt.Fprintln(o.io.Out(), tbl.String())
	return nil
}

// defaultHeaderFormatter is the default header formatter which prints the
// header in green.
func (o *Outputter) defaultHeaderFormatter(input string) string {
	nonPadded := strings.TrimRight(input, " ")
	cs := o.io.ColorScheme()
	return cs.String(nonPadded).Color(cs.Green()).Underline().String() +
		strings.Repeat(" ", len(input)-len(nonPadded))
}

// defaultFirstColumnFormatter is the default first column formatter which
// prints the column in yellow.
func (o *Outputter) defaultFirstColumnFormatter(input string) string {
	cs := o.io.ColorScheme()
	return cs.String(input).Color(cs.Yellow()).String()
}

// renderRow renders each field by executing the text/template given the
// payload.
func renderRow(p any, fields []Field) ([]interface{}, error) {
	renderedFields := make([]interface{}, len(fields))
	for i, f := range fields {
		tmpl, err := template.New("tfcloud").Parse(f.ValueFormat)
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, p); err != nil {
			return nil, err
		}

		renderedFields[i] = buf.String()
	}

	return renderedFields, nil
}
