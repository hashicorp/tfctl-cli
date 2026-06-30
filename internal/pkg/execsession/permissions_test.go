// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package execsession

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassFromPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "organization", path: "/organizations/tfc-demo-au", want: "organizations"},
		{name: "workspace", path: "/workspaces/ws-abc", want: "workspaces"},
		{name: "project", path: "/projects/prj-abc", want: "projects"},
		{name: "run", path: "/runs/run-abc", want: "runs"},
		{name: "nested var", path: "/workspaces/ws-abc/vars/var-xyz", want: "vars"},
		{name: "relationship link removal", path: "/workspaces/ws/relationships/remote-state-consumers", want: "remote-state-consumers"},
		{name: "api v2 prefix", path: "/api/v2/workspaces/ws-abc", want: "workspaces"},
		{name: "trailing slash", path: "/workspaces/ws-abc/", want: "workspaces"},
		{name: "short path collection only", path: "/workspaces", want: ""},
		{name: "empty", path: "", want: ""},
		{name: "root", path: "/", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ClassFromPath(tc.path))
		})
	}
}

func TestAllowsDelete(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		granted []string
		class   string
		want    bool
	}{
		{name: "explicit class match", granted: []string{"workspaces"}, class: "workspaces", want: true},
		{name: "explicit class no match", granted: []string{"workspaces"}, class: "runs", want: false},
		{name: "reversible covers workspaces", granted: []string{SentinelReversible}, class: "workspaces", want: true},
		{name: "all covers workspaces", granted: []string{SentinelAll}, class: "workspaces", want: true},
		{name: "reversible covers runs", granted: []string{SentinelReversible}, class: "runs", want: true},
		{name: "reversible does NOT cover organizations", granted: []string{SentinelReversible}, class: "organizations", want: false},
		{name: "reversible does NOT cover projects", granted: []string{SentinelReversible}, class: "projects", want: false},
		{name: "all does NOT cover organizations", granted: []string{SentinelAll}, class: "organizations", want: false},
		{name: "all does NOT cover projects", granted: []string{SentinelAll}, class: "projects", want: false},
		{name: "explicit organizations allowed", granted: []string{"organizations"}, class: "organizations", want: true},
		{name: "explicit projects allowed", granted: []string{"projects"}, class: "projects", want: true},
		{name: "explicit projects plus reversible", granted: []string{SentinelReversible, "projects"}, class: "projects", want: true},
		{name: "empty class denied even with all", granted: []string{SentinelAll}, class: "", want: false},
		{name: "empty granted denied", granted: nil, class: "workspaces", want: false},
		{name: "empty class empty granted", granted: nil, class: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, AllowsDelete(tc.granted, tc.class))
		})
	}
}

func TestNormalizeAllowDelete(t *testing.T) {
	t.Parallel()

	t.Run("csv split and lowercase", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{"Workspaces,RUNS"})
		assert.Equal(t, []string{"workspaces", "runs"}, out)
		assert.Empty(t, warnings)
	})

	t.Run("trims whitespace", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{" workspaces , runs "})
		assert.Equal(t, []string{"workspaces", "runs"}, out)
		assert.Empty(t, warnings)
	})

	t.Run("sentinel passthrough", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{"reversible"})
		assert.Equal(t, []string{"reversible"}, out)
		assert.Empty(t, warnings)
	})

	t.Run("all sentinel passthrough", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{"all"})
		assert.Equal(t, []string{"all"}, out)
		assert.Empty(t, warnings)
	})

	t.Run("repeated flags", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{"workspaces", "runs"})
		assert.Equal(t, []string{"workspaces", "runs"}, out)
		assert.Empty(t, warnings)
	})

	t.Run("unknown class warns but is kept", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{"floopwidgets"})
		assert.Equal(t, []string{"floopwidgets"}, out)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "floopwidgets")
	})

	t.Run("irreversible classes known", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{"organizations", "projects"})
		assert.Equal(t, []string{"organizations", "projects"}, out)
		assert.Empty(t, warnings)
	})

	t.Run("empty entries dropped", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete([]string{"workspaces,,", ""})
		assert.Equal(t, []string{"workspaces"}, out)
		assert.Empty(t, warnings)
	})

	t.Run("deduplicates", func(t *testing.T) {
		t.Parallel()
		out, _ := NormalizeAllowDelete([]string{"workspaces", "workspaces"})
		assert.Equal(t, []string{"workspaces"}, out)
	})

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()
		out, warnings := NormalizeAllowDelete(nil)
		assert.Empty(t, out)
		assert.Empty(t, warnings)
	})
}
