// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/openapi"
	"github.com/hashicorp/tfctl-cli/version"
)

type schemaOperationsLoader func(ctx *cmd.Context) openapi.Schema

type schemaSearcher interface {
	Search(ctx context.Context, query string, operations []*openapi.Operation, limit int) ([]schemaSearchResult, error)
}

// schemaOperationSearcher is the default searcher used by the schema commands.
// It is stateless and only ever read, so it is safe to share across commands.
var schemaOperationSearcher schemaSearcher = hybridSchemaSearcher{}

// schemaSearchOpts carries the dependencies and parsed inputs for the
// `api schema search` command so that runSchemaSearch can be tested in
// isolation without mutating any package-level state.
type schemaSearchOpts struct {
	IO          iostreams.IOStreams
	Output      *format.Outputter
	ShutdownCtx context.Context
	LoadSchema  func() openapi.Schema
	Searcher    schemaSearcher
	Query       string
}

// schemaGetOpts carries the dependencies and parsed inputs for the
// `api schema get` command so that runSchemaGet can be tested in isolation
// without mutating any package-level state.
type schemaGetOpts struct {
	IO         iostreams.IOStreams
	LoadSchema func() openapi.Schema
	Target     string
}

// defaultSchemaLoader binds the command context and logger into a loader
// closure that fetches the OpenAPI schema via the production SchemaFactory.
func defaultSchemaLoader(ctx *cmd.Context, c *cmd.Command) func() openapi.Schema {
	return func() openapi.Schema {
		return openapi.SchemaFactory(ctx)
	}
}

// schemaCmdConfig holds optional, construction-time overrides for the schema
// subcommands. It lets tests drive the full command.Run path (flag parsing,
// arg-count validation, exit-code mapping, IO wiring) with an injected loader
// instead of mutating package-level state, keeping parallel tests race-free.
type schemaCmdConfig struct {
	// loadSchema, when non-nil, overrides the production schema loader. When
	// nil, the command builds defaultSchemaLoader at run time.
	loadSchema func() openapi.Schema
}

// schemaCmdOption customizes a schema subcommand at construction time.
type schemaCmdOption func(*schemaCmdConfig)

// newSchemaCmdConfig applies the given options over the production defaults.
func newSchemaCmdConfig(opts ...schemaCmdOption) schemaCmdConfig {
	cfg := schemaCmdConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// withSchemaLoader overrides the schema loader, primarily for tests that want
// to exercise the full command.Run path against a fixture schema.
func withSchemaLoader(load func() openapi.Schema) schemaCmdOption {
	return func(cfg *schemaCmdConfig) {
		cfg.loadSchema = load
	}
}

// NewCmdAPISchema creates the api schema command group.
func NewCmdAPISchema(ctx *cmd.Context) *cmd.Command {
	c := &cmd.Command{
		Name:           "schema",
		ShortHelp:      "Search and inspect API operations",
		NoAuthRequired: true,
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s api schema" }} command group lets you search
		for API operations from the OpenAPI spec and inspect a single operation schema.
		`, version.Name),
	}

	c.AddChild(newCmdAPISchemaSearch(ctx))
	c.AddChild(newCmdAPISchemaGet(ctx))

	return c
}

func newCmdAPISchemaSearch(ctx *cmd.Context, opts ...schemaCmdOption) *cmd.Command {
	cfg := newSchemaCmdConfig(opts...)
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
			Command:  fmt.Sprintf("$ %s api schema search workspace", version.Name),
		}},
		RunF: func(c *cmd.Command, args []string) error {
			load := cfg.loadSchema
			if load == nil {
				load = defaultSchemaLoader(ctx, c)
			}
			return runSchemaSearch(schemaSearchOpts{
				IO:          ctx.IO,
				Output:      ctx.Output,
				ShutdownCtx: ctx.ShutdownCtx,
				LoadSchema:  load,
				Searcher:    schemaOperationSearcher,
				Query:       strings.Join(args, " "),
			})
		},
	}
}

// runSchemaSearch searches the loaded schema operations and renders the results.
func runSchemaSearch(opts schemaSearchOpts) error {
	schema := opts.LoadSchema()

	results, err := opts.Searcher.Search(opts.ShutdownCtx, opts.Query, schema.Operations(), maxSchemaSearchResults)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("%s No API operations matched %q", opts.IO.ColorScheme().FailureIcon(), opts.Query)
	}

	return opts.Output.Display(SchemaSearchResultsDisplayer{
		results: results,
	})
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

func newCmdAPISchemaGet(ctx *cmd.Context, opts ...schemaCmdOption) *cmd.Command {
	cfg := newSchemaCmdConfig(opts...)
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
				Command:  fmt.Sprintf("$ %s api schema get getWorkspace", version.Name),
			},
			{
				Preamble: "Show all operations on a path",
				Command:  fmt.Sprintf("$ %s api schema get /organizations/{organization}/workspaces", version.Name),
			},
		},
		RunF: func(c *cmd.Command, args []string) error {
			load := cfg.loadSchema
			if load == nil {
				load = defaultSchemaLoader(ctx, c)
			}
			return runSchemaGet(schemaGetOpts{
				IO:         ctx.IO,
				LoadSchema: load,
				Target:     args[0],
			})
		},
	}
}

// runSchemaGet renders a trimmed OpenAPI document for a single operationId or
// all operations on an exact API path.
func runSchemaGet(opts schemaGetOpts) error {
	schema := opts.LoadSchema()

	var err error
	var result openapi.Schema
	if strings.HasPrefix(opts.Target, "/") {
		result, err = schema.AtomizePath(opts.Target)
	} else {
		result, err = schema.AtomizeOperation(opts.Target)
	}
	if err != nil {
		return err
	}

	body, err := result.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal operation schema: %w", err)
	}

	fmt.Fprintln(opts.IO.Out(), string(body))
	return nil
}
