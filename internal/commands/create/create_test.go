// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package create

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdtest"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestRunCreate(t *testing.T) {
	t.Parallel()

	t.Run("create workspace with attributes", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var receivedBody map[string]any
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"POST /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				cmdtest.WriteJSONAPI(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-new123",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "foo",
						},
					},
				})
			},
		}))
		inv.Profile.DefaultOrganization = "my-org"

		opts := &Opts{Args: []string{"workspace"}, ProfileOrganization: "my-org"}
		opts.Attributes = map[string]string{"name": "foo"}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runCreate(inv.ShutdownCtx, opts)
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-new123")

		// Verify request body structure
		data := receivedBody["data"].(map[string]any)
		assert.Equal(t, "workspaces", data["type"])
		attrs := data["attributes"].(map[string]any)
		assert.Equal(t, "foo", attrs["name"])
	})

	t.Run("create workspace with input body", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var receivedBody map[string]any
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"POST /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				cmdtest.WriteJSONAPI(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-input123",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "from-input",
						},
					},
				})
			},
		}))

		opts := &Opts{
			Args:                []string{"workspace"},
			ProfileOrganization: "my-org",
		}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client
		opts.InputRequest = `{"data":{"type":"workspaces","attributes":{"name":"from-input"}}}`

		err = runCreate(inv.ShutdownCtx, opts)
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-input123")
	})

	t.Run("create workspace dry run", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{
			ProfileOrganization: "my-org",
			Args:                []string{"workspace"},
		}
		opts.Attributes = map[string]string{"name": "foo"}
		opts.DryRun = true
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runCreate(inv.ShutdownCtx, opts)
		require.NoError(t, err)
		// Dry run should show the request info on stderr
		assert.Contains(t, io.Error.String(), "POST")
	})

	t.Run("create with no attrs and no input errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))
		opts := &Opts{ProfileOrganization: "my-org", Args: []string{"workspace"}}
		opts.IO = io
		opts.Output = inv.Output

		err := runCreate(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provide attributes with -a key=value or a request body with -i")
	})

	t.Run("create unsupported resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{
			Args: []string{"apply"},
		}
		opts.Attributes = map[string]string{"name": "foo"}

		err := runCreate(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create is not supported for applies")
	})

	t.Run("create unknown resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{
			Args: []string{"blah"},
		}
		opts.IO = io
		opts.Output = inv.Output
		opts.Attributes = map[string]string{"x": "y"}

		err := runCreate(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
		assert.Contains(t, err.Error(), "Available resources:")
	})

	t.Run("create workspace without org errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{}
		opts.IO = io
		opts.Output = inv.Output
		opts.Attributes = map[string]string{"name": "foo"}
		opts.Args = []string{"workspace"}

		err := runCreate(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization is required but not set")
	})

	t.Run("no args returns usage", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runCreate(inv.ShutdownCtx, &Opts{})
		require.ErrorIs(t, err, cmd.ErrDisplayUsage)
	})

	t.Run("both attributes and input body errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{
			ProfileOrganization: "my-org",
			Args:                []string{"workspace"},
		}
		opts.Attributes = map[string]string{"name": "foo"}
		opts.InputRequest = `{"data":{"type":"workspaces"}}`

		err := runCreate(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use both -a (attributes) and -i (input body)")
	})

	t.Run("explicit org flag overrides profile", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"POST /api/v2/organizations/flag-org/workspaces": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-flagorg",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "created-in-flag-org",
						},
					},
				})
			},
		}))

		client, err := inv.NewAPIClient()
		require.NoError(t, err)

		opts := &Opts{
			ProfileOrganization: "profile-org",
			Organization:        "flag-org",
			Args:                []string{"workspace"},
			client:              client,
		}

		opts.IO = io
		opts.Output = inv.Output
		opts.Attributes = map[string]string{"name": "created-in-flag-org"}

		err = runCreate(inv.ShutdownCtx, opts)
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-flagorg")
	})

	t.Run("create workspace with stdin input", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var receivedBody map[string]any
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"POST /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				cmdtest.WriteJSONAPI(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-stdin",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "stdin-ws",
						},
					},
				})
			},
		}))

		client, err := inv.NewAPIClient()
		require.NoError(t, err)

		opts := &Opts{}
		opts.IO = io
		opts.Output = inv.Output
		opts.InputRequest = "-"
		opts.ProfileOrganization = "my-org"
		opts.Args = []string{"workspace"}
		opts.client = client

		// Write request body to stdin buffer.
		stdinJSON := `{"data":{"type":"workspaces","attributes":{"name":"stdin-ws"}}}`
		io.Input.WriteString(stdinJSON)

		err = runCreate(inv.ShutdownCtx, opts)
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-stdin")

		// Verify the server received the stdin contents.
		data := receivedBody["data"].(map[string]any)
		attrs := data["attributes"].(map[string]any)
		assert.Equal(t, "stdin-ws", attrs["name"])
	})
}

// TestNewCmdCreate_ArgValidation exercises the command through cmd.Command.Run
// to ensure the framework's arg-count validation accepts exactly 1 arg.
func TestNewCmdCreate_ArgValidation(t *testing.T) {
	t.Parallel()

	t.Run("one arg accepted by framework", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"POST /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, _ *http.Request) {
				cmdtest.WriteJSONAPI(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-created",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "new-ws",
						},
					},
				})
			},
		}))
		inv.Profile.DefaultOrganization = "my-org"

		c := NewCmdCreate(inv)
		root := &cmd.Command{Name: "tfctl"}
		cmd.ConfigureRootCommand(inv, root)
		root.AddChild(c)

		exitCode := c.Run([]string{"workspace", "-a", "name=new-ws"}, inv)
		assert.Equal(t, 0, exitCode)
		assert.Empty(t, io.Error.String())
		assert.Contains(t, io.Output.String(), "ws-created")
	})

	t.Run("zero args rejected by framework", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		c := NewCmdCreate(inv)
		root := &cmd.Command{Name: "tfctl"}
		cmd.ConfigureRootCommand(inv, root)
		root.AddChild(c)

		exitCode := c.Run([]string{}, inv)
		assert.NotEqual(t, 0, exitCode)
	})

	t.Run("two args rejected by framework", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		c := NewCmdCreate(inv)
		root := &cmd.Command{Name: "tfctl"}
		cmd.ConfigureRootCommand(inv, root)
		root.AddChild(c)

		exitCode := c.Run([]string{"workspace", "extra"}, inv)
		assert.NotEqual(t, 0, exitCode)
		assert.Contains(t, io.Error.String(), "accepts 1 arg(s)")
	})
}
