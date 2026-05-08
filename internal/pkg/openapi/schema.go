// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package openapi loads and provides access to the embedded OpenAPI specification for API v2.
package openapi

import (
	_ "embed"
	"fmt"
	"regexp"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
)

var schemaRefRe = regexp.MustCompile(`"\$ref"\s*:\s*"#/components/schemas/([^"]+)"`)

// collectSchemaRefs recursively collects all component schema names referenced
// from the given JSON-serialized value, including transitive references.
func collectSchemaRefs(data []byte, allSchemas openapi3.Schemas) map[string]struct{} {
	collected := make(map[string]struct{})

	queue := schemaRefRe.FindAllSubmatch(data, -1)
	for len(queue) > 0 {
		match := queue[0]
		queue = queue[1:]
		name := string(match[1])
		if _, seen := collected[name]; seen {
			continue
		}
		collected[name] = struct{}{}
		if ref, ok := allSchemas[name]; ok {
			nested, _ := ref.MarshalJSON()
			queue = append(queue, schemaRefRe.FindAllSubmatch(nested, -1)...)
		}
	}
	return collected
}

//go:embed spec/hcpt_v2_public_beta.json
var embeddedOpenAPISpec []byte

// Operation represents an API operation from the OpenAPI specification, including its path
// and method.
type Operation struct {
	openapi3.Operation
	Path   string
	Method string
}

// Schema provides access to the embedded OpenAPI specification for API v2.
type Schema interface {
	Paths() *openapi3.Paths
	Operations() []*Operation
	OperationByID(operationID string) (*Operation, error)
	PathByPath(path string) (*openapi3.PathItem, error)
	AtomizePath(path string) (Schema, error)
	AtomizeOperation(operationID string) (Schema, error)
	MarshalJSON() ([]byte, error)
}

type wrapper struct {
	T *openapi3.T
}

func (w *wrapper) MarshalJSON() ([]byte, error) {
	return w.T.MarshalJSON()
}

func (w *wrapper) PathByPath(path string) (*openapi3.PathItem, error) {
	if pathItem := w.T.Paths.Find(path); pathItem != nil {
		return pathItem, nil
	}
	return nil, fmt.Errorf("path %q not found", path)
}

var (
	cachedSchema Schema
	schemaOnce   sync.Once
	schemaErr    error
)

func (w *wrapper) Operations() []*Operation {
	result := make([]*Operation, 0, w.T.Paths.Len()*4)
	for _, path := range w.T.Paths.Keys() {
		pathItem := w.T.Paths.Value(path)
		for method, op := range pathItem.Operations() {
			result = append(result, &Operation{
				Operation: *op,
				Path:      path,
				Method:    method,
			})
		}
	}
	return result
}

func (w *wrapper) Paths() *openapi3.Paths {
	return w.T.Paths
}

func (w *wrapper) OperationByID(operationID string) (*Operation, error) {
	for _, path := range w.T.Paths.Keys() {
		pathItem := w.T.Paths.Value(path)
		for method, operation := range pathItem.Operations() {
			if operation.OperationID == operationID {
				return &Operation{
					Operation: *operation,
					Path:      path,
					Method:    method,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("operation with ID %q not found", operationID)
}

// NewFromData loads the OpenAPI specification from the given JSON data and returns a Schema
// interface for accessing it.
func NewFromData(data []byte) (Schema, error) {
	loader := openapi3.NewLoader()
	root, err := loader.LoadFromData(data)
	if err != nil {
		return nil, err
	}
	return &wrapper{T: root}, nil
}

// SchemaFactory loads the embedded OpenAPI specification and returns a Schema interface for
// accessing it. The schema is loaded once and cached for future calls. Any error encountered
// during loading is also cached and returned on subsequent calls.
func SchemaFactory(_ *cmd.Context) (Schema, error) {
	schemaOnce.Do(func() {
		cachedSchema, schemaErr = NewFromData(embeddedOpenAPISpec)
	})

	return cachedSchema, schemaErr
}

// AtomizePath returns a new schema containing only the components necessary to understand the
// specified path and its operations.
func (w *wrapper) AtomizePath(path string) (Schema, error) {
	pathItem := w.T.Paths.Find(path)
	if pathItem == nil {
		return nil, fmt.Errorf("path %q not found", path)
	}

	paths := openapi3.NewPaths()
	paths.Set(path, pathItem)

	pathsJSON, err := paths.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal path item: %w", err)
	}

	refs := collectSchemaRefs(pathsJSON, w.T.Components.Schemas)
	schemas := make(openapi3.Schemas, len(refs))
	for name := range refs {
		schemas[name] = w.T.Components.Schemas[name]
	}

	return &wrapper{T: &openapi3.T{
		OpenAPI: w.T.OpenAPI,
		Info:    w.T.Info,
		Paths:   paths,
		Components: &openapi3.Components{
			Schemas: schemas,
		},
	}}, nil
}

// AtomizeOperation returns a copy of the schema containing only the components necessary to understand the
// specified operation and its path.
func (w *wrapper) AtomizeOperation(operationID string) (Schema, error) {
	for _, path := range w.T.Paths.Keys() {
		pathItem := w.T.Paths.Value(path)
		for method, op := range pathItem.Operations() {
			if op.OperationID == operationID {
				// Build a path item with only this operation
				singleOpItem := &openapi3.PathItem{}
				switch method {
				case "GET":
					singleOpItem.Get = op
				case "PUT":
					singleOpItem.Put = op
				case "POST":
					singleOpItem.Post = op
				case "DELETE":
					singleOpItem.Delete = op
				case "PATCH":
					singleOpItem.Patch = op
				case "HEAD":
					singleOpItem.Head = op
				case "OPTIONS":
					singleOpItem.Options = op
				case "TRACE":
					singleOpItem.Trace = op
				}
				singleOpItem.Parameters = pathItem.Parameters

				paths := openapi3.NewPaths()
				paths.Set(path, singleOpItem)

				pathsJSON, err := paths.MarshalJSON()
				if err != nil {
					return nil, fmt.Errorf("marshal paths: %w", err)
				}

				refs := collectSchemaRefs(pathsJSON, w.T.Components.Schemas)
				schemas := make(openapi3.Schemas, len(refs))
				for name := range refs {
					schemas[name] = w.T.Components.Schemas[name]
				}

				return &wrapper{T: &openapi3.T{
					OpenAPI: w.T.OpenAPI,
					Info:    w.T.Info,
					Paths:   paths,
					Components: &openapi3.Components{
						Schemas: schemas,
					},
				}}, nil
			}
		}
	}
	return nil, fmt.Errorf("operation with ID %q not found", operationID)
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
