// Copyright IBM Corp. 2026

package get

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestRunGet(t *testing.T) {
	t.Parallel()

	t.Run("list workspaces with org", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{
			"GET /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, _ *http.Request) {
				jsonapi(w, map[string]any{
					"data": []any{
						map[string]any{
							"id":   "ws-1",
							"type": "workspaces",
							"attributes": map[string]any{
								"name": "alpha",
							},
						},
					},
				})
			},
		}))
		ctx.Profile.Organization = "my-org"

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"workspaces"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "alpha")
	})

	t.Run("list workspaces without org errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"workspaces"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization is required but not set")
	})

	t.Run("list runs not supported at top level", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"runs"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "listing runs is not supported at the top level")
	})

	t.Run("unknown resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"unknown"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type or ID")
		assert.Contains(t, err.Error(), "Available resources:")
	})

	t.Run("get workspace by ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{
			"GET /api/v2/workspaces/ws-abc123": func(w http.ResponseWriter, _ *http.Request) {
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-abc123",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "my-workspace",
						},
					},
				})
			},
		}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"ws-abc123"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "my-workspace")
	})

	t.Run("get run by ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{
			"GET /api/v2/runs/run-xyz": func(w http.ResponseWriter, _ *http.Request) {
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id":   "run-xyz",
						"type": "runs",
						"attributes": map[string]any{
							"status": "applied",
						},
					},
				})
			},
		}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"run-xyz"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "applied")
	})

	t.Run("unknown ID prefix", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"xyz-123"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type or ID")
	})

	t.Run("two args type and ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{
			"GET /api/v2/workspaces/ws-abc": func(w http.ResponseWriter, _ *http.Request) {
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-abc",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "fetched-ws",
						},
					},
				})
			},
		}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"workspace", "ws-abc"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "fetched-ws")
	})

	t.Run("two args unknown type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{"blah", "ws-123"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
	})

	t.Run("no args returns usage", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{}))

		err := runGet(ctx, &GetOpts{}, hclog.NewNullLogger(), []string{})
		require.ErrorIs(t, err, cmd.ErrDisplayUsage)
	})

	t.Run("list workspaces with --all paginates", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var serverURL string
		ctx := testContext(t, io, testServer(t, routeMap{
			"GET /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				// Only serve page 1 when no page param is set.
				if r.URL.Query().Get("page[number]") == "2" {
					jsonapi(w, map[string]any{
						"data": []any{
							map[string]any{
								"id":   "ws-2",
								"type": "workspaces",
								"attributes": map[string]any{
									"name": "workspace-two",
								},
							},
						},
						"links": map[string]any{"next": nil},
						"meta": map[string]any{
							"pagination": map[string]any{"total-count": 2},
						},
					})
					return
				}
				jsonapi(w, map[string]any{
					"data": []any{
						map[string]any{
							"id":   "ws-1",
							"type": "workspaces",
							"attributes": map[string]any{
								"name": "workspace-one",
							},
						},
					},
					"links": map[string]any{"next": serverURL + "/api/v2/organizations/my-org/workspaces?page%5Bnumber%5D=2"},
					"meta": map[string]any{
						"pagination": map[string]any{"total-count": 2},
					},
				})
			},
		}))
		serverURL = ctx.Profile.Hostname
		ctx.Profile.Organization = "my-org"

		err := runGet(ctx, &GetOpts{All: true}, hclog.NewNullLogger(), []string{"workspaces"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "workspace-one")
		assert.Contains(t, io.Output.String(), "workspace-two")
	})

	t.Run("explicit organization flag overrides profile", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := testContext(t, io, testServer(t, routeMap{
			"GET /api/v2/organizations/flag-org/workspaces": func(w http.ResponseWriter, _ *http.Request) {
				jsonapi(w, map[string]any{
					"data": []any{
						map[string]any{
							"id":   "ws-2",
							"type": "workspaces",
							"attributes": map[string]any{
								"name": "from-flag-org",
							},
						},
					},
				})
			},
		}))
		ctx.Profile.Organization = "profile-org"

		err := runGet(ctx, &GetOpts{Organization: "flag-org"}, hclog.NewNullLogger(), []string{"workspaces"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "from-flag-org")
	})
}

// --- test helpers ---

type routeMap map[string]http.HandlerFunc

func (rm routeMap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try exact match with query params first.
	keyWithQuery := fmt.Sprintf("%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
	if h, ok := rm[keyWithQuery]; ok {
		h(w, r)
		return
	}
	// Fall back to path-only match.
	key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	if h, ok := rm[key]; ok {
		h(w, r)
		return
	}
	http.Error(w, "unexpected: "+key, http.StatusInternalServerError)
}

func testServer(t *testing.T, routes routeMap) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(routes)
	t.Cleanup(server.Close)
	return server
}

func testContext(t *testing.T, io *iostreams.Testing, server *httptest.Server) *cmd.Context {
	t.Helper()
	p := profile.TestProfile(t)
	p.Hostname = server.URL
	return &cmd.Context{
		IO:          io,
		Output:      format.New(io),
		ShutdownCtx: context.Background(),
		Profile:     p,
	}
}

func jsonapi(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	_ = json.NewEncoder(w).Encode(payload)
}
