package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
)

var testSchemaOperations = []schemaOperation{
	{OperationID: "cancelRun", Method: "POST", Path: "/runs/{run_id}/actions/cancel", Tags: []string{"runs"}, Summary: "Cancel a Run"},
	{OperationID: "forceCancelRun", Method: "POST", Path: "/runs/{run_id}/actions/force-cancel", Tags: []string{"runs"}, Summary: "Force cancel a Run"},
	{OperationID: "getRun", Method: "GET", Path: "/runs/{run_id}", Tags: []string{"runs"}, Summary: "Get Run details"},
	{OperationID: "getWorkspace", Method: "GET", Path: "/workspaces/{workspace_id}", Tags: []string{"workspaces"}, Summary: "Get Workspace"},
	{OperationID: "listWorkspaces", Method: "GET", Path: "/organizations/{organization_name}/workspaces", Tags: []string{"workspaces"}, Summary: "List Workspaces"},
	{OperationID: "createWorkspaceVar", Method: "POST", Path: "/workspaces/{workspace_id}/vars", Tags: []string{"vars"}, Summary: "Create a Variable"},
	{OperationID: "listWorkspaceVars", Method: "GET", Path: "/workspaces/{workspace_id}/vars", Tags: []string{"vars"}, Summary: "List Variables"},
}

func TestSchemaSearchFiltersToSameResource(t *testing.T) {
	t.Parallel()

	results := filterSchemaResultsByResource(parseSchemaSearchIntent("workspace"), []schemaSearchResult{
		{Operation: schemaOperation{OperationID: "getWorkspace", Method: "GET", Path: "/workspaces/{workspace_id}", Tags: []string{"workspaces"}, Summary: "Get Workspace"}, Confidence: 0.9},
		{Operation: schemaOperation{OperationID: "cancelRun", Method: "POST", Path: "/runs/{run_id}/actions/cancel", Tags: []string{"runs"}, Summary: "Cancel Run"}, Confidence: 1.0},
		{Operation: schemaOperation{OperationID: "listWorkspaces", Method: "GET", Path: "/organizations/{organization_name}/workspaces", Tags: []string{"workspaces"}, Summary: "List Workspaces"}, Confidence: 0.8},
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
	originalLoader := loadSchemaOperationsForSearch
	originalSearcher := schemaOperationSearcher
	loadSchemaOperationsForSearch = func() ([]schemaOperation, error) {
		return testSchemaOperations, nil
	}
	schemaOperationSearcher = hybridSchemaSearcher{}
	t.Cleanup(func() {
		loadSchemaOperationsForSearch = originalLoader
		schemaOperationSearcher = originalSearcher
	})

	command := newCmdAPISchemaSearch(testCommandContext(io))
	command.SetIO(io)
	r.Equal(0, command.Run([]string{"cancel", "run"}))

	output := io.Output.String()
	r.Contains(output, "operation-id")
	r.Contains(output, "getRun")
	r.Contains(output, "/runs/{run_id}")
	r.Empty(io.Error.String())
}

func TestCmdAPISchemaSearchRunNoResults(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	originalLoader := loadSchemaOperationsForSearch
	originalSearcher := schemaOperationSearcher
	loadSchemaOperationsForSearch = func() ([]schemaOperation, error) {
		return testSchemaOperations, nil
	}
	schemaOperationSearcher = hybridSchemaSearcher{}
	t.Cleanup(func() {
		loadSchemaOperationsForSearch = originalLoader
		schemaOperationSearcher = originalSearcher
	})

	command := newCmdAPISchemaSearch(testCommandContext(io))
	command.SetIO(io)
	r.Equal(0, command.Run([]string{"wrokspaec"}))

	output := io.Output.String()
	r.Contains(output, `No API operations matched "wrokspaec"`)
	r.Empty(io.Error.String())
}

func TestSchemaOperationDocumentDereferencesRefs(t *testing.T) {
	t.Parallel()

	spec := map[string]any{
		"openapi": "3.0.0",
		"paths": map[string]any{
			"/workspaces/{workspace_id}/vars": map[string]any{
				"post": map[string]any{
					"operationId": "createWorkspaceVar",
					"summary":     "Create a Variable",
					"requestBody": map[string]any{
						"$ref": "#/components/requestBodies/CreateWorkspaceVar",
					},
					"responses": map[string]any{
						"200": map[string]any{
							"content": map[string]any{
								"application/vnd.api+json": map[string]any{
									"schema": map[string]any{
										"$ref": "#/components/schemas/WorkspaceVarCreate",
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]any{
			"requestBodies": map[string]any{
				"CreateWorkspaceVar": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/vnd.api+json": map[string]any{
							"schema": map[string]any{
								"$ref": "#/components/schemas/WorkspaceVarCreate",
							},
						},
					},
				},
			},
			"schemas": map[string]any{
				"WorkspaceVarCreate": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data": map[string]any{
							"type": "object",
						},
					},
				},
			},
		},
	}

	doc, err := schemaOperationDocument(spec, "createWorkspaceVar")
	if err != nil {
		t.Fatal(err)
	}

	paths := doc["paths"].(map[string]any)
	pathItem := paths["/workspaces/{workspace_id}/vars"].(map[string]any)
	post := pathItem["post"].(map[string]any)
	requestBody := post["requestBody"].(map[string]any)
	if requestBody["required"] != true {
		t.Fatalf("expected dereferenced requestBody, got %#v", requestBody)
	}
	content := requestBody["content"].(map[string]any)
	schema := content["application/vnd.api+json"].(map[string]any)["schema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("expected dereferenced schema, got %#v", schema)
	}

	responses := post["responses"].(map[string]any)
	responseSchema := responses["200"].(map[string]any)["content"].(map[string]any)["application/vnd.api+json"].(map[string]any)["schema"].(map[string]any)
	if responseSchema["$ref"] != "#/components/schemas/WorkspaceVarCreate" {
		t.Fatalf("expected response schema ref to remain unresolved, got %#v", responseSchema)
	}
}

func TestCmdAPISchemaGetRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	originalLoader := loadSchemaDocumentForGet
	loadSchemaDocumentForGet = func() (map[string]any, error) {
		return map[string]any{
			"openapi": "3.0.0",
			"paths": map[string]any{
				"/workspaces/{workspace_id}": map[string]any{
					"get": map[string]any{
						"operationId": "getWorkspace",
						"summary":     "Get Workspace",
					},
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		loadSchemaDocumentForGet = originalLoader
	})

	command := newCmdAPISchemaGet(testCommandContext(io))
	command.SetIO(io)
	r.Equal(0, command.Run([]string{"getWorkspace"}))

	output := io.Output.String()
	r.Contains(output, `"operationId": "getWorkspace"`)
	r.Contains(output, `"/workspaces/{workspace_id}"`)
	r.Empty(io.Error.String())
}

func testCommandContext(io *iostreams.Testing) *cmd.Context {
	return &cmd.Context{
		IO: io,
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
