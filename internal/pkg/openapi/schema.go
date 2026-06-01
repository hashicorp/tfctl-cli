// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package openapi loads and provides access to the embedded OpenAPI specification for API v2.
package openapi

import (
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

var (
	schemaRefRe                    = regexp.MustCompile(`"\$ref"\s*:\s*"#/components/schemas/([^"]+)"`)
	openAPISpecFile profile.FileID = "openapi.json"
)

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

// LoadEmbeddedSchema loads the embedded OpenAPI specification and returns a Schema interface for
// accessing it. This function panics if the embedded spec cannot be loaded, which should never
// happen and indicates a bug in the code.
func LoadEmbeddedSchema() Schema {
	result, err := NewFromData(embeddedOpenAPISpec)
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded OpenAPI spec: %v (this is always a bug)", err))
	}
	return result
}

// SchemaFactory attempts to fetch the OpenAPI specification from the profile host
// and returns a Schema interface for accessing it. If any step of this process fails, it falls back
// to loading the embedded specification and an error is logged. The result is cached for the
// duration of the process run to avoid repeated fetch attempts.
func SchemaFactory(cmdCtx *cmd.Context, logger hclog.Logger) Schema {
	// Per run, attempt to fetch the hosted OpenAPI spec and update the cache if it's newer than the
	// cached version. If any step of this process fails, fall back to the embedded version. It's
	// critical that this process set cachedSchema or panic.
	schemaOnce.Do(func() {
		if cmdCtx == nil || cmdCtx.Profile == nil {
			cachedSchema = LoadEmbeddedSchema()
			return
		}

		p := cmdCtx.Profile
		api, err := cmdCtx.NewAPIClient(logger)
		api.Adapter.Client.Timeout = 2 * time.Second // Don't wait too long for the API in case it's unresponsive

		if err != nil {
			logger.Error("Failed to create API client for OpenAPI schema loading, falling back to embedded version", "error", err)
			cachedSchema = LoadEmbeddedSchema()
			return
		}

		loader, err := p.HostCache()
		if err != nil {
			logger.Error("Failed to get host cache for OpenAPI schema loading, falling back to embedded version", "error", err)
			cachedSchema = LoadEmbeddedSchema()
			return
		}

		data, err := loader.ReadOrRefresh(openAPISpecFile, func(mTime *time.Time) ([]byte, error) {
			// This function should return nil data if the cached version is still fresh,
			// or new data if the cache is outdated. Any error will be treated as a fetch failure.
			data, err := api.TFE.Meta.OpenAPI.Read(cmdCtx.ShutdownCtx, true, mTime)
			if err != nil {
				// Logged below
				return nil, errors.New("failed to read openapi spec from server")
			}
			return data, nil
		})

		if err != nil {
			logger.Error("Failed to fetch hosted OpenAPI spec, falling back to embedded version", "error", err)
			cachedSchema = LoadEmbeddedSchema()
			return
		}

		cachedSchema, err = NewFromData(data)
		if err != nil {
			logger.Error("Failed to parse OpenAPI spec from server, falling back to embedded version", "error", err)
			cachedSchema = LoadEmbeddedSchema()
			return
		}
	})

	return cachedSchema
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
