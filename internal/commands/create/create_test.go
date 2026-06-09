// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package create

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
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
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
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
		ctx.Profile.Organization = "my-org"

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"name": "foo"},
		}, hclog.NewNullLogger(), []string{"workspace"})
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
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
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
		ctx.Profile.Organization = "my-org"

		inputJSON := `{"data":{"type":"workspaces","attributes":{"name":"from-input"}}}`
		err := runCreate(ctx, &Opts{
			InputBody: inputJSON,
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-input123")
	})

	t.Run("create workspace dry run", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))
		ctx.Profile.Organization = "my-org"

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"name": "foo"},
			DryRun:     true,
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		// Dry run should show the request info on stderr
		assert.Contains(t, io.Error.String(), "POST")
	})

	t.Run("create with no attrs and no input errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))
		ctx.Profile.Organization = "my-org"

		err := runCreate(ctx, &Opts{}, hclog.NewNullLogger(), []string{"workspace"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provide attributes with -a key=value or a request body with -i")
	})

	t.Run("create unsupported resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"x": "y"},
		}, hclog.NewNullLogger(), []string{"apply"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create is not supported for applies")
	})

	t.Run("create unknown resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"x": "y"},
		}, hclog.NewNullLogger(), []string{"blah"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
		assert.Contains(t, err.Error(), "Available resources:")
	})

	t.Run("create workspace without org errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"name": "foo"},
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization is required but not set")
	})

	t.Run("no args returns usage", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		err := runCreate(ctx, &Opts{}, hclog.NewNullLogger(), []string{})
		require.ErrorIs(t, err, cmd.ErrDisplayUsage)
	})

	t.Run("both attributes and input body errors", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))
		ctx.Profile.Organization = "my-org"

		err := runCreate(ctx, &Opts{
			Attributes: map[string]string{"name": "foo"},
			InputBody:  `{"data":{"type":"workspaces"}}`,
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot use both -a (attributes) and -i (input body)")
	})

	t.Run("explicit org flag overrides profile", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
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
		ctx.Profile.Organization = "profile-org"

		err := runCreate(ctx, &Opts{
			Organization: "flag-org",
			Attributes:   map[string]string{"name": "created-in-flag-org"},
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-flagorg")
	})

	t.Run("create workspace with @filename input", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var receivedBody map[string]any
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"POST /api/v2/organizations/my-org/workspaces": func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&receivedBody)
				cmdtest.WriteJSONAPI(w, map[string]any{
					"data": map[string]any{
						"id":   "ws-fromfile",
						"type": "workspaces",
						"attributes": map[string]any{
							"name": "file-ws",
						},
					},
				})
			},
		}))
		ctx.Profile.Organization = "my-org"

		// Write a temp file with the request body.
		dir := t.TempDir()
		bodyFile := filepath.Join(dir, "body.json")
		bodyJSON := `{"data":{"type":"workspaces","attributes":{"name":"file-ws"}}}`
		require.NoError(t, os.WriteFile(bodyFile, []byte(bodyJSON), 0o644))

		err := runCreate(ctx, &Opts{
			InputBody: "@" + bodyFile,
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-fromfile")

		// Verify the server received the file contents.
		data := receivedBody["data"].(map[string]any)
		assert.Equal(t, "workspaces", data["type"])
	})

	t.Run("create workspace with stdin input", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		var receivedBody map[string]any
		ctx := cmdtest.NewContext(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
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
		ctx.Profile.Organization = "my-org"

		// Write request body to stdin buffer.
		stdinJSON := `{"data":{"type":"workspaces","attributes":{"name":"stdin-ws"}}}`
		io.Input.WriteString(stdinJSON)

		err := runCreate(ctx, &Opts{
			InputBody: "-",
		}, hclog.NewNullLogger(), []string{"workspace"})
		require.NoError(t, err)
		assert.Contains(t, io.Output.String(), "ws-stdin")

		// Verify the server received the stdin contents.
		data := receivedBody["data"].(map[string]any)
		attrs := data["attributes"].(map[string]any)
		assert.Equal(t, "stdin-ws", attrs["name"])
	})
}
