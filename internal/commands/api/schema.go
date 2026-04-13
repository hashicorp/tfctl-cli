package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/render"
)

type schemaOperation struct {
	OperationID string
	Method      string
	Path        string
	Tags        []string
	Summary     string
}

type schemaOperationsLoader func(ctx *cmd.Context) ([]schemaOperation, error)
type schemaDocumentLoader func(ctx *cmd.Context) (map[string]any, error)

type schemaSearcher interface {
	Search(ctx context.Context, query string, operations []schemaOperation, limit int) ([]schemaSearchResult, error)
}

var (
	loadSchemaOperationsForSearch schemaOperationsLoader = cachedSchemaOperations
	loadSchemaDocumentForGet      schemaDocumentLoader   = cachedSchemaDocument
	schemaOperationSearcher       schemaSearcher         = hybridSchemaSearcher{}
)

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
		Search API operations from the OpenAPI spec. Results are ranked by Bluge and
		rendered as a compact table when possible.
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
			query := joinSchemaQuery(args)
			operations, err := loadSchemaOperationsForSearch(ctx)
			if err != nil {
				return err
			}

			results, err := schemaOperationSearcher.Search(schemaSearchContext(ctx), query, operations, maxSchemaSearchResults)
			if err != nil {
				return err
			}
			if len(results) == 0 {
				writeSchemaNoResults(ctx, query)
				return nil
			}

			body, err := schemaSearchJSONAPIResponse(results)
			if err != nil {
				return err
			}

			table, ok, err := render.JSONAPITable(body)
			if err != nil {
				return err
			}
			if ok {
				_, _ = ctx.IO.Out().Write([]byte(table))
				return nil
			}

			_, _ = ctx.IO.Out().Write(body)
			return nil
		},
	}
}

func newCmdAPISchemaGet(ctx *cmd.Context) *cmd.Command {
	return &cmd.Command{
		Name:           "get",
		ShortHelp:      "Show one API operation schema",
		NoAuthRequired: true,
		LongHelp: heredoc.New(ctx.IO).Must(`
		Show a trimmed OpenAPI document for a single operationId.
		`),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{{
				Name:          "OPERATION_ID",
				Documentation: "The exact OpenAPI operationId to inspect.",
			}},
		},
		Examples: []cmd.Example{{
			Preamble: "Inspect the getWorkspace operation",
			Command:  "$ tfcloud api schema get getWorkspace",
		}},
		RunF: func(_ *cmd.Command, args []string) error {
			document, err := loadSchemaDocumentForGet(ctx)
			if err != nil {
				return err
			}

			operationDocument, err := schemaOperationDocument(document, args[0])
			if err != nil {
				return err
			}

			body, err := json.MarshalIndent(operationDocument, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal operation schema: %w", err)
			}

			_, _ = ctx.IO.Out().Write(append(body, '\n'))
			return nil
		},
	}
}

func joinSchemaQuery(args []string) string {
	return strings.Join(args, " ")
}

func writeSchemaNoResults(ctx *cmd.Context, query string) {
	cs := ctx.IO.ColorScheme()
	_, _ = fmt.Fprintf(ctx.IO.Out(), "%s No API operations matched %q.\n", cs.WarningLabel(), query)
}

func schemaSearchContext(ctx *cmd.Context) context.Context {
	if ctx != nil && ctx.ShutdownCtx != nil {
		return ctx.ShutdownCtx
	}
	return context.Background()
}
