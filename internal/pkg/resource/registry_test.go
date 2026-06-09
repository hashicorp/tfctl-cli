// Copyright IBM Corp. 2026

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
