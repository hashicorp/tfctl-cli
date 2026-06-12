// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package skills

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/mitchellh/go-homedir"
)

// AgentSpec defines the necessary information to install a skill for a coding agent.
type AgentSpec struct {
	Name                string
	DisplayName         string
	SkillsDir           string
	GlobalSkillsDir     func() string
	Detect              func() bool
	DetectParentProcess func() bool
}

func detectHomeDirPath(dir string) bool {
	path, err := homedir.Expand(fmt.Sprintf("~/%s", dir))
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// TFCTLSkillPath is the path to the embedded SKILL.md file within the binary.
const TFCTLSkillPath = "tfctl/SKILL.md"

var agents map[string]AgentSpec

// AgentNames is a list of the names of all supported agents.
var AgentNames []string

func init() {
	agents = registerAgents()

	AgentNames = make([]string, len(agents))
	i := 0
	for k := range agents {
		AgentNames[i] = k
		i++
	}

	slices.Sort(AgentNames)
}

func registerAgents() map[string]AgentSpec {
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		claudeDir, _ = homedir.Expand("~/.claude")
	}

	codexDir := os.Getenv("CODEX_HOME")
	if codexDir == "" {
		codexDir, _ = homedir.Expand("~/.codex")
	}

	return map[string]AgentSpec{
		"antigravity": {
			Name:        "antigravity",
			DisplayName: "Antigravity CLI",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.gemini/config/skills")
				return path
			},
			Detect: func() bool {
				return detectHomeDirPath(".gemini")
			},
			DetectParentProcess: func() bool {
				// TODO
				return false
			},
		},
		"bob": {
			Name:        "bob",
			DisplayName: "IBM Bob",
			SkillsDir:   ".bob/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.bob/skills")
				return path
			},
			Detect: func() bool {
				return detectHomeDirPath(".bob")
			},
			DetectParentProcess: func() bool {
				// TODO
				return false
			},
		},
		"claude": {
			Name:        "claude",
			DisplayName: "Claude Code",
			SkillsDir:   ".claude/skills",
			GlobalSkillsDir: func() string {
				return filepath.Join(claudeDir, "skills")
			},
			Detect: func() bool {
				_, err := os.Stat(claudeDir)
				return err == nil
			},
			DetectParentProcess: func() bool {
				return os.Getenv("CLAUDECODE") == "1"
			},
		},
		"codex": {
			Name:        "codex",
			DisplayName: "OpenAI Codex",
			SkillsDir:   ".codex/skills",
			GlobalSkillsDir: func() string {
				return filepath.Join(codexDir, "skills")
			},
			Detect: func() bool {
				_, err := os.Stat(codexDir)
				return err == nil
			},
			DetectParentProcess: func() bool {
				// TODO
				return false
			},
		},
		"copilot": {
			Name:        "copilot",
			DisplayName: "GitHub Copilot",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.copilot/skills")
				return path
			},
			Detect: func() bool {
				return detectHomeDirPath(".copilot")
			},
			DetectParentProcess: func() bool {
				return os.Getenv("COPILOT_GH") == "true" || os.Getenv("COPILOT_CLI") == "1"
			},
		},
		"opencode": {
			Name:        "opencode",
			DisplayName: "OpenCode",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.config/opencode/skills")
				return path
			},
			Detect: func() bool {
				return detectHomeDirPath(".config/opencode")
			},
			DetectParentProcess: func() bool {
				return os.Getenv("OPENCODE") == "1"
			},
		},
		"pi": {
			Name:        "pi",
			DisplayName: "Pi CLI",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.pi/agent/skills")
				return path
			},
			Detect: func() bool {
				return detectHomeDirPath(".pi")
			},
			DetectParentProcess: func() bool {
				return os.Getenv("PI_CODING_AGENT") == "true"
			},
		},
	}
}

// GetAgent returns the AgentSpec for a given agent name, along with a boolean indicating whether
// the agent was found.
func GetAgent(name string) (AgentSpec, bool) {
	agent, ok := agents[name]
	return agent, ok
}

// DetectAgent returns a list of AgentSpecs for agents detected on the current system.
func DetectAgent() []AgentSpec {
	var detected []AgentSpec
	for _, agent := range agents {
		if agent.Detect() {
			detected = append(detected, agent)
		}
	}

	return detected
}

// InstallSkill installs the tfctl skill for the agent, either to the project directory or the
// global config directory based on the value of the global parameter.
func (a AgentSpec) InstallSkill(global bool) error {
	file, err := FS.Open(TFCTLSkillPath)
	if err != nil {
		return fmt.Errorf("failed to open embedded SKILL.md file: %w", err)
	}
	defer file.Close()

	targetDir := a.SkillsDir
	if global {
		targetDir = a.GlobalSkillsDir()
	}

	targetDir = filepath.Join(targetDir, "tfctl")

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %q: %w", targetDir, err)
	}

	targetPath := fmt.Sprintf("%s/SKILL.md", targetDir)
	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file %q: %w", targetPath, err)
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, file)
	if err != nil {
		return fmt.Errorf("failed to copy skill file to target location: %w", err)
	}
	return nil
}
