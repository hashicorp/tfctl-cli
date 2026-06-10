// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantType string
		wantNil  bool
	}{
		{
			name:     "canonical name",
			input:    "workspaces",
			wantType: "workspaces",
		},
		{
			name:     "alias",
			input:    "ws",
			wantType: "workspaces",
		},
		{
			name:     "singular alias",
			input:    "workspace",
			wantType: "workspaces",
		},
		{
			name:     "uppercase",
			input:    "Workspaces",
			wantType: "workspaces",
		},
		{
			name:     "mixed case alias",
			input:    "WS",
			wantType: "workspaces",
		},
		{
			name:    "unknown",
			input:   "foobar",
			wantNil: true,
		},
		{
			name:    "empty",
			input:   "",
			wantNil: true,
		},
		{
			name:     "runs canonical",
			input:    "runs",
			wantType: "runs",
		},
		{
			name:     "run alias",
			input:    "run",
			wantType: "runs",
		},
		{
			name:     "variable-sets alias",
			input:    "variable-sets",
			wantType: "varsets",
		},
		{
			name:     "org alias",
			input:    "org",
			wantType: "organizations",
		},
		{
			name:     "configuration-version alias",
			input:    "cv",
			wantType: "configuration-versions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ByName(tt.input)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantType, got.Type)
			}
		})
	}
}

func TestByIDPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantType string
		wantNil  bool
	}{
		{
			name:     "workspace ID",
			input:    "ws-abc123",
			wantType: "workspaces",
		},
		{
			name:     "run ID",
			input:    "run-xyz789",
			wantType: "runs",
		},
		{
			name:     "plan ID",
			input:    "plan-def456",
			wantType: "plans",
		},
		{
			name:     "cv ID",
			input:    "cv-abc123",
			wantType: "configuration-versions",
		},
		{
			name:    "unknown prefix",
			input:   "xyz-123",
			wantNil: true,
		},
		{
			name:    "empty",
			input:   "",
			wantNil: true,
		},
		{
			name:     "prefix only",
			input:    "ws-",
			wantType: "workspaces",
		},
		{
			name:    "no dash",
			input:   "workspaces",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ByIDPrefix(tt.input)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantType, got.Type)
			}
		})
	}
}

func TestAll(t *testing.T) {
	t.Parallel()

	all := All()
	assert.NotEmpty(t, all)
	assert.Len(t, all, len(registry))

	// Ensure it's a copy, not a reference to the original
	all[0].Type = "modified"
	assert.NotEqual(t, "modified", registry[0].Type)
}

func TestNames(t *testing.T) {
	t.Parallel()

	names := Names()
	assert.NotEmpty(t, names)
	assert.Len(t, names, len(registry))

	// Verify sorted
	assert.True(t, sort.StringsAreSorted(names))

	// Verify contains expected entries
	assert.Contains(t, names, "workspaces")
	assert.Contains(t, names, "runs")
	assert.Contains(t, names, "organizations")
}

func TestCompletionNames(t *testing.T) {
	t.Parallel()

	names := CompletionNames()
	assert.NotEmpty(t, names)
	assert.True(t, sort.StringsAreSorted(names))

	// Should include canonical types and aliases.
	assert.Contains(t, names, "workspaces")
	assert.Contains(t, names, "workspace")
	assert.Contains(t, names, "ws")
	assert.Contains(t, names, "runs")
	assert.Contains(t, names, "run")

	// Should be longer than Names() since it includes aliases.
	assert.Greater(t, len(names), len(Names()))
}

func TestCreatableNames(t *testing.T) {
	t.Parallel()

	names := CreatableNames()
	assert.NotEmpty(t, names)
	assert.True(t, sort.StringsAreSorted(names))

	// Should include types that have PathCreate.
	assert.Contains(t, names, "workspaces")
	assert.Contains(t, names, "workspace")
	assert.Contains(t, names, "projects")

	// Should NOT include types without PathCreate (e.g. runs, applies).
	assert.NotContains(t, names, "runs")
	assert.NotContains(t, names, "run")
	assert.NotContains(t, names, "applies")
}

// TestRegistryInvariants validates structural invariants that keep the registry
// correct across get, create, and table rendering subsystems.
func TestRegistryInvariants(t *testing.T) {
	t.Parallel()

	all := All()

	t.Run("no duplicate types", func(t *testing.T) {
		t.Parallel()
		seen := make(map[string]bool, len(all))
		for _, r := range all {
			if seen[r.Type] {
				t.Errorf("duplicate type: %q", r.Type)
			}
			seen[r.Type] = true
		}
	})

	t.Run("no duplicate aliases", func(t *testing.T) {
		t.Parallel()
		seen := make(map[string]string) // alias -> owning type
		for _, r := range all {
			for _, alias := range r.Aliases {
				if owner, ok := seen[alias]; ok {
					t.Errorf("alias %q used by both %q and %q", alias, owner, r.Type)
				}
				seen[alias] = r.Type
			}
		}
	})

	t.Run("aliases do not collide with canonical types", func(t *testing.T) {
		t.Parallel()
		types := make(map[string]bool, len(all))
		for _, r := range all {
			types[r.Type] = true
		}
		for _, r := range all {
			for _, alias := range r.Aliases {
				if types[alias] && alias != r.Type {
					t.Errorf("alias %q of %q collides with another resource's canonical type", alias, r.Type)
				}
			}
		}
	})

	t.Run("PathGet contains {id}", func(t *testing.T) {
		t.Parallel()
		for _, r := range all {
			if r.PathGet == "" {
				continue
			}
			assert.Containsf(t, r.PathGet, "{id}",
				"resource %q PathGet=%q must contain {id}", r.Type, r.PathGet)
		}
	})

	t.Run("PathCreate contains {organization_name}", func(t *testing.T) {
		t.Parallel()
		for _, r := range all {
			if r.PathCreate == "" {
				continue
			}
			assert.Containsf(t, r.PathCreate, "{organization_name}",
				"resource %q PathCreate=%q must contain {organization_name}", r.Type, r.PathCreate)
		}
	})

	t.Run("PathCreate implies PathGet", func(t *testing.T) {
		t.Parallel()
		for _, r := range all {
			if r.PathCreate == "" {
				continue
			}
			assert.NotEmptyf(t, r.PathGet,
				"resource %q has PathCreate but no PathGet", r.Type)
		}
	})

	t.Run("Resolvable implies IDPrefix and PathList", func(t *testing.T) {
		t.Parallel()
		for _, r := range all {
			if !r.Resolvable {
				continue
			}
			assert.NotEmptyf(t, r.IDPrefix,
				"resource %q is Resolvable but has no IDPrefix", r.Type)
			assert.NotEmptyf(t, r.PathList,
				"resource %q is Resolvable but has no PathList", r.Type)
		}
	})

	t.Run("IDPrefix ends with dash", func(t *testing.T) {
		t.Parallel()
		for _, r := range all {
			if r.IDPrefix == "" {
				continue
			}
			assert.Truef(t, r.IDPrefix[len(r.IDPrefix)-1] == '-',
				"resource %q IDPrefix=%q must end with '-'", r.Type, r.IDPrefix)
		}
	})

	t.Run("no duplicate IDPrefixes", func(t *testing.T) {
		t.Parallel()
		seen := make(map[string]string) // prefix -> owning type
		for _, r := range all {
			if r.IDPrefix == "" {
				continue
			}
			if owner, ok := seen[r.IDPrefix]; ok {
				t.Errorf("IDPrefix %q used by both %q and %q", r.IDPrefix, owner, r.Type)
			}
			seen[r.IDPrefix] = r.Type
		}
	})
}
