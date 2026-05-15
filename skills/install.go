package skills

import (
	"fmt"
	"io"
	"os"
	"path"
	"slices"

	"github.com/mitchellh/go-homedir"
)

type AgentSpec struct {
	Name            string
	DisplayName     string
	SkillsDir       string
	GlobalSkillsDir func() string
	Detect          func() bool
}

func detectHomeDirPath(dir string) bool {
	path, err := homedir.Expand(fmt.Sprintf("~/%s", dir))
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

const TFCTLSkillPath = "tfctl/SKILL.md"

var agents map[string]AgentSpec
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
		},
		"claude": {
			Name:        "claude",
			DisplayName: "Claude Code",
			SkillsDir:   ".claude/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.claude/skills")
				return path
			},
			Detect: func() bool {
				_, err := os.Stat(claudeDir)
				return err == nil
			},
		},
		"codex": {
			Name:        "codex",
			DisplayName: "OpenAI Codex",
			SkillsDir:   ".codex/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.codex/skills")
				return path
			},
			Detect: func() bool {
				_, err := os.Stat(codexDir)
				return err == nil
			},
		},
		"gemini": {
			Name:        "gemini",
			DisplayName: "Gemini CLI",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.gemini/skills")
				return path
			},
			Detect: func() bool {
				return detectHomeDirPath(".gemini")
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
		},
		"pi": {
			Name:        "pi",
			DisplayName: "Pi CLI",
			SkillsDir:   ".agents/skills",
			GlobalSkillsDir: func() string {
				path, _ := homedir.Expand("~/.pi/skills")
				return path
			},
			Detect: func() bool {
				return detectHomeDirPath(".pi")
			},
		},
	}
}

func ListAgents() []string {
	var list []string
	for _, agent := range agents {
		list = append(list, agent.Name)
	}
	return list
}

func GetAgent(name string) (AgentSpec, bool) {
	agent, ok := agents[name]
	return agent, ok
}

func DetectAgent() []AgentSpec {
	var detected []AgentSpec
	for _, agent := range agents {
		if agent.Detect() {
			detected = append(detected, agent)
		}
	}

	return detected
}

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

	targetDir = path.Join(targetDir, "tfctl")

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
