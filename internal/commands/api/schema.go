// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/openapi"
)

type schemaOperationsLoader func(ctx *cmd.Context) (openapi.Schema, error)

type schemaSearcher interface {
	Search(ctx context.Context, query string, operations []*openapi.Operation, limit int) ([]schemaSearchResult, error)
}

var (
	loadSchemaOperationsForSchemaCommand schemaOperationsLoader = openapi.SchemaFactory
	schemaOperationSearcher              schemaSearcher         = hybridSchemaSearcher{}
)

// NewCmdAPISchema creates the api schema command group.
func NewCmdAPISchema(ctx *cmd.Context) *cmd.Command {
	c := &cmd.Command{
		Name:           "schema",
		ShortHelp:      "Search and inspect API operations",
		NoAuthRequired: true,
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud api schema" }} command group lets you search
		for API operations from the OpenAPI spec and inspect a single operation schema.
		`),
	}

	c.AddChild(newCmdAPISchemaSearch(ctx))
	c.AddChild(newCmdAPISchemaGet(ctx))

	return c
}

func newCmdAPISchemaSearch(ctx *cmd.Context) *cmd.Command {
	return &cmd.Command{
		Name:           "search",
		ShortHelp:      "Search API operations",
		NoAuthRequired: true,
		LongHelp: heredoc.New(ctx.IO).Must(`
		Search API operations by keywords.
		`),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{{
				Name:          "QUERY",
				Documentation: "The search query to match against API operations.",
				Repeatable:    true,
			}},
		},
		Examples: []cmd.Example{{
			Preamble: "Search for workspace operations",
			Command:  "$ tfcloud api schema search workspace",
		}},
		RunF: func(_ *cmd.Command, args []string) error {
			query := strings.Join(args, " ")
			schema, err := openapi.SchemaFactory(ctx)
			if err != nil {
				return err
			}

			results, err := schemaOperationSearcher.Search(ctx.ShutdownCtx, query, schema.Operations(), maxSchemaSearchResults)
			if err != nil {
				return err
			}
			if len(results) == 0 {
				return fmt.Errorf("%s No API operations matched %q", ctx.IO.ColorScheme().FailureIcon(), query)
			}

			return ctx.Output.Display(SchemaSearchResultsDisplayer{
				results: results,
			})
		},
	}
}

// SchemaSearchResultsDisplayer is the displayer for schema search results.
type SchemaSearchResultsDisplayer struct {
	results []schemaSearchResult
}

// Check interface at compile time.
var _ format.Displayer = SchemaSearchResultsDisplayer{}

// DefaultFormat implements the Displayer interface.
func (d SchemaSearchResultsDisplayer) DefaultFormat() format.Format {
	return format.Table
}

// Payload implements the Displayer interface.
func (d SchemaSearchResultsDisplayer) Payload() any {
	return d.results
}

// FieldTemplates implements the Displayer interface.
func (d SchemaSearchResultsDisplayer) FieldTemplates() []format.Field {
	return []format.Field{
		{
			Name:        "Operation ID",
			ValueFormat: "{{ .Operation.OperationID }}",
		},
		{
			Name:        "Method",
			ValueFormat: "{{ .Operation.Method }}",
		},
		{
			Name:        "Path",
			ValueFormat: "{{ .Operation.Path }}",
		},
		{
			Name:        "Summary",
			ValueFormat: "{{ .Operation.Summary }}",
		},
	}
}

func newCmdAPISchemaGet(ctx *cmd.Context) *cmd.Command {
	return &cmd.Command{
		Name:           "get",
		ShortHelp:      "Show API operation schema by operation ID or path",
		NoAuthRequired: true,
		LongHelp: heredoc.New(ctx.IO).Must(`
		Show a trimmed OpenAPI document for a single operationId or all operations on an exact API path.
		`),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{{
				Name:          "OPERATION_ID_OR_PATH",
				Documentation: "An exact OpenAPI operationId or an API path (starting with /) to inspect.",
			}},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Inspect the getWorkspace operation",
				Command:  "$ tfcloud api schema get getWorkspace",
			},
			{
				Preamble: "Show all operations on a path",
				Command:  "$ tfcloud api schema get /organizations/{organization}/workspaces",
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			schema, err := loadSchemaOperationsForSchemaCommand(ctx)
			if err != nil {
				return err
			}

			var result openapi.Schema
			if strings.HasPrefix(args[0], "/") {
				result, err = schema.AtomizePath(args[0])
			} else {
				result, err = schema.AtomizeOperation(args[0])
			}
			if err != nil {
				return err
			}

			body, err := result.MarshalJSON()
			if err != nil {
				return fmt.Errorf("failed to marshal operation schema: %w", err)
			}

			fmt.Fprintln(ctx.IO.Out(), string(body))
			return nil
		},
	}
}
