// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	_ "embed"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/openapi"
)

//go:embed fixtures/openapi.json
var embeddedFixtureSpec []byte

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
	cCtx := testCommandContext(io)

	err := runSchemaSearch(schemaSearchOpts{
		IO:          io,
		Output:      cCtx.Output,
		ShutdownCtx: cCtx.ShutdownCtx,
		LoadSchema:  fixtureSchemaLoader(t),
		Searcher:    schemaOperationSearcher,
		Query:       "cancel run",
	})
	r.NoError(err)

	output := io.Output.String()
	r.Contains(output, "getRun")
	r.Contains(output, "/runs/{run_id}")
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaGetRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()

	err := runSchemaGet(schemaGetOpts{
		IO:         io,
		LoadSchema: fixtureSchemaLoader(t),
		Target:     "getWorkspace",
	})
	r.NoError(err)

	output := io.Output.String()
	r.Contains(output, `"operationId":"getWorkspace"`)
	r.Contains(output, `"/workspaces/{workspace_id}"`)
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaGetByPath(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()

	err := runSchemaGet(schemaGetOpts{
		IO:         io,
		LoadSchema: fixtureSchemaLoader(t),
		Target:     "/workspaces/{workspace_id}/vars",
	})
	r.NoError(err)

	output := io.Output.String()
	r.Contains(output, `"listWorkspaceVars"`)
	r.Contains(output, `"createWorkspaceVar"`)
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaGetByPathNotFound(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()

	err := runSchemaGet(schemaGetOpts{
		IO:         io,
		LoadSchema: fixtureSchemaLoader(t),
		Target:     "/nonexistent",
	})
	r.Error(err)
}

// The following tests drive the full command.Run path (flag parsing, arg-count
// validation, exit-code mapping, IO wiring) with a per-command injected loader,
// so they exercise the RunF closures and constructor wiring while staying free
// of any shared package state.

func TestCmdAPISchemaSearchCommandRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	cCtx := testCommandContext(io)

	command := newCmdAPISchemaSearch(cCtx, withSchemaLoader(fixtureSchemaLoader(t)))
	command.SetIO(io)

	// Multiple args exercise strings.Join and MinimumNArgs(1) validation.
	r.Equal(0, command.Run([]string{"cancel", "run"}, cCtx))

	output := io.Output.String()
	r.Contains(output, "getRun")
	r.Contains(output, "/runs/{run_id}")
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaSearchCommandRequiresArg(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	cCtx := testCommandContext(io)

	command := newCmdAPISchemaSearch(cCtx, withSchemaLoader(fixtureSchemaLoader(t)))
	command.SetIO(io)

	// MinimumNArgs(1): missing QUERY must fail before RunF runs.
	r.Equal(1, command.Run([]string{}, cCtx))
	r.NotEmpty(io.Error.String())
}

func TestCmdAPISchemaGetCommandRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	cCtx := testCommandContext(io)

	command := newCmdAPISchemaGet(cCtx, withSchemaLoader(fixtureSchemaLoader(t)))
	command.SetIO(io)

	r.Equal(0, command.Run([]string{"getWorkspace"}, cCtx))

	output := io.Output.String()
	r.Contains(output, `"operationId":"getWorkspace"`)
	r.Contains(output, `"/workspaces/{workspace_id}"`)
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaGetCommandRequiresExactlyOneArg(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// ExactArgs(1): both too few and too many args must fail in validation,
	// before the RunF closure indexes args[0].
	for _, args := range [][]string{{}, {"getWorkspace", "extra"}} {
		io := iostreams.Test()
		cCtx := testCommandContext(io)

		command := newCmdAPISchemaGet(cCtx, withSchemaLoader(fixtureSchemaLoader(t)))
		command.SetIO(io)

		r.Equal(1, command.Run(args, cCtx))
		r.NotEmpty(io.Error.String())
		r.Empty(io.Output.String())
	}
}

func TestCmdAPISchemaGetCommandNotFoundExitCode(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	cCtx := testCommandContext(io)

	command := newCmdAPISchemaGet(cCtx, withSchemaLoader(fixtureSchemaLoader(t)))
	command.SetIO(io)

	// A RunF error maps to a non-zero exit code via command.Run.
	r.Equal(1, command.Run([]string{"/nonexistent"}, cCtx))
}

// fixtureSchemaLoader returns a loader closure backed by the embedded test
// fixture spec, suitable for injecting into schemaSearchOpts/schemaGetOpts.
func fixtureSchemaLoader(t *testing.T) func() openapi.Schema {
	t.Helper()
	return func() openapi.Schema {
		schema, err := openapi.NewFromData(embeddedFixtureSpec)
		if err != nil {
			t.Fatalf("failed to load test schema: %v", err)
		}
		return schema
	}
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
