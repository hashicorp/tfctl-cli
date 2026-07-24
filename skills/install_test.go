// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package skills

import (
	"os"
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

			installed := agent.DetectExistingSkill()
			require.NotNil(t, installed)
			require.Equal(t, expected, installed.Path)

			if c.expectedLocalInstall != "" {
				tmpLocal := t.TempDir()
				t.Chdir(tmpLocal)
				err := agent.InstallSkill(false)
				require.NoError(t, err)

				require.FileExists(t, c.expectedLocalInstall)

				installedLocal := agent.DetectExistingSkill()
				require.NotNil(t, installedLocal)
				require.Equal(t, c.expectedLocalInstall, installedLocal.Path)
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
