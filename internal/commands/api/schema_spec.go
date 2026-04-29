package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	_ "embed"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
)

type openAPISpec struct {
	Paths map[string]json.RawMessage `json:"paths"`
}

type openAPIOperation struct {
	OperationID string   `json:"operationId"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

//go:embed openapi/spec.json
var embeddedOpenAPISpec []byte

var (
	schemaOperationsOnce  sync.Once
	schemaOperationsCache []schemaOperation
	schemaOperationsErr   error
	schemaDocumentOnce    sync.Once
	schemaDocumentCache   map[string]any
	schemaDocumentErr     error
)

func cachedSchemaOperations(ctx *cmd.Context) ([]schemaOperation, error) {
	schemaOperationsOnce.Do(func() {
		data, source, err := loadSchemaSpecBytes(ctx)
		if err != nil {
			schemaOperationsErr = err
			return
		}
		schemaOperationsCache, schemaOperationsErr = loadSchemaOperationsFromBytes(data, source)
	})
	return schemaOperationsCache, schemaOperationsErr
}

func cachedSchemaDocument(ctx *cmd.Context) (map[string]any, error) {
	schemaDocumentOnce.Do(func() {
		data, source, err := loadSchemaSpecBytes(ctx)
		if err != nil {
			schemaDocumentErr = err
			return
		}
		schemaDocumentCache, schemaDocumentErr = loadSchemaDocumentFromBytes(data, source)
	})
	return schemaDocumentCache, schemaDocumentErr
}

func loadSchemaSpecBytes(ctx *cmd.Context) ([]byte, string, error) {
	// Always use the embedded fallback until the hosted OpenAPI endpoint is
	// confirmed and deployed consistently on the platform.
	_ = ctx
	//
	// When the platform endpoint is known and deployed consistently, restore the
	// hosted-first path here:
	//
	// if hosted, err := fetchHostedOpenAPISpec(ctx); err == nil {
	// 	return hosted, "from host", nil
	// }
	if len(embeddedOpenAPISpec) == 0 {
		return nil, "", fmt.Errorf("embedded OpenAPI spec is empty")
	}
	return embeddedOpenAPISpec, "from embedded fallback", nil
}

// Future hosted fetch path, intentionally disabled until the real endpoint is
// confirmed and deployed consistently on the platform.
//
// func fetchHostedOpenAPISpec(ctx *cmd.Context) ([]byte, error) {
// 	if ctx == nil || ctx.APIClient == nil || ctx.APIClient.BaseURL == nil {
// 		return nil, fmt.Errorf("hosted OpenAPI spec unavailable without API client")
// 	}
//
// 	requestURL, err := client.ResolveURL(ctx.APIClient.BaseURL, "/api/v2/openapi.json")
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	resp, err := ctx.APIClient.RawRequest(schemaSearchContext(ctx), &client.Request{
// 		Method: http.MethodGet,
// 		URL:    requestURL,
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
// 		return nil, fmt.Errorf("hosted OpenAPI spec unavailable: %s", resp.Status)
// 	}
// 	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
// 		return nil, fmt.Errorf("fetch hosted OpenAPI spec: %s", resp.Status)
// 	}
// 	return resp.Body, nil
// }

func loadSchemaDocumentFromBytes(data []byte, source string) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse OpenAPI spec %s: %w", source, err)
	}

	return document, nil
}

func loadSchemaOperationsFromBytes(data []byte, source string) ([]schemaOperation, error) {
	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse OpenAPI spec %s: %w", source, err)
	}

	operations := make([]schemaOperation, 0, len(spec.Paths))
	for path, rawPathItem := range spec.Paths {
		var methods map[string]json.RawMessage
		if err := json.Unmarshal(rawPathItem, &methods); err != nil {
			return nil, fmt.Errorf("parse OpenAPI path item %q: %w", path, err)
		}

		for method, rawOperation := range methods {
			if !isHTTPMethod(method) {
				continue
			}

			var operation openAPIOperation
			if err := json.Unmarshal(rawOperation, &operation); err != nil {
				return nil, fmt.Errorf("parse OpenAPI operation %s %s: %w", strings.ToUpper(method), path, err)
			}
			if operation.OperationID == "" {
				continue
			}

			summary := strings.TrimSpace(operation.Summary)
			if summary == "" {
				summary = strings.TrimSpace(operation.Description)
			}

			operations = append(operations, schemaOperation{
				OperationID: operation.OperationID,
				Method:      strings.ToUpper(method),
				Path:        path,
				Tags:        operation.Tags,
				Summary:     summary,
			})
		}
	}

	sort.Slice(operations, func(i, j int) bool {
		if operations[i].OperationID != operations[j].OperationID {
			return operations[i].OperationID < operations[j].OperationID
		}
		if operations[i].Method != operations[j].Method {
			return operations[i].Method < operations[j].Method
		}
		return operations[i].Path < operations[j].Path
	})

	if len(operations) == 0 {
		return nil, fmt.Errorf("no API operations found in OpenAPI spec %s", source)
	}

	return operations, nil
}

func schemaOperationDocument(spec map[string]any, operationID string) (map[string]any, error) {
	pathValue, ok := spec["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenAPI spec does not contain a valid paths object")
	}

	path, method, _, err := findSchemaOperation(pathValue, operationID)
	if err != nil {
		return nil, err
	}

	doc, err := schemaPathDocument(spec, path)
	if err != nil {
		return nil, err
	}

	paths := doc["paths"].(map[string]any)
	pathItem := paths[path].(map[string]any)
	for key := range pathItem {
		if isHTTPMethod(key) && strings.ToLower(key) != strings.ToLower(method) {
			delete(pathItem, key)
		}
	}

	return doc, nil
}

func schemaPathDocument(spec map[string]any, path string) (map[string]any, error) {
	pathsValue, ok := spec["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenAPI spec does not contain a valid paths object")
	}

	rawPathItem, ok := pathsValue[path]
	if !ok {
		return nil, fmt.Errorf("path %q not found", path)
	}

	methods, ok := rawPathItem.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid path item for %q", path)
	}

	pathItem := make(map[string]any)

	if params, ok := methods["parameters"]; ok {
		pathItem["parameters"] = dereferenceOpenAPIValue(params, spec, map[string]bool{})
	}

	for method, rawOperation := range methods {
		if !isHTTPMethod(method) {
			continue
		}
		operation, ok := rawOperation.(map[string]any)
		if !ok {
			continue
		}
		pathItem[strings.ToLower(method)] = dereferenceSchemaOperation(operation, spec, map[string]bool{})
	}

	result := map[string]any{
		"paths": map[string]any{
			path: pathItem,
		},
	}
	if version, ok := spec["openapi"]; ok {
		result["openapi"] = version
	}
	return result, nil
}

func dereferenceSchemaOperation(operation map[string]any, root map[string]any, refsInProgress map[string]bool) map[string]any {
	resolved := make(map[string]any, len(operation))
	for key, value := range operation {
		if key == "responses" {
			resolved[key] = cloneValue(value)
			continue
		}
		resolved[key] = dereferenceOpenAPIValue(value, root, refsInProgress)
	}
	return resolved
}

func findSchemaOperation(paths map[string]any, operationID string) (string, string, map[string]any, error) {
	var caseInsensitiveMatch struct {
		path      string
		method    string
		operation map[string]any
		found     bool
	}

	for path, rawPathItem := range paths {
		methods, ok := rawPathItem.(map[string]any)
		if !ok {
			continue
		}
		for method, rawOperation := range methods {
			if !isHTTPMethod(method) {
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			currentID, _ := operation["operationId"].(string)
			if currentID == operationID {
				return path, strings.ToUpper(method), operation, nil
			}
			if strings.EqualFold(currentID, operationID) {
				if caseInsensitiveMatch.found {
					return "", "", nil, fmt.Errorf("multiple operations match %q; use exact operationId", operationID)
				}
				caseInsensitiveMatch = struct {
					path      string
					method    string
					operation map[string]any
					found     bool
				}{path: path, method: strings.ToUpper(method), operation: operation, found: true}
			}
		}
	}

	if caseInsensitiveMatch.found {
		return caseInsensitiveMatch.path, caseInsensitiveMatch.method, caseInsensitiveMatch.operation, nil
	}

	return "", "", nil, fmt.Errorf("operation %q not found", operationID)
}

func dereferenceOpenAPIValue(value any, root map[string]any, refsInProgress map[string]bool) any {
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
			if refsInProgress[ref] {
				return cloneMap(typed)
			}
			resolvedValue, err := resolveJSONPointer(root, ref)
			if err == nil {
				refsInProgress[ref] = true
				resolved := dereferenceOpenAPIValue(resolvedValue, root, refsInProgress)
				delete(refsInProgress, ref)
				if resolvedMap, ok := resolved.(map[string]any); ok {
					merged := cloneMap(resolvedMap)
					for key, item := range typed {
						if key == "$ref" {
							continue
						}
						merged[key] = dereferenceOpenAPIValue(item, root, refsInProgress)
					}
					return merged
				}
				if len(typed) == 1 {
					return resolved
				}
			}
		}

		resolved := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved[key] = dereferenceOpenAPIValue(item, root, refsInProgress)
		}
		return resolved
	case []any:
		resolved := make([]any, 0, len(typed))
		for _, item := range typed {
			resolved = append(resolved, dereferenceOpenAPIValue(item, root, refsInProgress))
		}
		return resolved
	default:
		return value
	}
}

func resolveJSONPointer(root map[string]any, pointer string) (any, error) {
	if pointer == "#" {
		return root, nil
	}
	if !strings.HasPrefix(pointer, "#/") {
		return nil, fmt.Errorf("unsupported ref %q", pointer)
	}

	var current any = root
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		next, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid ref %q", pointer)
		}
		current, ok = next[part]
		if !ok {
			return nil, fmt.Errorf("ref %q not found", pointer)
		}
	}

	return current, nil
}

func cloneMap(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneValue(item))
		}
		return cloned
	default:
		return value
	}
}

func isHTTPMethod(method string) bool {
	switch strings.ToLower(method) {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}
