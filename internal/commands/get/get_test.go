// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package get

import (
	"net/http"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdtest"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/resource"
)

func TestRunGet(t *testing.T) {
	t.Parallel()

	t.Run("list workspaces with org", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
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

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"workspaces"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "alpha")
	})

	t.Run("list workspaces without org errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"workspaces"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization is required but not set")
	})

	t.Run("list runs not supported at top level", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"runs"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "listing is not supported for runs")
	})

	t.Run("unknown resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"unknown"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
		assert.Contains(t, err.Error(), "Available resources:")
	})

	t.Run("get workspace by ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/workspaces/ws-abc123": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
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

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"ws-abc123"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "my-workspace")
	})

	t.Run("get run by ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/runs/run-xyz": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
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

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"run-xyz"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "applied")
	})

	t.Run("unknown ID prefix", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"xyz-123"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized ID prefix")
	})

	t.Run("two args type and ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/workspaces/ws-abc": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
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

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"workspace", "ws-abc"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "fetched-ws")
	})

	t.Run("two args unknown type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"blah", "ws-123"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
	})

	t.Run("two args mismatched ID prefix", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{"workspace", "run-xyz"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `does not look like a workspace resource`)
		assert.Contains(t, err.Error(), `expected prefix "ws-"`)
	})

	t.Run("no args returns usage", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runGet(ctx, &Opts{}, hclog.NewNullLogger(), []string{})
		require.ErrorIs(t, err, cmd.ErrDisplayUsage)
	})

	t.Run("list workspaces with --all paginates", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var serverURL string
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				// Only serve page 1 when no page param is set.
				if r.URL.Query().Get("page[number]") == "2" {
					cmdtest.WriteJSONAPI(w, map[string]any{
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
				cmdtest.WriteJSONAPI(w, map[string]any{
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

		err := runGet(ctx, &Opts{All: true}, hclog.NewNullLogger(), []string{"workspaces"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "workspace-one")
		assert.Contains(t, io.Output.String(), "workspace-two")
	})

	t.Run("explicit organization flag overrides profile", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/organizations/flag-org/workspaces": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
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

		err := runGet(ctx, &Opts{Organization: "flag-org"}, hclog.NewNullLogger(), []string{"workspaces"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "from-flag-org")
	})
}

// TestRunGetByID_NoPathGet covers the defensive guard in runGetByID for a
// resource with no PathGet (currently unreachable from the registry, but
// protects against future additions).
func TestRunGetByID_NoPathGet(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()
	ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

	synthetic := &resource.Resource{
		Type:     "widgets",
		IDPrefix: "wgt-",
	}

	err := runGetByID(ctx, &Opts{}, hclog.NewNullLogger(), synthetic, "wgt-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get is not supported for widgets")
}

// TestNewCmdGet_ArgValidation exercises the command through cmd.Command.Run to
// ensure the framework's arg-count validation (derived from PositionalArguments)
// allows 1 and 2 args but rejects 0 and 3.
func TestNewCmdGet_ArgValidation(t *testing.T) {
	t.Parallel()

	t.Run("two args accepted by framework", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/workspaces/ws-abc": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
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

		c := NewCmdGet(ctx)
		root := &cmd.Command{Name: "tfctl"}
		cmd.ConfigureRootCommand(ctx, root)
		root.AddChild(c)

		exitCode := c.Run([]string{"workspace", "ws-abc"}, ctx)
		assert.Equal(t, 0, exitCode)
		assert.Contains(t, io.Output.String(), "fetched-ws")
	})

	t.Run("one arg accepted by framework", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"GET /api/v2/workspaces/ws-one": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-one",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "single-arg",
						},
					},
				})
			},
		}))

		c := NewCmdGet(ctx)
		root := &cmd.Command{Name: "tfctl"}
		cmd.ConfigureRootCommand(ctx, root)
		root.AddChild(c)

		exitCode := c.Run([]string{"ws-one"}, ctx)
		assert.Equal(t, 0, exitCode)
		assert.Contains(t, io.Output.String(), "single-arg")
	})

	t.Run("three args rejected by framework", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		c := NewCmdGet(ctx)
		root := &cmd.Command{Name: "tfctl"}
		cmd.ConfigureRootCommand(ctx, root)
		root.AddChild(c)

		exitCode := c.Run([]string{"workspace", "ws-abc", "extra"}, ctx)
		assert.NotEqual(t, 0, exitCode)
		assert.Contains(t, io.Error.String(), "accepts between 1 and 2 arg(s)")
	})
}
