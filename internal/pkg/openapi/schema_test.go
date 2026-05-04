// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package openapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchema_OperationById(t *testing.T) {
	embedded, err := SchemaFactory(nil)
	require.NoError(t, err)

	op, err := embedded.OperationByID("getWorkspace")
	require.NoError(t, err)

	require.NotNil(t, op)
	require.Equal(t, "getWorkspace", op.OperationID)

	nf, err := embedded.OperationByID("foobar")
	require.ErrorContains(t, err, "operation with ID \"foobar\" not found")
	require.Nil(t, nf)
}

func TestSchema_AtomizePath(t *testing.T) {
	embedded, err := SchemaFactory(nil)
	require.NoError(t, err)

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
	embedded, err := SchemaFactory(nil)
	require.NoError(t, err)

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
