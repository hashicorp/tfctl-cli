// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"fmt"
	"strings"

	"github.com/hashicorp/tfcloud/internal/format"
	"github.com/hashicorp/tfcloud/internal/iostreams"
)

func ExampleOutputter() {
	// Create the outputter. This is typically passed to the command.
	io := iostreams.Test()
	outputter := format.New(io)

	// Resource is an example resource that we want to display
	type Metadata struct {
		Owner     string
		CreatedAt string
	}

	type Resource struct {
		Name        string
		ID          string
		Description string
		Bytes       int
		Metadata    Metadata
	}

	// Build our mock resources. Typically this is the response payload from an API
	// request.
	payload := []Resource{
		{
			Name:        "hello",
			ID:          "123",
			Description: "world",
			Bytes:       100,
			Metadata: Metadata{
				Owner:     "Bob Builder",
				CreatedAt: "2021-01-01",
			},
		},
		{
			Name:        "another",
			ID:          "456",
			Description: "example",
			Bytes:       1024 * 1024,
			Metadata: Metadata{
				Owner:     "Jeff Bezos",
				CreatedAt: "2023-02-04",
			},
		},
	}

	// For displaying a table of the exact values, Show can be used:
	_ = outputter.Show(payload, format.Table)

	// For displaying a table with a subset of the fields, list the fields as
	// such:
	// _ = outputter.Show(payload, format.Table, "Name", "ID", "Metadata.Owner")

	// Since the IO is a test io, manually print it.
	// We trim the lines to make examples testing pass correctly.
	lines := strings.Split(io.Output.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	fmt.Println(strings.Join(lines, "\n"))

	// Output:
	// Name      ID    Description   Bytes     Metadata Owner   Metadata Created At
	// hello     123   world         100       Bob Builder      2021-01-01
	// another   456   example       1048576   Jeff Bezos       2023-02-04
	//

}

func ExampleDisplayer() {
	// Create the outputter. This is typically passed to the command.
	io := iostreams.Test()
	outputter := format.New(io)

	// Resource is an example resource that we want to display
	type Resource struct {
		Name        string
		ID          string
		Description string
		Bytes       int
	}

	// Build our mock resources. Typically this is the response payload from an API
	// request.
	payload := []Resource{
		{
			Name:        "hello",
			ID:          "123",
			Description: "world",
			Bytes:       100,
		},
		{
			Name:        "another",
			ID:          "456",
			Description: "example",
			Bytes:       1024 * 1024,
		},
	}

	// If you wish to format the values differently, use a Displayer:

	// Define the fields that we want to ExampleDisplayer
	var fields = []format.Field{
		format.NewField("Name", "{{ .Name }}"),
		format.NewField("ID", "{{ .ID }}"),
		format.NewField("Description", "{{ .Description }}"),
		format.NewField("Bytes", "{{ .Bytes }} bytes"),
	}

	// Build the displayer
	d := format.NewDisplayer(payload, format.Pretty, fields)

	// Run the displayer
	if err := outputter.Display(d); err != nil {
		fmt.Printf("error displaying resources: %s\n", err)
	}

	// Since the IO is a test io, manually print it
	fmt.Println(io.Output.String())

	// Output:
	// Name:        hello
	// ID:          123
	// Description: world
	// Bytes:       100 bytes
	// ---
	// Name:        another
	// ID:          456
	// Description: example
	// Bytes:       1048576 bytes
}
