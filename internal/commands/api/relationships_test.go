// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/openapi"
)

const relationshipTestSchema = `{
  "openapi": "3.0.0",
  "info": {"title": "relationship test", "version": "1"},
  "paths": {
    "/custom-resources": {
      "post": {
        "requestBody": {
          "content": {
            "application/vnd.api+json": {
              "schema": {
                "type": "object",
                "properties": {
                  "data": {
                    "type": "object",
                    "properties": {
                      "relationships": {
                        "type": "object",
                        "properties": {
                          "owner": {
                            "type": "object",
                            "properties": {
                              "data": {
                                "type": "object",
                                "properties": {
                                  "type": {"type": "string", "enum": ["custom-owners"]},
                                  "id": {"type": "string"}
                                }
                              }
                            }
                          }
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        },
        "responses": {"200": {"description": "OK"}}
      }
    }
  }
}`

func TestMatchTemplatePath(t *testing.T) {
	t.Parallel()

	oas := openapi.LoadEmbeddedSchema()

	require.Equal(t,
		"/organizations/{organization_name}/workspaces",
		matchTemplatePath(oas, "/organizations/acme/workspaces"),
	)
	// A placeholder segment matches any concrete value.
	require.Equal(t,
		"/organizations/{organization_name}",
		matchTemplatePath(oas, "/organizations/acme"),
	)
	// A path the spec does not describe.
	require.Equal(t, "", matchTemplatePath(oas, "/nope/not/a/real/path"))
}

func TestRelationshipLinkages_FromEmbeddedSchema(t *testing.T) {
	t.Parallel()

	linkages, ok := relationshipLinkages(openapi.LoadEmbeddedSchema(), http.MethodPost, "/organizations/acme/workspaces")
	require.True(t, ok)

	// To-one linkage: the key differs from the type, which is exactly why we
	// read the type from the schema rather than the flag key.
	require.Equal(t, linkage{Type: "projects", Types: []string{"projects"}, ToMany: false}, linkages["project"])
	require.Equal(t, linkage{Type: "agent-pools", Types: []string{"agent-pools"}, ToMany: false}, linkages["agent-pool"])

	// To-many linkage.
	require.Equal(t, linkage{Type: "workspace-outputs", Types: []string{"workspace-outputs"}, ToMany: true}, linkages["outputs"])

	// Ambiguous (type enum has several members, e.g. users|teams|runs): kept
	// with an empty Type so the caller requires an explicit type, but its
	// candidates are preserved to name them in the error.
	lockedBy, ok := linkages["locked-by"]
	require.True(t, ok, "ambiguous relationship should still be reported")
	require.Empty(t, lockedBy.Type, "ambiguous relationship has no single pinned type")
	require.Greater(t, len(lockedBy.Types), 1, "ambiguous relationship lists its candidates")

	// Links-only relationships (no data linkage) are omitted.
	_, linksOnly := linkages["remote-state-consumers"]
	require.False(t, linksOnly, "links-only relationship should be omitted")
}

func TestRelationshipLinkages_UsesHTTPMethod(t *testing.T) {
	t.Parallel()

	linkages, ok := relationshipLinkages(openapi.LoadEmbeddedSchema(), http.MethodPatch, "/workspaces/ws-1")
	require.True(t, ok)
	require.Equal(t, linkage{Type: "projects", Types: []string{"projects"}, ToMany: false}, linkages["project"])

	linkages, ok = relationshipLinkages(openapi.LoadEmbeddedSchema(), http.MethodPut, "/organizations/acme")
	require.True(t, ok)
	require.Equal(t, linkage{Type: "projects", Types: []string{"projects"}, ToMany: false}, linkages["default-project"])

	_, ok = relationshipLinkages(openapi.LoadEmbeddedSchema(), http.MethodPost, "/workspaces/ws-1")
	require.False(t, ok)
}

func TestRelationshipLinkages_NoSchemaOrNoMatch(t *testing.T) {
	t.Parallel()

	_, ok := relationshipLinkages(nil, http.MethodPost, "/organizations/acme/workspaces")
	require.False(t, ok)

	_, ok = relationshipLinkages(openapi.LoadEmbeddedSchema(), http.MethodPost, "/nope/not/real")
	require.False(t, ok)
}

func TestBuildRelationships(t *testing.T) {
	t.Parallel()

	linkages := map[string]linkage{
		"project": {Type: "projects", ToMany: false},
		"outputs": {Type: "workspace-outputs", ToMany: true},
	}

	t.Run("schema-inferred to-one", func(t *testing.T) {
		t.Parallel()
		got, err := buildRelationships(map[string]string{"project": "prj-1"}, linkages, true)
		require.NoError(t, err)
		require.Equal(t, map[string]any{
			"project": map[string]any{"data": map[string]any{"type": "projects", "id": "prj-1"}},
		}, got)
	})

	t.Run("schema-inferred to-many, comma-separated ids", func(t *testing.T) {
		t.Parallel()
		got, err := buildRelationships(map[string]string{"outputs": "wsout-1, wsout-2"}, linkages, true)
		require.NoError(t, err)
		require.Equal(t, map[string]any{
			"outputs": map[string]any{"data": []any{
				map[string]any{"type": "workspace-outputs", "id": "wsout-1"},
				map[string]any{"type": "workspace-outputs", "id": "wsout-2"},
			}},
		}, got)
	})

	t.Run("explicit name:type=id override", func(t *testing.T) {
		t.Parallel()
		// "locked-by" is ambiguous in the schema, so the user pins the type.
		got, err := buildRelationships(map[string]string{"locked-by:users": "user-1"}, linkages, true)
		require.NoError(t, err)
		require.Equal(t, map[string]any{
			"locked-by": map[string]any{"data": map[string]any{"type": "users", "id": "user-1"}},
		}, got)
	})

	t.Run("ambiguous relationship names its candidate types", func(t *testing.T) {
		t.Parallel()
		ambiguous := map[string]linkage{
			"locked-by": {Types: []string{"users", "teams", "runs"}, ToMany: false},
		}
		_, err := buildRelationships(map[string]string{"locked-by": "user-1"}, ambiguous, true)
		require.Error(t, err)
		// Distinct from the "unknown relationship" message: it names the types
		// and points at the explicit override.
		assert.Contains(t, err.Error(), "maps to multiple types")
		assert.Contains(t, err.Error(), "runs, teams, users") // sorted
		assert.Contains(t, err.Error(), "locked-by:<type>=<id>")
	})

	t.Run("same relationship specified twice errors", func(t *testing.T) {
		t.Parallel()
		_, err := buildRelationships(
			map[string]string{"project": "prj-1", "project:projects": "prj-2"},
			linkages, true,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `relationship "project" specified more than once`)
	})

	t.Run("unknown relationship with schema lists valid names", func(t *testing.T) {
		t.Parallel()
		_, err := buildRelationships(map[string]string{"projects": "prj-1"}, linkages, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown relationship "projects"`)
		assert.Contains(t, err.Error(), "valid relationships:")
		assert.Contains(t, err.Error(), "project")
	})

	t.Run("unknown relationship without schema advises explicit type", func(t *testing.T) {
		t.Parallel()
		_, err := buildRelationships(map[string]string{"whatever": "x-1"}, nil, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not infer the resource type")
		assert.Contains(t, err.Error(), "whatever:<type>=<id>")
	})

	t.Run("to-one with multiple ids errors", func(t *testing.T) {
		t.Parallel()
		_, err := buildRelationships(map[string]string{"project": "prj-1,prj-2"}, linkages, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is to-one but got 2 ids")
	})

	t.Run("empty id errors", func(t *testing.T) {
		t.Parallel()
		_, err := buildRelationships(map[string]string{"project": "  "}, linkages, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "has no id")
	})

	t.Run("unknown relationship with explicit type and multiple ids infers to-many", func(t *testing.T) {
		t.Parallel()
		got, err := buildRelationships(map[string]string{"widgets:widgets": "w-1,w-2"}, nil, false)
		require.NoError(t, err)
		require.Equal(t, map[string]any{
			"widgets": map[string]any{"data": []any{
				map[string]any{"type": "widgets", "id": "w-1"},
				map[string]any{"type": "widgets", "id": "w-2"},
			}},
		}, got)
	})
}

// TestRunAPI_RelationshipInfersTypeAndPost exercises the full path: -r implies
// POST, and the linkage type is read from the embedded schema for the matched
// operation.
func TestRunAPI_RelationshipInfersTypeAndPost(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/organizations/acme/workspaces": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{"id": "ws-1", "type": "workspaces"},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := RunAPI(context.Background(), newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/organizations/acme/workspaces")
		opts.Attributes = map[string]string{"name": "foo"}
		opts.Relationships = map[string]string{"project": "prj-12dff4673ab9"}
	}))
	require.NoError(t, err)

	require.Equal(t, "POST", recorder.Last().Method)
	assertJSONBodyEqual(t, map[string]any{
		"data": map[string]any{
			"type":       "workspaces",
			"attributes": map[string]any{"name": "foo"},
			"relationships": map[string]any{
				"project": map[string]any{
					"data": map[string]any{"type": "projects", "id": "prj-12dff4673ab9"},
				},
			},
		},
	}, recorder.Last().JSONBody(t))
}

// TestRunAPI_RelationshipOnlyBody confirms a relationship-only request still
// produces a valid data envelope (no attributes key).
func TestRunAPI_RelationshipOnlyBody(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/organizations/acme/workspaces": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{"id": "ws-1", "type": "workspaces"},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := RunAPI(context.Background(), newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/organizations/acme/workspaces")
		opts.Relationships = map[string]string{"project": "prj-1"}
	}))
	require.NoError(t, err)

	body := recorder.Last().JSONBody(t)
	data := nestedMap(t, body, "data")
	_, hasAttrs := data["attributes"]
	require.False(t, hasAttrs, "no attributes key expected for relationship-only body")
	require.Contains(t, data, "relationships")
}

func TestRunAPI_RelationshipInfersTypeForPatch(t *testing.T) {
	t.Parallel()

	server, recorder := newAPITestServer(map[string]http.HandlerFunc{
		"PATCH /api/v2/workspaces/ws-1": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": map[string]any{"id": "ws-1", "type": "workspaces"},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	err := RunAPI(context.Background(), newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-1")
		opts.Method = http.MethodPatch
		opts.Relationships = map[string]string{"project": "prj-1"}
	}))
	require.NoError(t, err)

	require.Equal(t, http.MethodPatch, recorder.Last().Method)
	assertJSONBodyEqual(t, map[string]any{
		"data": map[string]any{
			"type": "workspaces",
			"relationships": map[string]any{
				"project": map[string]any{
					"data": map[string]any{"type": "projects", "id": "prj-1"},
				},
			},
		},
	}, recorder.Last().JSONBody(t))
}

func TestRunAPI_RelationshipSchemaSelection(t *testing.T) {
	t.Parallel()

	t.Run("uses injected schema", func(t *testing.T) {
		t.Parallel()

		schema, err := openapi.NewFromData([]byte(relationshipTestSchema))
		require.NoError(t, err)
		server, recorder := newAPITestServer(map[string]http.HandlerFunc{
			"POST /api/v2/custom-resources": func(w http.ResponseWriter, _ *http.Request) {
				writeJSONAPIResponse(w, http.StatusOK, map[string]any{
					"data": map[string]any{"id": "custom-1", "type": "custom-resources"},
				})
			},
		})
		defer server.Close()

		err = RunAPI(context.Background(), newTestOpts(t, server.URL, iostreams.Test(), func(opts *Opts) {
			opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/custom-resources")
			opts.Schema = schema
			opts.Relationships = map[string]string{"owner": "owner-1"}
		}))
		require.NoError(t, err)

		data := nestedMap(t, recorder.Last().JSONBody(t), "data")
		relationships := nestedMap(t, data, "relationships")
		owner := nestedMap(t, relationships, "owner")
		require.Equal(t, map[string]any{"type": "custom-owners", "id": "owner-1"}, owner["data"])
	})

	t.Run("allows explicit type without endpoint schema", func(t *testing.T) {
		t.Parallel()

		server, recorder := newAPITestServer(map[string]http.HandlerFunc{
			"POST /api/v2/custom-resources": func(w http.ResponseWriter, _ *http.Request) {
				writeJSONAPIResponse(w, http.StatusOK, map[string]any{
					"data": map[string]any{"id": "custom-1", "type": "custom-resources"},
				})
			},
		})
		defer server.Close()

		err := RunAPI(context.Background(), newTestOpts(t, server.URL, iostreams.Test(), func(opts *Opts) {
			opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/custom-resources")
			opts.Relationships = map[string]string{"owner:custom-owners": "owner-1"}
		}))
		require.NoError(t, err)

		data := nestedMap(t, recorder.Last().JSONBody(t), "data")
		relationships := nestedMap(t, data, "relationships")
		owner := nestedMap(t, relationships, "owner")
		require.Equal(t, map[string]any{"type": "custom-owners", "id": "owner-1"}, owner["data"])
	})
}
