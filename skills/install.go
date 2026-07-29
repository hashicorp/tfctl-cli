// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package skills

import (
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"slices"

	"github.com/mitchellh/go-homedir"

	"github.com/hashicorp/tfctl-cli/version"
)

// AgentSpec defines the necessary information to install a skill for a coding agent.
type AgentSpec struct {
	Name                string
	DisplayName         string
	SkillsDir           string
	GlobalSkillsDir     func() string
	DetectInstalled     func() bool
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

const (
	// TFCTLSkillPath is the path to the embedded SKILL.md file within the binary.
	TFCTLSkillPath = "tfctl/SKILL.md"
	// TFCTLKnownHashesPath is the path to the embedded hashes file within the binary.
	TFCTLKnownHashesPath = "tfctl/known_release_hashes"
)

var (
	agents map[string]AgentSpec

	// AgentNames is a list of the names of all supported agents.
	AgentNames []string
)

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
		"amp": {
			Name:        "amp",
			DisplayName: "Amp CLI",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.config/agents/skills")
				return path
			},
			DetectInstalled: func() bool {
				return detectHomeDirPath(".config/amp")
			},
			DetectParentProcess: func() bool {
				return os.Getenv("AGENT") == "amp"
			},
		},
		"antigravity": {
			Name:        "antigravity",
			DisplayName: "Antigravity CLI",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.gemini/config/skills")
				return path
			},
			DetectInstalled: func() bool {
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
			DetectInstalled: func() bool {
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
			DetectInstalled: func() bool {
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
			DetectInstalled: func() bool {
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
			DetectInstalled: func() bool {
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
			DetectInstalled: func() bool {
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
			DetectInstalled: func() bool {
				return detectHomeDirPath(".pi")
			},
			DetectParentProcess: func() bool {
				return os.Getenv("PI_CODING_AGENT") == "true"
			},
		},
	}
}

// DetectAnyExistingSkill checks returns the first existing skill it finds from any known
// agent, starting with locally install skills, or nil if none are found.
func DetectAnyExistingSkill() *InstalledSkill {
	for _, name := range AgentNames {
		if agent, ok := GetAgent(name); ok {
			if s := agent.DetectLocallyInstalledSkill(); s != nil {
				return s
			}
		}
	}
	for _, name := range AgentNames {
		if agent, ok := GetAgent(name); ok {
			if s := agent.DetectGloballyInstalledSkill(); s != nil {
				return s
			}
		}
	}
	return nil
}

// DetectLocallyInstalledSkill checks if the tfctl skill already exists for the agent in the
// local project directory, and returns an InstalledSkill if found.
func (a *AgentSpec) DetectLocallyInstalledSkill() *InstalledSkill {
	skillPath := filepath.Join(a.SkillsDir, TFCTLSkillPath)
	if s, err := os.Stat(skillPath); err == nil && !s.IsDir() {
		return &InstalledSkill{path: skillPath, global: false, agentName: a.Name}
	}
	return nil
}

// DetectGloballyInstalledSkill checks if the tfctl skill already exists for the agent in the
// global config directory, and returns an InstalledSkill if found.
func (a *AgentSpec) DetectGloballyInstalledSkill() *InstalledSkill {
	globalSkillPath := filepath.Join(a.GlobalSkillsDir(), TFCTLSkillPath)
	if s, err := os.Stat(globalSkillPath); err == nil && !s.IsDir() {
		return &InstalledSkill{path: globalSkillPath, global: true, agentName: a.Name}
	}
	return nil
}

// InstalledSkills returns a sequence of both/either/neither installed skills for the agent.
// First local, then global.
func (a *AgentSpec) InstalledSkills() iter.Seq[*InstalledSkill] {
	return func(yield func(*InstalledSkill) bool) {
		if s := a.DetectLocallyInstalledSkill(); s != nil {
			if !yield(s) {
				return
			}
		}
		if s := a.DetectGloballyInstalledSkill(); s != nil {
			if !yield(s) {
				return
			}
		}
	}
}

// GetAgent returns the AgentSpec for a given agent name, along with a boolean indicating whether
// the agent was found.
func GetAgent(name string) (AgentSpec, bool) {
	agent, ok := agents[name]
	return agent, ok
}

// DetectAgent returns the first AgentSpec for any agent detected on the current system.
func DetectAgent() (AgentSpec, bool) {
	for _, agent := range agents {
		if agent.DetectInstalled() {
			return agent, true
		}
	}
	return AgentSpec{}, false
}

// DetectAgents returns a list of AgentSpecs for agents detected on the current system.
func DetectAgents() []AgentSpec {
	var detected []AgentSpec
	for _, agent := range agents {
		if agent.DetectInstalled() {
			detected = append(detected, agent)
		}
	}

	return detected
}

func (a AgentSpec) skillFilePath(global bool) (string, error) {
	targetDir := a.SkillsDir
	if global {
		targetDir = a.GlobalSkillsDir()
	}

	targetDir = filepath.Join(targetDir, "tfctl")

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory %q: %w", targetDir, err)
	}

	return fmt.Sprintf("%s/SKILL.md", targetDir), nil
}

// installSkillToPath installs the tfctl skill for the agent to a specific file path.
func (a AgentSpec) installSkillToPath(path string) error {
	file, err := FS.Open(TFCTLSkillPath)
	if err != nil {
		return fmt.Errorf("failed to open embedded SKILL.md file: %w", err)
	}
	defer file.Close()

	tempFile, err := os.CreateTemp(filepath.Dir(path), fmt.Sprintf("SKILL-%s-*", version.Name))
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	_, err = io.Copy(tempFile, file)
	if err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}
	err = tempFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	err = tempFile.Close()
	if err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), path); err != nil {
		return fmt.Errorf("failed to install to target path: %w", err)
	}
	return nil
}

// InstallSkill installs the tfctl skill for the agent, either to the project directory or the
// global config directory based on the value of the global parameter.
func (a AgentSpec) InstallSkill(global bool) error {
	targetPath, err := a.skillFilePath(global)
	if err != nil {
		return err
	}
	return a.installSkillToPath(targetPath)
}
