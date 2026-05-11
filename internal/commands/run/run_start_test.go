// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestRunStart(t *testing.T) {
	t.Parallel()

	t.Run("dry run with workspace ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/workspaces/ws-abc123", r.URL.Path)
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{
						"name": "foobar",
					},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{
								"id": "org-resolved", "type": "organizations",
							},
						},
					},
				},
			})
		}))

		err := runStart(context.Background(), StartOpts{
			IO:        io,
			Profile:   profile.TestProfile(t),
			Workspace: "ws-abc123",
			DryRun:    true,
			APIClient: c,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "would create run for workspace ID ws-abc123")
	})

	t.Run("dry run with workspace name resolves ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/organizations/my-org/workspaces/my-workspace", r.URL.Path)
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "my-workspace"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{
								"id": "org-resolved", "type": "organizations",
							},
						},
					},
				},
			})
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "my-workspace",
			Organization: "my-org",
			DryRun:       true,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "would create run for workspace ID ws-resolved")
	})

	t.Run("workspace name resolution failure", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "no-such-ws",
			Organization: "my-org",
			DryRun:       false,
		}, CreateOpts{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolving workspace")
	})

	t.Run("successful start with workspace ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/workspaces/ws-abc123":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "foobar"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-new123", "type": "runs",
						"attributes": map[string]any{"status": "pending"},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:        io,
			APIClient: c,
			Profile:   profile.TestProfile(t),
			Workspace: "ws-abc123",
			DryRun:    false,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "run-new123")
		assert.Contains(t, io.Error.String(), "created")
	})

	t.Run("successful start with workspace name", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/organizations/my-org/workspaces/my-ws":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "my-ws"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-fromname", "type": "runs",
						"attributes": map[string]any{"status": "pending"},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "my-ws",
			Organization: "my-org",
			DryRun:       false,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "run-fromname")
	})

	t.Run("successful start with create options", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/organizations/my-org/workspaces/my-ws":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "my-ws"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				data := body["data"].(map[string]any)
				attrs := data["attributes"].(map[string]any)
				assert.Equal(t, true, attrs["debugging-mode"])
				assert.Equal(t, "test start", attrs["message"])
				assert.Equal(t, true, attrs["allow-empty-apply"])

				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-fromname", "type": "runs",
						"attributes": map[string]any{"status": "pending"},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "my-ws",
			Organization: "my-org",
			DryRun:       false,
		}, CreateOpts{
			DebuggingMode:   true,
			Message:         "test start",
			AllowEmptyApply: true,
		})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "run-fromname")
	})

	t.Run("API error on run creation", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/workspaces/ws-abc123":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "foobar"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				http.Error(w, "server error", http.StatusInternalServerError)
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:        io,
			APIClient: c,
			Profile:   profile.TestProfile(t),
			Workspace: "ws-abc123",
			DryRun:    false,
		}, CreateOpts{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start run")
	})
}
