// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package destroy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdtest"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestRunDestroy(t *testing.T) {
	t.Parallel()

	t.Run("destroy workspace", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		io.ErrorTTY = true
		io.InputTTY = true
		io.Input.Write([]byte("y\n"))
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"DELETE /api/v2/workspaces/ws-abc123": func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
		}))

		opts := &Opts{Args: []string{"workspace", "ws-abc123"}}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runDestroy(inv.ShutdownCtx, opts)
		require.NoError(t, err)
	})

	t.Run("destroy explorer-saved-queries with org", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		io.ErrorTTY = true
		io.InputTTY = true
		io.Input.Write([]byte("y\n"))
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"DELETE /api/v2/organizations/my-org/explorer/views/sq-abc": func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
		}))

		opts := &Opts{
			Args:                []string{"explorer-saved-queries", "sq-abc"},
			Organization:        "my-org",
			ProfileOrganization: "my-org",
		}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runDestroy(inv.ShutdownCtx, opts)
		require.NoError(t, err)
	})

	t.Run("unknown resource type", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{Args: []string{"bogus", "id-123"}}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runDestroy(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown resource type")
	})

	t.Run("not destroyable", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{Args: []string{"runs", "run-abc"}}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runDestroy(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "destroy is not supported")
	})

	t.Run("ID prefix mismatch", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{Args: []string{"workspaces", "prj-wrong"}}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runDestroy(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not look like")
	})

	t.Run("missing arguments", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{Args: []string{"workspace"}}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runDestroy(inv.ShutdownCtx, opts)
		require.ErrorIs(t, err, cmd.ErrDisplayUsage)
	})

	t.Run("requires organization for org-scoped resources", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))

		opts := &Opts{Args: []string{"explorer-saved-queries", "sq-abc"}}
		opts.IO = io
		opts.Output = inv.Output

		client, err := inv.NewAPIClient()
		require.NoError(t, err)
		opts.client = client

		err = runDestroy(inv.ShutdownCtx, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization is required")
	})
}
