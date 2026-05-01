// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package openapi loads and provides access to the embedded OpenAPI specification for API v2.
package openapi

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
)

//go:embed spec/hcpt_v2_public_beta.json
var embeddedOpenAPISpec []byte

// Schema provides access to the embedded OpenAPI specification for API v2.
type Schema interface {
	Paths() *openapi3.Paths
	OperationByID(operationID string) (*openapi3.Operation, error)
}

type wrapper struct {
	T *openapi3.T
}

var (
	cachedSchema Schema
	schemaOnce   sync.Once
	schemaErr    error
)

func (w *wrapper) Paths() *openapi3.Paths {
	return w.T.Paths
}

func (w *wrapper) OperationByID(operationID string) (*openapi3.Operation, error) {
	for _, path := range w.T.Paths.Keys() {
		pathItem := w.T.Paths.Value(path)
		for _, operation := range pathItem.Operations() {
			if operation.OperationID == operationID {
				return operation, nil
			}
		}
	}
	return nil, fmt.Errorf("operation with ID %q not found", operationID)
}

// SchemaFactory loads the embedded OpenAPI specification and returns a Schema interface for
// accessing it. The schema is loaded once and cached for future calls. Any error encountered
// during loading is also cached and returned on subsequent calls.
func SchemaFactory(_ *cmd.Context) (Schema, error) {
	schemaOnce.Do(func() {
		loader := openapi3.NewLoader()
		root, schemaErr := loader.LoadFromData(embeddedOpenAPISpec)
		if schemaErr == nil {
			cachedSchema = &wrapper{T: root}
		}
	})

	return cachedSchema, schemaErr
}
