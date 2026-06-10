// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/openapi"
)

type schemaSearchFixture struct {
	name       string
	query      string
	wantTop3   []string
	operations []*openapi.Operation
}

var testSchemaOperations = []*openapi.Operation{
	{Method: "POST", Path: "/runs/{run_id}/actions/cancel", Operation: openapi3.Operation{OperationID: "cancelRun", Tags: []string{"runs"}, Summary: "Cancel a Run"}},
	{Method: "POST", Path: "/runs/{run_id}/actions/force-cancel", Operation: openapi3.Operation{OperationID: "forceCancelRun", Tags: []string{"runs"}, Summary: "Force cancel a Run"}},
	{Method: "GET", Path: "/runs/{run_id}", Operation: openapi3.Operation{OperationID: "getRun", Tags: []string{"runs"}, Summary: "Get Run details"}},
	{Method: "GET", Path: "/workspaces/{workspace_id}", Operation: openapi3.Operation{OperationID: "getWorkspace", Tags: []string{"workspaces"}, Summary: "Get Workspace"}},
	{Method: "GET", Path: "/organizations/{organization_name}/workspaces", Operation: openapi3.Operation{OperationID: "listWorkspaces", Tags: []string{"workspaces"}, Summary: "List Workspaces"}},
	{Method: "POST", Path: "/workspaces/{workspace_id}/vars", Operation: openapi3.Operation{OperationID: "createWorkspaceVar", Tags: []string{"vars"}, Summary: "Create a Variable"}},
	{Method: "GET", Path: "/workspaces/{workspace_id}/vars", Operation: openapi3.Operation{OperationID: "listWorkspaceVars", Tags: []string{"vars"}, Summary: "List Variables"}},
}

func representativeSchemaSearchFixtures() []schemaSearchFixture {
	return []schemaSearchFixture{
		{
			name:       "workspace query",
			query:      "workspace",
			wantTop3:   []string{"getWorkspace", "listWorkspaces"},
			operations: testSchemaOperations,
		},
		{
			name:       "variable query",
			query:      "variable",
			wantTop3:   []string{"createWorkspaceVar", "listWorkspaceVars"},
			operations: testSchemaOperations,
		},
		{
			name:       "run query",
			query:      "run",
			wantTop3:   []string{"getRun"},
			operations: testSchemaOperations,
		},
	}
}

func TestRepresentativeSchemaQueriesStayWithinResource(t *testing.T) {
	t.Parallel()

	for _, fixture := range representativeSchemaSearchFixtures() {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			results, err := schemaOperationSearcher.Search(context.Background(), fixture.query, fixture.operations, 3)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) == 0 {
				t.Fatal("got no results")
			}

			got := resultIDs(results)
			for _, want := range fixture.wantTop3 {
				if !containsOperation(got, want) {
					t.Fatalf("expected %q in top results, got %v", want, got)
				}
			}
		})
	}
}

func TestHybridSchemaSearcherLimitsAndShapesResults(t *testing.T) {
	t.Parallel()

	operations := make([]*openapi.Operation, 0, 12)
	for i := 0; i < 12; i++ {
		operations = append(operations, &openapi.Operation{
			Method: "GET",
			Path:   "/workspaces/{workspace_id}",
			Operation: openapi3.Operation{
				OperationID: fmt.Sprintf("getWorkspace%c", 'A'+i),
				Tags:        []string{"workspaces"},
				Summary:     "Get Workspace",
			},
		})
	}

	results, err := schemaOperationSearcher.Search(context.Background(), "workspace", operations, maxSchemaSearchResults)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != maxSchemaSearchResults {
		t.Fatalf("got %d results, want %d", len(results), maxSchemaSearchResults)
	}
}

func TestSchemaSearchSummary(t *testing.T) {
	t.Parallel()

	fixtures := representativeSchemaSearchFixtures()
	rows := make([]string, 0, len(fixtures)+1)
	rows = append(rows, fmt.Sprintf("%-18s  %s", "query", "top results"))

	for _, fixture := range fixtures {
		results, err := schemaOperationSearcher.Search(context.Background(), fixture.query, fixture.operations, maxSchemaSearchResults)
		if err != nil {
			t.Fatalf("query=%q error: %v", fixture.query, err)
		}
		rows = append(rows, fmt.Sprintf("%-18s  %s", fixture.name, strings.Join(resultIDs(results[:minInt(len(results), 3)]), ", ")))
	}

	t.Log("\n" + strings.Join(rows, "\n"))
}

func TestSpecBackedSchemaSearch(t *testing.T) {
	schema := openapi.SchemaFactory(testCommandContext(iostreams.Test()))
	if schema == nil {
		t.Fatal("failed to load schema")
	}

	fixtures := []struct {
		name    string
		query   string
		wantTop []string
	}{
		{name: "workspace", query: "workspace", wantTop: []string{"getWorkspace", "listWorkspaces"}},
		{name: "organization", query: "organization", wantTop: []string{"getOrganization", "listOrganizations"}},
		{name: "variable", query: "variable", wantTop: []string{"createWorkspaceVar", "listWorkspaceVars"}},
		{name: "apply run", query: "apply run", wantTop: []string{"applyRun"}},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			results, err := schemaOperationSearcher.Search(context.Background(), fixture.query, schema.Operations(), maxSchemaSearchResults)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) == 0 {
				t.Fatal("got no results")
			}

			got := resultIDs(results)
			for _, want := range fixture.wantTop {
				if !containsOperation(got, want) {
					t.Fatalf("expected %q in spec-backed results, got %v", want, got)
				}
			}
		})
	}
}

func resultIDs(results []schemaSearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.Operation.OperationID)
	}
	return ids
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
