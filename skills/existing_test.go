// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstalledSkill_ReinstallCommand(t *testing.T) {
	cases := []struct {
		name      string
		global    bool
		agentName string
		expected  string
	}{
		{
			name:      "global",
			global:    true,
			agentName: "opencode",
			expected:  "tfctl harness install --global opencode",
		},
		{
			name:      "local",
			global:    false,
			agentName: "opencode",
			expected:  "tfctl harness install opencode",
		},
		{
			name:      "global claude",
			global:    true,
			agentName: "claude",
			expected:  "tfctl harness install --global claude",
		},
		{
			name:      "local claude",
			global:    false,
			agentName: "claude",
			expected:  "tfctl harness install claude",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &InstalledSkill{
				path:      "/some/path/SKILL.md",
				global:    c.global,
				agentName: c.agentName,
			}
			require.Equal(t, c.expected, s.ReinstallCommand())
		})
	}
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

func TestInstalledSkill_KnownVersion(t *testing.T) {
	t.Run("matches embedded hash", func(t *testing.T) {
		dir := t.TempDir()
		dst := filepath.Join(dir, "SKILL.md")
		require.NoError(t, copyFile(filepath.Join("fixtures", "0_3_0.md"), dst))

		s := &InstalledSkill{path: dst}
		ver, ok := s.MatchesKnownVersion()
		if ok {
			require.NotEmpty(t, ver)
		}
		require.Equal(t, ver.Version, "v0.3.0")
	})

	t.Run("unknown content returns no match", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "SKILL.md")
		require.NoError(t, os.WriteFile(path, []byte("unknown content that won't match any hash"), 0644))

		s := &InstalledSkill{path: path}
		version, ok := s.MatchesKnownVersion()
		require.False(t, ok)
		require.Empty(t, version)
	})

	t.Run("missing file returns no match", func(t *testing.T) {
		s := &InstalledSkill{path: "/nonexistent/path/SKILL.md"}
		version, ok := s.MatchesKnownVersion()
		require.False(t, ok)
		require.Empty(t, version)
	})

	t.Run("empty file returns no match", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "SKILL.md")
		require.NoError(t, os.WriteFile(path, []byte{}, 0644))

		s := &InstalledSkill{path: path}
		version, ok := s.MatchesKnownVersion()
		require.False(t, ok)
		require.Empty(t, version)
	})
}

func TestMatchesKnownVersionAtPath_UsesResolvedPath(t *testing.T) {
	dir := t.TempDir()
	knownPath := filepath.Join(dir, "known.md")
	unknownPath := filepath.Join(dir, "unknown.md")
	linkPath := filepath.Join(dir, "SKILL.md")

	require.NoError(t, copyFile(filepath.Join("fixtures", "0_3_0.md"), knownPath))
	require.NoError(t, os.WriteFile(unknownPath, []byte("user-edited content"), 0644))
	require.NoError(t, os.Symlink(knownPath, linkPath))

	skill := &InstalledSkill{path: linkPath}
	resolvedPath, err := skill.ResolvePath()
	require.NoError(t, err)

	// Simulate the original symlink changing after migration resolves its target.
	require.NoError(t, os.Remove(linkPath))
	require.NoError(t, os.Symlink(unknownPath, linkPath))

	match, ok := matchesKnownVersionAtPath(resolvedPath)
	require.True(t, ok)
	require.Equal(t, "v0.3.0", match.Version)
}
