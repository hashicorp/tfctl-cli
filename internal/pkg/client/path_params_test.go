// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePathParams_NoParams(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathParams("/workspaces", nil)
	require.NoError(t, err)
	assert.Equal(t, "/workspaces", result)
}

func TestResolvePathParams_SingleParam(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathParams("/workspaces/{workspace_id}/runs", map[string]string{
		"workspace_id": "ws-abc123",
	})
	require.NoError(t, err)
	assert.Equal(t, "/workspaces/ws-abc123/runs", result)
}

func TestResolvePathParams_MultipleParams(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathParams("/organizations/{organization_name}/workspaces/{workspace_name}", map[string]string{
		"organization_name": "my-org",
		"workspace_name":    "my-ws",
	})
	require.NoError(t, err)
	assert.Equal(t, "/organizations/my-org/workspaces/my-ws", result)
}

func TestResolvePathParams_UnresolvedParam(t *testing.T) {
	t.Parallel()
	_, err := ResolvePathParams("/workspaces/{workspace_id}/runs", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
	assert.Contains(t, err.Error(), "-p")
}

func TestResolvePathParams_PartialResolution(t *testing.T) {
	t.Parallel()
	_, err := ResolvePathParams("/organizations/{org}/workspaces/{ws}", map[string]string{
		"org": "my-org",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ws")
}

func TestResolvePathParams_NoBraces(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathParams("/account/details", map[string]string{"foo": "bar"})
	require.NoError(t, err)
	assert.Equal(t, "/account/details", result)
}

func TestResolvePathParams_RepeatedParam(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathParams("/workspaces/{id}/varsets/{id}", map[string]string{
		"id": "ws-123",
	})
	require.NoError(t, err)
	assert.Equal(t, "/workspaces/ws-123/varsets/ws-123", result)
}

func TestParsePathParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		path     string
		expected map[string]string
	}{
		{
			name:     "single param",
			path:     "/workspaces/{workspace_id}/runs",
			expected: map[string]string{"workspace_id": "workspaces"},
		},
		{
			name:     "multiple params",
			path:     "/organizations/{org_name}/workspaces/{workspace_id}",
			expected: map[string]string{"org_name": "organizations", "workspace_id": "workspaces"},
		},
		{
			name:     "no params",
			path:     "/account/details",
			expected: map[string]string{},
		},
		{
			name:     "param at root",
			path:     "/{resource_id}",
			expected: map[string]string{"resource_id": ""},
		},
		{
			name:     "nested path",
			path:     "/varsets/{varset_id}/relationships/vars",
			expected: map[string]string{"varset_id": "varsets"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := ParsePathParams(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsResolvableSegment(t *testing.T) {
	t.Parallel()
	assert.True(t, IsResolvableSegment("workspaces"))
	assert.True(t, IsResolvableSegment("teams"))
	assert.True(t, IsResolvableSegment("projects"))
	assert.True(t, IsResolvableSegment("varsets"))
	assert.False(t, IsResolvableSegment("runs"))
	assert.False(t, IsResolvableSegment("organizations"))
	assert.False(t, IsResolvableSegment(""))
}

func TestLooksLikeID(t *testing.T) {
	t.Parallel()
	assert.True(t, LooksLikeID("workspaces", "ws-abc123"))
	assert.True(t, LooksLikeID("teams", "team-abc123"))
	assert.True(t, LooksLikeID("projects", "prj-abc123"))
	assert.True(t, LooksLikeID("varsets", "varset-abc123"))

	assert.False(t, LooksLikeID("workspaces", "my-workspace"))
	assert.False(t, LooksLikeID("teams", "owners"))
	assert.False(t, LooksLikeID("projects", "default"))
	assert.False(t, LooksLikeID("varsets", "my-varset"))
	assert.False(t, LooksLikeID("runs", "run-abc123"))
}
