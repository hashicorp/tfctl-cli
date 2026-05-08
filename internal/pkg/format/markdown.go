// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"bytes"
	"fmt"
	"reflect"
	"text/template"

	"github.com/hashicorp/tfctl-cli/internal/pkg/table"
)

// outputMarkdown outputs the payload as a markdown table.
func (o *Outputter) outputMarkdown(d Displayer) error {
	if sp, ok := d.(StringPayload); ok {
		fmt.Fprintln(o.io.Out(), sp.StringPayload(Markdown))
		return nil
	}

	// Gather the headers and the row template
	fields := d.FieldTemplates()
	headers := make([]interface{}, len(fields))
	for i, f := range fields {
		headers[i] = f.Name
	}

	// Create the table outputter
	tbl := table.MarkdownTable{}

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
		// Add the headers
		tbl.AddRow(headers...)

		for i := 0; i < rv.Len(); i++ {
			vf := rv.Index(i)
			row, err := renderRow(vf.Interface(), fields)
			if err != nil {
				return err
			}

			rows = append(rows, row)
		}
	} else {
		// Pivot the table if the payload is not a slice, rendering each field as a row and adding
		// it to the table.
		tbl.AddRow("Field", "Value")
		for _, f := range fields {
			row, err := renderNameValue(p, f.Name, fields)
			if err != nil {
				return err
			}

			rows = append(rows, row)
		}
	}

	for _, row := range rows {
		tbl.AddRow(row...)
	}

	// Output the table
	fmt.Fprintln(o.io.Out())
	fmt.Fprintln(o.io.Out(), tbl.String())
	fmt.Fprintln(o.io.Out())
	return nil
}

// renderRow renders each field by executing the text/template given the
// payload.
func renderNameValue(p any, name string, fields []Field) ([]interface{}, error) {
	renderedFields := make([]interface{}, 2)
	renderedFields[0] = name

	for _, f := range fields {
		if f.Name != name {
			continue
		}
		tmpl, err := template.New("cli").Parse(f.ValueFormat)
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, p); err != nil {
			return nil, err
		}

		renderedFields[1] = buf.String()
	}

	return renderedFields, nil
}
