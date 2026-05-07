// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
)

func TestMarkdown_StringPayload(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)
	out.SetFormat(format.Markdown)

	d := &StringPayloadDisplayer{
		payload:      map[string]string{"key": "value"},
		markdownText: "**formatted** markdown output",
	}

	r.NoError(out.Display(d))
	r.Equal("**formatted** markdown output\n", io.Output.String())
}

func TestMarkdown_Complex_Slice(t *testing.T) {
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
		Default: format.Markdown,
	}

	// Display the table
	r.NoError(out.Display(d))

	expected := []string{
		"| Name  | Description       | Version | Created At     | Updated At    |",
		"| ----- | ----------------- | ------- | -------------- | ------------- |",
		"| Test  | Test description  | v12     | 5 seconds ago  | 1 second ago  |",
		"| Other | Other description | v15     | 10 minutes ago | 3 seconds ago |",
	}
	r.Equal(strings.TrimSpace(io.Output.String()), strings.TrimSpace(strings.Join(expected, "\n")))
}

func TestMarkdown_Complex_Struct(t *testing.T) {
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
		Default: format.Markdown,
	}

	// Display the table
	r.NoError(out.Display(d))

	// Check the output is expected
	expected := []string{
		"| Field       | Value            |",
		"| ----------- | ---------------- |",
		"| Name        | Test             |",
		"| Description | Test description |",
		"| Version     | v12              |",
		"| Created At  | 5 seconds ago    |",
		"| Updated At  | 1 second ago     |",
	}
	r.Equal(strings.TrimSpace(io.Output.String()), strings.TrimSpace(strings.Join(expected, "\n")))
}

type KVMarkdownFormatter struct {
	KVDisplayer
}

func TestMarkdown_TableFormatter(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer
	d := &KVMarkdownFormatter{
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
			Default: format.Markdown,
		},
	}

	// Display the table
	r.NoError(out.Display(d))

	// Check the output is expected
	expected := []string{
		"| Key     | Value  |",
		"| ------- | ------ |",
		"| HELLO   | World! |",
		"| ANOTHER | Test   |",
	}
	r.Equal(strings.TrimSpace(io.Output.String()), strings.TrimSpace(strings.Join(expected, "\n")))
}
