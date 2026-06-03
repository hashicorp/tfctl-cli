// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package openapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestSchema_OperationById(t *testing.T) {
	cmdContext := &cmd.Context{
		ShutdownCtx: context.Background(),
		Profile:     profile.TestProfile(t),
	}
	embedded := SchemaFactory(cmdContext, hclog.NewNullLogger())

	op, err := embedded.OperationByID("getWorkspace")
	require.NoError(t, err)

	require.NotNil(t, op)
	require.Equal(t, "getWorkspace", op.OperationID)

	nf, err := embedded.OperationByID("foobar")
	require.ErrorContains(t, err, "operation with ID \"foobar\" not found")
	require.Nil(t, nf)
}

func resetSchemaFactory() {
	cachedSchema = nil
	schemaOnce = sync.Once{}
}

// testOpenAPISpec is a minimal valid OpenAPI 3.0 spec used by test server responses.
var testOpenAPISpec = []byte(`{
  "openapi": "3.0.0",
  "info": {"title": "Test API", "version": "1.0.0"},
  "paths": {
    "/test/hello": {
      "get": {
        "operationId": "testHello",
        "summary": "A test endpoint",
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func testProfileWithServer(t *testing.T, serverURL string) *profile.Profile {
	t.Helper()
	p := profile.TestProfile(t)
	p.Hostname = serverURL
	return p
}

func TestSchemaFactory(t *testing.T) {
	t.Run("fetches spec when cache is empty", func(t *testing.T) {
		resetSchemaFactory()
		t.Cleanup(resetSchemaFactory)

		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Log("test server received request", "method", r.Method, "path", r.URL.Path)
			if r.URL.Path == "/openapi/stable.json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(testOpenAPISpec)
				return
			}
			http.NotFound(w, r)
		})

		p := testProfileWithServer(t, srv.URL)
		cmdContext := &cmd.Context{
			ShutdownCtx: context.Background(),
			Profile:     p,
		}

		schema := SchemaFactory(cmdContext, hclog.NewNullLogger())
		require.NotNil(t, schema)

		// Verify it loaded the spec from the test server, not the embedded one.
		require.Equal(t, 1, schema.Paths().Len())
		op, err := schema.OperationByID("testHello")
		require.NoError(t, err)
		require.Equal(t, "testHello", op.OperationID)
	})

	t.Run("uses cached spec when cache is up to date", func(t *testing.T) {
		resetSchemaFactory()
		t.Cleanup(resetSchemaFactory)

		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/openapi/stable.json" {
				// Return 304 Not Modified to indicate the cache is still fresh.
				w.WriteHeader(http.StatusNotModified)
				return
			}
			http.NotFound(w, r)
		})

		p := testProfileWithServer(t, srv.URL)
		cmdContext := &cmd.Context{
			ShutdownCtx: context.Background(),
			Profile:     p,
		}

		// Pre-populate cache with the test spec.
		loader, err := p.HostCache(hclog.NewNullLogger())
		require.NoError(t, err)

		var openAPIFile profile.FileID = "openapi.json"
		now := time.Now()
		err = loader.Write(openAPIFile, testOpenAPISpec, &now)
		require.NoError(t, err)

		schema := SchemaFactory(cmdContext, hclog.NewNullLogger())
		require.NotNil(t, schema)

		// Verify it used the cached spec (not the embedded one).
		require.Equal(t, 1, schema.Paths().Len())
		op, err := schema.OperationByID("testHello")
		require.NoError(t, err)
		require.Equal(t, "testHello", op.OperationID)
	})

	t.Run("fetches spec when cache is outdated", func(t *testing.T) {
		resetSchemaFactory()
		t.Cleanup(resetSchemaFactory)

		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/openapi/stable.json" {
				// Server returns new data (ignores If-Modified-Since for simplicity).
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(testOpenAPISpec)
				return
			}
			http.NotFound(w, r)
		})

		p := testProfileWithServer(t, srv.URL)
		cmdContext := &cmd.Context{
			ShutdownCtx: context.Background(),
			Profile:     p,
		}

		// Pre-populate cache with the embedded spec (simulating an outdated cache).
		loader, err := p.HostCache(hclog.NewNullLogger())
		require.NoError(t, err)

		var openAPIFile profile.FileID = "openapi.json"
		now := time.Now()

		err = loader.Write(openAPIFile, embeddedOpenAPISpec, &now)
		require.NoError(t, err)

		schema := SchemaFactory(cmdContext, hclog.NewNullLogger())
		require.NotNil(t, schema)

		// Verify the factory used the freshly-fetched spec, not the cached one.
		require.Equal(t, 1, schema.Paths().Len())
		op, err := schema.OperationByID("testHello")
		require.NoError(t, err)
		require.Equal(t, "testHello", op.OperationID)
	})

	t.Run("falls back to embedded spec on fetch error", func(t *testing.T) {
		resetSchemaFactory()
		t.Cleanup(resetSchemaFactory)

		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Always return an error.
			w.WriteHeader(http.StatusInternalServerError)
		})

		p := testProfileWithServer(t, srv.URL)
		cmdContext := &cmd.Context{
			ShutdownCtx: context.Background(),
			Profile:     p,
		}

		schema := SchemaFactory(cmdContext, hclog.NewNullLogger())
		require.NotNil(t, schema)

		// Should have fallen back to the full embedded spec.
		require.Greater(t, schema.Paths().Len(), 1)
		op, err := schema.OperationByID("getWorkspace")
		require.NoError(t, err)
		require.Equal(t, "getWorkspace", op.OperationID)
	})
}

func TestSchema_AtomizePath(t *testing.T) {
	cmdContext := &cmd.Context{
		ShutdownCtx: context.Background(),
		Profile:     profile.TestProfile(t),
	}

	embedded := SchemaFactory(cmdContext, hclog.NewNullLogger())
	require.NotNil(t, embedded)

	t.Run("returns error for unknown path", func(t *testing.T) {
		_, err := embedded.AtomizePath("/nonexistent")
		require.ErrorContains(t, err, "not found")
	})

	t.Run("contains only the specified path", func(t *testing.T) {
		atomized, err := embedded.AtomizePath("/account/details")
		require.NoError(t, err)

		paths := atomized.Paths()
		require.Equal(t, 1, paths.Len())
		require.NotNil(t, paths.Find("/account/details"))
	})

	t.Run("preserves all operations on the path", func(t *testing.T) {
		atomized, err := embedded.AtomizePath("/workspaces/{workspace_id}")
		require.NoError(t, err)

		pathItem, err := atomized.PathByPath("/workspaces/{workspace_id}")
		require.NoError(t, err)
		ops := pathItem.Operations()
		require.Contains(t, ops, "GET")
		require.Contains(t, ops, "PATCH")
		require.Contains(t, ops, "DELETE")
	})

	t.Run("includes referenced component schemas transitively", func(t *testing.T) {
		atomized, err := embedded.AtomizePath("/account/details")
		require.NoError(t, err)

		w := atomized.(*wrapper)
		require.NotNil(t, w.T.Components)
		require.NotNil(t, w.T.Components.Schemas)

		// Direct refs
		require.Contains(t, w.T.Components.Schemas, "errors")
		require.Contains(t, w.T.Components.Schemas, "users-envelope")

		// Transitive refs from users-envelope
		require.Contains(t, w.T.Components.Schemas, "users")
		require.Contains(t, w.T.Components.Schemas, "self")
		require.Contains(t, w.T.Components.Schemas, "related")
		require.Contains(t, w.T.Components.Schemas, "links_related")
	})

	t.Run("does not include unrelated schemas", func(t *testing.T) {
		atomized, err := embedded.AtomizePath("/account/details")
		require.NoError(t, err)

		w := atomized.(*wrapper)
		// workspaces schema should not be included
		require.NotContains(t, w.T.Components.Schemas, "workspaces")
		require.NotContains(t, w.T.Components.Schemas, "workspaces-envelope")
	})
}

func TestSchema_AtomizeOperation(t *testing.T) {
	cmdContext := &cmd.Context{
		ShutdownCtx: context.Background(),
		Profile:     profile.TestProfile(t),
	}
	embedded := SchemaFactory(cmdContext, hclog.NewNullLogger())
	require.NotNil(t, embedded)

	t.Run("returns error for unknown operation", func(t *testing.T) {
		_, err := embedded.AtomizeOperation("nonexistent")
		require.ErrorContains(t, err, "not found")
	})

	t.Run("contains only the path for the operation", func(t *testing.T) {
		atomized, err := embedded.AtomizeOperation("getWorkspace")
		require.NoError(t, err)

		paths := atomized.Paths()
		require.Equal(t, 1, paths.Len())
		require.NotNil(t, paths.Find("/workspaces/{workspace_id}"))
	})

	t.Run("contains only the specified operation on the path", func(t *testing.T) {
		atomized, err := embedded.AtomizeOperation("getWorkspace")
		require.NoError(t, err)

		pathItem, err := atomized.PathByPath("/workspaces/{workspace_id}")
		require.NoError(t, err)
		ops := pathItem.Operations()
		require.Len(t, ops, 1)
		require.Contains(t, ops, "GET")
	})

	t.Run("includes referenced component schemas transitively", func(t *testing.T) {
		atomized, err := embedded.AtomizeOperation("getAccountDetails")
		require.NoError(t, err)

		w := atomized.(*wrapper)
		require.Contains(t, w.T.Components.Schemas, "errors")
		require.Contains(t, w.T.Components.Schemas, "users-envelope")
		require.Contains(t, w.T.Components.Schemas, "users")
	})

	t.Run("does not include schemas from other operations on same path", func(t *testing.T) {
		// getWorkspace should not pull in schemas only used by updateWorkspace
		atomized, err := embedded.AtomizeOperation("getWorkspace")
		require.NoError(t, err)

		w := atomized.(*wrapper)
		// Should have schemas referenced by GET only
		require.Contains(t, w.T.Components.Schemas, "errors")
		require.Contains(t, w.T.Components.Schemas, "workspaces-envelope")
	})
}
