// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmdutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestResolveOrganization(t *testing.T) {
	t.Parallel()

	t.Run("explicit flag wins", func(t *testing.T) {
		t.Parallel()
		ctx := &cmd.Context{
			IO:          iostreams.Test(),
			ShutdownCtx: context.Background(),
			Profile:     &profile.Profile{Organization: "profile-org"},
		}
		got := ResolveOrganization(ctx, "flag-org")
		assert.Equal(t, "flag-org", got)
	})

	t.Run("falls back to profile", func(t *testing.T) {
		t.Parallel()
		ctx := &cmd.Context{
			IO:          iostreams.Test(),
			ShutdownCtx: context.Background(),
			Profile:     &profile.Profile{Organization: "profile-org"},
		}
		got := ResolveOrganization(ctx, "")
		assert.Equal(t, "profile-org", got)
	})

	t.Run("returns empty when nothing set", func(t *testing.T) {
		t.Parallel()
		ctx := &cmd.Context{
			IO:          iostreams.Test(),
			ShutdownCtx: context.Background(),
			Profile:     &profile.Profile{},
		}
		got := ResolveOrganization(ctx, "")
		// May pick up from terraform cloud config if present in cwd,
		// but in test env it should be empty.
		assert.Equal(t, "", got)
	})
}

func TestResolvePath(t *testing.T) {
	t.Parallel()

	t.Run("substitutes organization_name", func(t *testing.T) {
		t.Parallel()
		got, err := ResolvePath("/organizations/{organization_name}/workspaces", "my-org")
		require.NoError(t, err)
		assert.Equal(t, "/organizations/my-org/workspaces", got)
	})

	t.Run("no placeholder passes through", func(t *testing.T) {
		t.Parallel()
		got, err := ResolvePath("/workspaces/{id}", "my-org")
		require.NoError(t, err)
		assert.Equal(t, "/workspaces/{id}", got)
	})

	t.Run("empty org with placeholder errors", func(t *testing.T) {
		t.Parallel()
		_, err := ResolvePath("/organizations/{organization_name}/workspaces", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization is required but not set")
	})

	t.Run("empty org without placeholder succeeds", func(t *testing.T) {
		t.Parallel()
		got, err := ResolvePath("/runs/{id}", "")
		require.NoError(t, err)
		assert.Equal(t, "/runs/{id}", got)
	})
}
