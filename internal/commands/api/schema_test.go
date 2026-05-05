package api

import (
	"context"
	_ "embed"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/openapi"
)

//go:embed fixtures/openapi.json
var embeddedOpenAPISpec []byte

func TestSchemaSearchFiltersToSameResource(t *testing.T) {
	t.Parallel()

	results := filterSchemaResultsByResource(parseSchemaSearchIntent("workspace"), []schemaSearchResult{
		{Operation: &openapi.Operation{Method: "GET", Path: "/workspaces/{workspace_id}", Operation: openapi3.Operation{OperationID: "getWorkspace", Tags: []string{"workspaces"}, Summary: "Get Workspace"}}, Confidence: 0.9},
		{Operation: &openapi.Operation{Method: "POST", Path: "/runs/{run_id}/actions/cancel", Operation: openapi3.Operation{OperationID: "cancelRun", Tags: []string{"runs"}, Summary: "Cancel Run"}}, Confidence: 1.0},
		{Operation: &openapi.Operation{Method: "GET", Path: "/organizations/{organization_name}/workspaces", Operation: openapi3.Operation{OperationID: "listWorkspaces", Tags: []string{"workspaces"}, Summary: "List Workspaces"}}, Confidence: 0.8},
	}, 3)
	got := resultIDs(results)
	for _, operationID := range got {
		if !strings.Contains(strings.ToLower(operationID), "workspace") {
			t.Fatalf("expected workspace-related results, got %v", got)
		}
	}
	if containsOperation(got, "listWorkspaceResources") {
		t.Fatalf("expected root workspace query to exclude workspace resources, got %v", got)
	}
}

func TestCmdAPISchemaSearchRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	originalLoader := loadSchemaOperationsForSchemaCommand
	loadSchemaOperationsForSchemaCommand = func(*cmd.Context) (openapi.Schema, error) {
		return openapi.NewFromData(embeddedOpenAPISpec)
	}
	t.Cleanup(func() {
		loadSchemaOperationsForSchemaCommand = originalLoader
	})

	command := newCmdAPISchemaSearch(testCommandContext(io))
	command.SetIO(io)
	r.Equal(0, command.Run([]string{"cancel", "run"}))

	output := io.Output.String()
	r.Contains(output, "getRun")
	r.Contains(output, "/runs/{run_id}")
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaGetRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	originalLoader := loadSchemaOperationsForSchemaCommand
	loadSchemaOperationsForSchemaCommand = func(*cmd.Context) (openapi.Schema, error) {
		return openapi.NewFromData(embeddedOpenAPISpec)
	}
	t.Cleanup(func() {
		loadSchemaOperationsForSchemaCommand = originalLoader
	})

	command := newCmdAPISchemaGet(testCommandContext(io))
	command.SetIO(io)
	r.Equal(0, command.Run([]string{"getWorkspace"}))

	output := io.Output.String()
	r.Contains(output, `"operationId":"getWorkspace"`)
	r.Contains(output, `"/workspaces/{workspace_id}"`)
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaGetByPath(t *testing.T) {
	r := require.New(t)

	io := iostreams.Test()
	originalLoader := loadSchemaOperationsForSchemaCommand
	loadSchemaOperationsForSchemaCommand = func(ctx *cmd.Context) (openapi.Schema, error) {
		return openapi.NewFromData(embeddedOpenAPISpec)
	}
	t.Cleanup(func() {
		loadSchemaOperationsForSchemaCommand = originalLoader
	})

	command := newCmdAPISchemaGet(testCommandContext(io))
	command.SetIO(io)
	r.Equal(0, command.Run([]string{"/workspaces/{workspace_id}/vars"}))

	output := io.Output.String()
	r.Contains(output, `"listWorkspaceVars"`)
	r.Contains(output, `"createWorkspaceVar"`)
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaGetByPathNotFound(t *testing.T) {
	r := require.New(t)

	io := iostreams.Test()
	originalLoader := loadSchemaOperationsForSchemaCommand
	loadSchemaOperationsForSchemaCommand = func(*cmd.Context) (openapi.Schema, error) {
		return openapi.NewFromData(embeddedOpenAPISpec)
	}
	t.Cleanup(func() {
		loadSchemaOperationsForSchemaCommand = originalLoader
	})

	command := newCmdAPISchemaGet(testCommandContext(io))
	command.SetIO(io)
	r.Equal(1, command.Run([]string{"/nonexistent"}))
}

func testCommandContext(io *iostreams.Testing) *cmd.Context {
	return &cmd.Context{
		IO:          io,
		Output:      format.New(io),
		ShutdownCtx: context.Background(),
	}
}

func containsOperation(operations []string, want string) bool {
	for _, operation := range operations {
		if operation == want {
			return true
		}
	}
	return false
}
