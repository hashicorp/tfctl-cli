// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mitchellh/go-homedir"
	"github.com/stretchr/testify/require"
)

func TestInstallSkill(t *testing.T) {
	cases := []struct {
		agentName             string
		expectedGlobalInstall string
		expectedLocalInstall  string
		setup                 func(t *testing.T)
	}{
		{
			agentName:             "amp",
			expectedGlobalInstall: "~/.config/agents/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".agents/skills/tfctl/SKILL.md",
		},
		{
			agentName:             "antigravity",
			expectedGlobalInstall: "~/.gemini/config/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".agents/skills/tfctl/SKILL.md",
		},
		{
			agentName:             "bob",
			expectedGlobalInstall: "~/.bob/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".bob/skills/tfctl/SKILL.md",
		},
		{
			agentName:             "claude",
			expectedGlobalInstall: "~/CustomClaudeDir/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".claude/skills/tfctl/SKILL.md",
			setup: func(t *testing.T) {
				t.Helper()
				customDir, err := homedir.Expand("~/CustomClaudeDir")
				if err != nil {
					t.Fatal(err)
				}
				originalClaudeConfig := os.Getenv("CLAUDE_CONFIG_DIR")
				err = os.Setenv("CLAUDE_CONFIG_DIR", customDir)
				if err != nil {
					t.Fatal(err)
				}

				t.Cleanup(func() {
					os.Setenv("CLAUDE_CONFIG_DIR", originalClaudeConfig)
				})
			},
		},
		{
			agentName:             "codex",
			expectedGlobalInstall: "~/.codex/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".codex/skills/tfctl/SKILL.md",
		},
		{
			agentName:             "copilot",
			expectedGlobalInstall: "~/.copilot/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".agents/skills/tfctl/SKILL.md",
		},
		{
			agentName:             "opencode",
			expectedGlobalInstall: "~/.config/opencode/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".agents/skills/tfctl/SKILL.md",
		},
		{
			agentName:             "pi",
			expectedGlobalInstall: "~/.pi/agent/skills/tfctl/SKILL.md",
			expectedLocalInstall:  ".agents/skills/tfctl/SKILL.md",
		},
	}

	for _, c := range cases {
		t.Run(c.agentName, func(t *testing.T) {
			tmpHome := t.TempDir()
			originalHome := os.Getenv("HOME")
			os.Setenv("HOME", tmpHome)
			homedir.Reset()
			t.Cleanup(func() {
				os.Setenv("HOME", originalHome)
				homedir.Reset()
			})

			if c.setup != nil {
				c.setup(t)
			}

			// Re-register agents to ensure they pick up the updated environment variables
			agents = registerAgents()

			agent, ok := GetAgent(c.agentName)
			require.True(t, ok, "agent should exist")

			err := agent.InstallSkill(true)
			require.NoError(t, err)

			expected, err := homedir.Expand(c.expectedGlobalInstall)
			require.NoError(t, err)
			require.FileExists(t, expected)

			installed := agent.DetectGloballyInstalledSkill()
			require.NotNil(t, installed)
			require.Equal(t, expected, installed.Path())

			if c.expectedLocalInstall != "" {
				tmpLocal := t.TempDir()
				t.Chdir(tmpLocal)
				err := agent.InstallSkill(false)
				require.NoError(t, err)

				require.FileExists(t, c.expectedLocalInstall)

				installedLocal := agent.DetectLocallyInstalledSkill()
				require.NotNil(t, installedLocal)
				require.Equal(t, c.expectedLocalInstall, installedLocal.Path())
			}
		})
	}

	// Make sure every agent has some basic fields defined
	for _, agent := range agents {
		agent.DetectInstalled()
		agent.DetectParentProcess()
		require.NotEmpty(t, agent.Name)
		require.NotEmpty(t, agent.DisplayName)
		require.NotEmpty(t, agent.SkillsDir)
		require.NotEmpty(t, agent.GlobalSkillsDir())
	}
}

func TestInstalledSkill_ResolvePath(t *testing.T) {
	t.Run("returns absolute path for regular file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "tfctl", "SKILL.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(t, os.WriteFile(filePath, []byte("test"), 0644))

		skill := &InstalledSkill{path: filePath, global: false, agentName: "pi"}
		resolved, err := skill.ResolvePath()
		require.NoError(t, err)
		expected, err := filepath.EvalSymlinks(filePath)
		require.NoError(t, err)
		require.Equal(t, expected, resolved)
	})

	t.Run("follows symlink to target", func(t *testing.T) {
		tmpDir := t.TempDir()
		realFile := filepath.Join(tmpDir, "real_skill.md")
		require.NoError(t, os.WriteFile(realFile, []byte("test"), 0644))

		linkPath := filepath.Join(tmpDir, "linked_skill.md")
		require.NoError(t, os.Symlink(realFile, linkPath))

		skill := &InstalledSkill{path: linkPath, global: false, agentName: "pi"}
		resolved, err := skill.ResolvePath()
		require.NoError(t, err)
		expected, err := filepath.EvalSymlinks(realFile)
		require.NoError(t, err)
		require.Equal(t, expected, resolved)
	})

	t.Run("follows symlink with relative target", func(t *testing.T) {
		tmpDir := t.TempDir()
		realFile := filepath.Join(tmpDir, "real_skill.md")
		require.NoError(t, os.WriteFile(realFile, []byte("test"), 0644))

		linkPath := filepath.Join(tmpDir, "linked_skill.md")
		require.NoError(t, os.Symlink("real_skill.md", linkPath))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		skill := &InstalledSkill{path: linkPath, global: false, agentName: "pi"}
		resolved, err := skill.ResolvePath()
		require.NoError(t, err)
		// resolved should point to the real file (basename match, since /var -> /private/var on macOS)
		require.Equal(t, "real_skill.md", filepath.Base(resolved))
		// verify the resolved path actually points to the real file
		resolvedAbs, err := filepath.EvalSymlinks(resolved)
		require.NoError(t, err)
		realAbs, err := filepath.EvalSymlinks(realFile)
		require.NoError(t, err)
		require.Equal(t, realAbs, resolvedAbs)
	})

	t.Run("follows nested symlinks", func(t *testing.T) {
		tmpDir := t.TempDir()
		realFile := filepath.Join(tmpDir, "real_skill.md")
		require.NoError(t, os.WriteFile(realFile, []byte("test"), 0644))

		secondLink := filepath.Join(tmpDir, "second_link.md")
		require.NoError(t, os.Symlink("real_skill.md", secondLink))
		firstLink := filepath.Join(tmpDir, "first_link.md")
		require.NoError(t, os.Symlink("second_link.md", firstLink))

		skill := &InstalledSkill{path: firstLink, global: false, agentName: "pi"}
		resolved, err := skill.ResolvePath()
		require.NoError(t, err)
		expected, err := filepath.EvalSymlinks(realFile)
		require.NoError(t, err)
		require.Equal(t, expected, resolved)
	})

	t.Run("returns error for empty path", func(t *testing.T) {
		skill := &InstalledSkill{path: "", global: false, agentName: "pi"}
		_, err := skill.ResolvePath()
		require.Error(t, err)
		require.Contains(t, err.Error(), "path is empty")
	})

	t.Run("returns error for non-existent path", func(t *testing.T) {
		skill := &InstalledSkill{path: "/nonexistent/path/SKILL.md", global: false, agentName: "pi"}
		_, err := skill.ResolvePath()
		require.Error(t, err)
	})
}

func TestAgentSpec_InstalledSkills(t *testing.T) {
	t.Run("returns both local and global skills", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()
		agent, ok := GetAgent("pi")
		require.True(t, ok)

		// Create local skill
		localPath := filepath.Join(tmpDir, agent.SkillsDir, TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0755))
		require.NoError(t, os.WriteFile(localPath, []byte("local"), 0644))

		// Create global skill
		globalPath := filepath.Join(agent.GlobalSkillsDir(), TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(globalPath), 0755))
		require.NoError(t, os.WriteFile(globalPath, []byte("global"), 0644))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		var collected []*InstalledSkill
		for s := range agent.InstalledSkills() {
			collected = append(collected, s)
		}

		require.Len(t, collected, 2)
		require.Equal(t, filepath.Join(agent.SkillsDir, TFCTLSkillPath), collected[0].Path())
		require.False(t, collected[0].global)
		require.Equal(t, globalPath, collected[1].Path())
		require.True(t, collected[1].global)
	})

	t.Run("returns only local skill when global does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()
		agent, ok := GetAgent("pi")
		require.True(t, ok)

		localPath := filepath.Join(tmpDir, agent.SkillsDir, TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0755))
		require.NoError(t, os.WriteFile(localPath, []byte("local"), 0644))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		var collected []*InstalledSkill
		for s := range agent.InstalledSkills() {
			collected = append(collected, s)
		}

		require.Len(t, collected, 1)
		require.Equal(t, filepath.Join(agent.SkillsDir, TFCTLSkillPath), collected[0].Path())
		require.False(t, collected[0].global)
	})

	t.Run("returns only global skill when local does not exist", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()
		agent, ok := GetAgent("pi")
		require.True(t, ok)

		globalPath := filepath.Join(agent.GlobalSkillsDir(), TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(globalPath), 0755))
		require.NoError(t, os.WriteFile(globalPath, []byte("global"), 0644))

		var collected []*InstalledSkill
		for s := range agent.InstalledSkills() {
			collected = append(collected, s)
		}

		require.Len(t, collected, 1)
		require.Equal(t, globalPath, collected[0].Path())
		require.True(t, collected[0].global)
	})

	t.Run("returns nothing when no skills exist", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()
		agent, ok := GetAgent("pi")
		require.True(t, ok)

		var collected []*InstalledSkill
		for s := range agent.InstalledSkills() {
			collected = append(collected, s)
		}

		require.Empty(t, collected)
	})

	t.Run("skips directories with same name as skill file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()
		agent, ok := GetAgent("pi")
		require.True(t, ok)

		// Create a directory instead of a file
		dirPath := filepath.Join(tmpDir, agent.SkillsDir, "tfctl", "SKILL.md")
		require.NoError(t, os.MkdirAll(dirPath, 0755))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		var collected []*InstalledSkill
		for s := range agent.InstalledSkills() {
			collected = append(collected, s)
		}

		require.Empty(t, collected)
	})
}

func TestDetectAnyExistingSkill(t *testing.T) {
	t.Run("returns nil when no skills exist", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()
		result := DetectAnyExistingSkill()
		require.Nil(t, result)
	})

	t.Run("returns first local skill found", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()

		// Create a local skill for amp
		ampAgent, ok := GetAgent("amp")
		require.True(t, ok)
		localPath := filepath.Join(tmpDir, ampAgent.SkillsDir, TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0755))
		require.NoError(t, os.WriteFile(localPath, []byte("amp skill"), 0644))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		result := DetectAnyExistingSkill()
		require.NotNil(t, result)
		require.Equal(t, "amp", result.agentName)
		require.False(t, result.global)
	})

	t.Run("returns first global skill when no local skills exist", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()

		// Create a global skill for amp
		ampAgent, ok := GetAgent("amp")
		require.True(t, ok)
		globalPath := filepath.Join(ampAgent.GlobalSkillsDir(), TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(globalPath), 0755))
		require.NoError(t, os.WriteFile(globalPath, []byte("amp skill"), 0644))

		result := DetectAnyExistingSkill()
		require.NotNil(t, result)
		require.Equal(t, "amp", result.agentName)
		require.True(t, result.global)
	})

	t.Run("prefers local over global", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()

		// Create both local and global skills for amp
		ampAgent, ok := GetAgent("amp")
		require.True(t, ok)

		localPath := filepath.Join(tmpDir, ampAgent.SkillsDir, TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0755))
		require.NoError(t, os.WriteFile(localPath, []byte("local"), 0644))

		globalPath := filepath.Join(ampAgent.GlobalSkillsDir(), TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(globalPath), 0755))
		require.NoError(t, os.WriteFile(globalPath, []byte("global"), 0644))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		result := DetectAnyExistingSkill()
		require.NotNil(t, result)
		require.False(t, result.global)
		require.Contains(t, result.Path(), ".agents")
	})

	t.Run("ignores directories with same name as skill file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agents = registerAgents()

		ampAgent, ok := GetAgent("amp")
		require.True(t, ok)

		// Create a directory instead of a file
		dirPath := filepath.Join(tmpDir, ampAgent.SkillsDir, "tfctl", "SKILL.md")
		require.NoError(t, os.MkdirAll(dirPath, 0755))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		result := DetectAnyExistingSkill()
		require.Nil(t, result)
	})
}

func TestAgentSpec_DetectLocallyInstalledSkill(t *testing.T) {
	t.Run("returns nil when skill does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		agent := AgentSpec{Name: "pi", SkillsDir: ".agents/skills"}
		result := agent.DetectLocallyInstalledSkill()
		require.Nil(t, result)
	})

	t.Run("returns InstalledSkill when file exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent := AgentSpec{Name: "pi", SkillsDir: ".agents/skills"}
		localPath := filepath.Join(tmpDir, agent.SkillsDir, TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0755))
		require.NoError(t, os.WriteFile(localPath, []byte("skill content"), 0644))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		result := agent.DetectLocallyInstalledSkill()
		require.NotNil(t, result)
		require.Equal(t, filepath.Join(agent.SkillsDir, TFCTLSkillPath), result.Path())
		require.False(t, result.global)
		require.Equal(t, "pi", result.agentName)
	})

	t.Run("returns nil when path is a directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		agent := AgentSpec{Name: "pi", SkillsDir: ".agents/skills"}
		dirPath := filepath.Join(tmpDir, agent.SkillsDir, "tfctl", "SKILL.md")
		require.NoError(t, os.MkdirAll(dirPath, 0755))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		result := agent.DetectLocallyInstalledSkill()
		require.Nil(t, result)
	})

	t.Run("respects agent-specific skills directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// bob uses .bob/skills
		agent := AgentSpec{Name: "bob", SkillsDir: ".bob/skills"}
		localPath := filepath.Join(tmpDir, agent.SkillsDir, TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0755))
		require.NoError(t, os.WriteFile(localPath, []byte("skill content"), 0644))

		oldWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(oldWd) })

		result := agent.DetectLocallyInstalledSkill()
		require.NotNil(t, result)
		require.Contains(t, result.Path(), ".bob/skills")
	})
}

func TestAgentSpec_DetectGloballyInstalledSkill(t *testing.T) {
	t.Run("returns nil when global skill does not exist", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		agent := AgentSpec{
			Name:            "pi",
			SkillsDir:       ".agents/skills",
			GlobalSkillsDir: func() string { return filepath.Join(tmpHome, ".pi/agent/skills") },
		}
		result := agent.DetectGloballyInstalledSkill()
		require.Nil(t, result)
	})

	t.Run("returns InstalledSkill when global file exists", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		globalSkillsDir := filepath.Join(tmpHome, ".pi/agent/skills")
		agent := AgentSpec{
			Name:            "pi",
			SkillsDir:       ".agents/skills",
			GlobalSkillsDir: func() string { return globalSkillsDir },
		}
		globalPath := filepath.Join(globalSkillsDir, TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(globalPath), 0755))
		require.NoError(t, os.WriteFile(globalPath, []byte("global skill"), 0644))

		result := agent.DetectGloballyInstalledSkill()
		require.NotNil(t, result)
		require.Equal(t, globalPath, result.Path())
		require.True(t, result.global)
		require.Equal(t, "pi", result.agentName)
	})

	t.Run("returns nil when path is a directory", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		globalSkillsDir := filepath.Join(tmpHome, ".pi/agent/skills")
		agent := AgentSpec{
			Name:            "pi",
			SkillsDir:       ".agents/skills",
			GlobalSkillsDir: func() string { return globalSkillsDir },
		}
		dirPath := filepath.Join(globalSkillsDir, "tfctl", "SKILL.md")
		require.NoError(t, os.MkdirAll(dirPath, 0755))

		result := agent.DetectGloballyInstalledSkill()
		require.Nil(t, result)
	})

	t.Run("respects custom global skills directory", func(t *testing.T) {
		tmpHome := t.TempDir()
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpHome)
		homedir.Reset()
		t.Cleanup(func() {
			os.Setenv("HOME", originalHome)
			homedir.Reset()
		})

		// Claude can have a custom global dir via CLAUDE_CONFIG_DIR
		customDir := filepath.Join(tmpHome, "my-claude-config")
		originalEnv := os.Getenv("CLAUDE_CONFIG_DIR")
		os.Setenv("CLAUDE_CONFIG_DIR", customDir)
		t.Cleanup(func() {
			os.Setenv("CLAUDE_CONFIG_DIR", originalEnv)
		})

		agents = registerAgents()
		agent, ok := GetAgent("claude")
		require.True(t, ok)

		globalPath := filepath.Join(customDir, "skills", TFCTLSkillPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(globalPath), 0755))
		require.NoError(t, os.WriteFile(globalPath, []byte("claude skill"), 0644))

		result := agent.DetectGloballyInstalledSkill()
		require.NotNil(t, result)
		require.Contains(t, result.Path(), "my-claude-config")
		require.True(t, result.global)
	})

	t.Run("iterates agents in sorted order for determinism", func(t *testing.T) {
		// AgentNames is sorted in init(), verify that DetectAnyExistingSkill
		// finds skills in a deterministic order
		require.True(t, slices.IsSorted(AgentNames))
	})
}
