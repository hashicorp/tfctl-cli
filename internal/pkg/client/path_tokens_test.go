package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePathTokens_NoTokens(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathTokens(context.Background(), "/workspaces", PathTokenResolutionOpts{}, nil)
	require.NoError(t, err)
	assert.Equal(t, "/workspaces", result)
}

func TestResolvePathTokens_OrganizationDirectSubstitution(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathTokens(context.Background(), "/organizations/{organization}/workspaces", PathTokenResolutionOpts{
		Organization: "my-org",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "/organizations/my-org/workspaces", result)
}

func TestResolvePathTokens_OrganizationNameToken(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathTokens(context.Background(), "/organizations/{organization_name}/workspaces", PathTokenResolutionOpts{
		Organization: "my-org",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "/organizations/my-org/workspaces", result)
}

func TestResolvePathTokens_OrganizationFromExplicitFlag(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathTokens(context.Background(), "/organizations/{organization}/teams", PathTokenResolutionOpts{
		PathTokens:   map[string]string{"organization": "flag-org"},
		Organization: "profile-org",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "/organizations/flag-org/teams", result)
}

func TestResolvePathTokens_OrganizationMissing(t *testing.T) {
	t.Parallel()
	_, err := ResolvePathTokens(context.Background(), "/organizations/{organization}/workspaces", PathTokenResolutionOpts{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no organization configured")
}

func TestResolvePathTokens_WorkspaceResolution(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/organizations/my-org/workspaces/my-ws" && r.Method == "GET" {
			writeJSONAPI(w, map[string]any{
				"data": map[string]any{
					"id":   "ws-abc123",
					"type": "workspaces",
					"attributes": map[string]any{
						"name": "my-ws",
					},
				},
			})
			return
		}
		http.Error(w, "unexpected: "+r.URL.Path, 500)
	}))
	defer server.Close()

	apiClient, err := New(server.URL, "test-token", nil)
	require.NoError(t, err)
	resolver := NewResolver(apiClient, false, false)

	result, err := ResolvePathTokens(context.Background(), "/workspaces/{workspace}/vars", PathTokenResolutionOpts{
		PathTokens:   map[string]string{"workspace": "my-ws"},
		Organization: "my-org",
	}, resolver)
	require.NoError(t, err)
	assert.Equal(t, "/workspaces/ws-abc123/vars", result)
}

func TestResolvePathTokens_WorkspaceAutoResolveFromConfig(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/organizations/my-org/workspaces/config-ws" && r.Method == "GET" {
			writeJSONAPI(w, map[string]any{
				"data": map[string]any{
					"id":   "ws-fromcfg",
					"type": "workspaces",
					"attributes": map[string]any{
						"name": "config-ws",
					},
				},
			})
			return
		}
		http.Error(w, "unexpected: "+r.URL.Path, 500)
	}))
	defer server.Close()

	apiClient, err := New(server.URL, "test-token", nil)
	require.NoError(t, err)
	resolver := NewResolver(apiClient, false, false)

	result, err := ResolvePathTokens(context.Background(), "/workspaces/{workspace_id}/runs", PathTokenResolutionOpts{
		Organization: "my-org",
		Workspace:    "config-ws",
	}, resolver)
	require.NoError(t, err)
	assert.Equal(t, "/workspaces/ws-fromcfg/runs", result)
}

func TestResolvePathTokens_WorkspaceMissingOrganization(t *testing.T) {
	t.Parallel()
	_, err := ResolvePathTokens(context.Background(), "/workspaces/{workspace}/vars", PathTokenResolutionOpts{
		PathTokens: map[string]string{"workspace": "my-ws"},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "organization is required")
}

func TestResolvePathTokens_TeamResolution(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/organizations/my-org/teams" && r.Method == "GET" {
			writeJSONAPI(w, map[string]any{
				"data": []any{
					map[string]any{
						"id":   "team-owners123",
						"type": "teams",
						"attributes": map[string]any{
							"name": "owners",
						},
					},
				},
			})
			return
		}
		http.Error(w, "unexpected: "+r.URL.Path, 500)
	}))
	defer server.Close()

	apiClient, err := New(server.URL, "test-token", nil)
	require.NoError(t, err)
	resolver := NewResolver(apiClient, false, false)

	result, err := ResolvePathTokens(context.Background(), "/teams/{team_id}", PathTokenResolutionOpts{
		PathTokens:   map[string]string{"team_id": "owners"},
		Organization: "my-org",
	}, resolver)
	require.NoError(t, err)
	assert.Equal(t, "/teams/team-owners123", result)
}

func TestResolvePathTokens_ProjectResolution(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/organizations/my-org/projects" && r.Method == "GET" {
			writeJSONAPI(w, map[string]any{
				"data": []any{
					map[string]any{
						"id":   "prj-def456",
						"type": "projects",
						"attributes": map[string]any{
							"name": "my-project",
						},
					},
				},
			})
			return
		}
		http.Error(w, "unexpected: "+r.URL.Path, 500)
	}))
	defer server.Close()

	apiClient, err := New(server.URL, "test-token", nil)
	require.NoError(t, err)
	resolver := NewResolver(apiClient, false, false)

	result, err := ResolvePathTokens(context.Background(), "/projects/{project_id}/varsets", PathTokenResolutionOpts{
		PathTokens:   map[string]string{"project_id": "my-project"},
		Organization: "my-org",
	}, resolver)
	require.NoError(t, err)
	assert.Equal(t, "/projects/prj-def456/varsets", result)
}

func TestResolvePathTokens_VarsetResolution(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/organizations/my-org/varsets" && r.Method == "GET" {
			writeJSONAPI(w, map[string]any{
				"data": []any{
					map[string]any{
						"id":   "varset-ghi789",
						"type": "varsets",
						"attributes": map[string]any{
							"name": "my-varset",
						},
					},
				},
			})
			return
		}
		http.Error(w, "unexpected: "+r.URL.Path, 500)
	}))
	defer server.Close()

	apiClient, err := New(server.URL, "test-token", nil)
	require.NoError(t, err)
	resolver := NewResolver(apiClient, false, false)

	result, err := ResolvePathTokens(context.Background(), "/varsets/{varset_id}/relationships/vars", PathTokenResolutionOpts{
		PathTokens:   map[string]string{"varset_id": "my-varset"},
		Organization: "my-org",
	}, resolver)
	require.NoError(t, err)
	assert.Equal(t, "/varsets/varset-ghi789/relationships/vars", result)
}

func TestResolvePathTokens_MultipleTokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/organizations/my-org/workspaces/ws-one" && r.Method == "GET" {
			writeJSONAPI(w, map[string]any{
				"data": map[string]any{
					"id":   "ws-resolved1",
					"type": "workspaces",
					"attributes": map[string]any{
						"name": "ws-one",
					},
				},
			})
			return
		}
		http.Error(w, "unexpected: "+r.URL.Path, 500)
	}))
	defer server.Close()

	apiClient, err := New(server.URL, "test-token", nil)
	require.NoError(t, err)
	resolver := NewResolver(apiClient, false, false)

	result, err := ResolvePathTokens(context.Background(), "/organizations/{organization_name}/workspaces/{workspace_name}", PathTokenResolutionOpts{
		PathTokens:   map[string]string{"workspace_name": "ws-one"},
		Organization: "my-org",
	}, resolver)
	require.NoError(t, err)
	// organization_name substitutes the name directly, workspace_name resolves to ID
	assert.Equal(t, "/organizations/my-org/workspaces/ws-resolved1", result)
}

func TestResolvePathTokens_UnknownTokenWithExplicitValue(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathTokens(context.Background(), "/some-resource/{custom_id}/details", PathTokenResolutionOpts{
		PathTokens: map[string]string{"custom_id": "literal-value"},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "/some-resource/literal-value/details", result)
}

func TestResolvePathTokens_UnknownTokenWithoutValue(t *testing.T) {
	t.Parallel()
	_, err := ResolvePathTokens(context.Background(), "/some-resource/{unknown_thing}/details", PathTokenResolutionOpts{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognized path token")
	assert.Contains(t, err.Error(), "unknown_thing")
}

func TestResolvePathTokens_StackNotRecognized(t *testing.T) {
	t.Parallel()
	result, err := ResolvePathTokens(context.Background(), "/stacks/{stack_id}", PathTokenResolutionOpts{
		PathTokens:   map[string]string{"stack_id": "my-stack"},
		Organization: "my-org",
	}, nil)
	// stack_id is not a recognized resource type, so explicit value passes through as-is.
	require.NoError(t, err)
	assert.Equal(t, "/stacks/my-stack", result)
}

func writeJSONAPI(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}
