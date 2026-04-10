package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
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

var (
	schemaOperationsOnce  sync.Once
	schemaOperationsCache []schemaOperation
	schemaOperationsErr   error
	schemaDocumentOnce    sync.Once
	schemaDocumentCache   map[string]any
	schemaDocumentErr     error
)

func cachedSchemaOperations() ([]schemaOperation, error) {
	schemaOperationsOnce.Do(func() {
		specPath, err := defaultOpenAPISpecPath()
		if err != nil {
			schemaOperationsErr = err
			return
		}
		schemaOperationsCache, schemaOperationsErr = loadSchemaOperationsFromSpec(specPath)
	})
	return schemaOperationsCache, schemaOperationsErr
}

func cachedSchemaDocument() (map[string]any, error) {
	schemaDocumentOnce.Do(func() {
		specPath, err := defaultOpenAPISpecPath()
		if err != nil {
			schemaDocumentErr = err
			return
		}
		schemaDocumentCache, schemaDocumentErr = loadSchemaDocumentFromSpec(specPath)
	})
	return schemaDocumentCache, schemaDocumentErr
}

func loadSchemaDocumentFromSpec(specPath string) (map[string]any, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("read OpenAPI spec %q: %w", specPath, err)
	}

	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse OpenAPI spec %q: %w", specPath, err)
	}

	return document, nil
}

func loadSchemaOperationsFromSpec(specPath string) ([]schemaOperation, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("read OpenAPI spec %q: %w", specPath, err)
	}

	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse OpenAPI spec %q: %w", specPath, err)
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
		return nil, fmt.Errorf("no API operations found in OpenAPI spec %q", specPath)
	}

	return operations, nil
}

func defaultOpenAPISpecPath() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve schema command source path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	goModPath := filepath.Join(repoRoot, "go.mod")
	candidates := []string{filepath.Join(repoRoot, "openapi", "spec.json")}

	if replaceTarget, err := goModReplaceTarget(goModPath, "github.com/hashicorp/go-tfe"); err == nil && replaceTarget != "" {
		if !filepath.IsAbs(replaceTarget) {
			replaceTarget = filepath.Join(repoRoot, replaceTarget)
		}
		candidates = append(candidates, filepath.Join(replaceTarget, "openapi", "spec.json"))
	}

	candidates = append(candidates,
		filepath.Join(repoRoot, "..", "go-tfe", "openapi", "spec.json"),
		filepath.Join(filepath.Dir(repoRoot), "go-tfe", "openapi", "spec.json"),
		"/Users/shweta.murali/go-tfe/openapi/spec.json",
	)

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not locate an OpenAPI spec; checked %s", strings.Join(candidates, ", "))
}

func schemaOperationDocument(spec map[string]any, operationID string) (map[string]any, error) {
	pathValue, ok := spec["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenAPI spec does not contain a valid paths object")
	}

	path, method, operation, err := findSchemaOperation(pathValue, operationID)
	if err != nil {
		return nil, err
	}

	resolved := dereferenceSchemaOperation(operation, spec, map[string]bool{})
	result := map[string]any{
		"paths": map[string]any{
			path: map[string]any{
				strings.ToLower(method): resolved,
			},
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

func goModReplaceTarget(goModPath, modulePath string) (string, error) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "replace ") || !strings.Contains(line, modulePath) || !strings.Contains(line, "=>") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "=>" {
				return parts[i+1], nil
			}
		}
	}

	return "", fmt.Errorf("replace target for %s not found", modulePath)
}

func isHTTPMethod(method string) bool {
	switch strings.ToLower(method) {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}
