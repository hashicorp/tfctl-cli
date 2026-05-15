package harness

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/skills"
)

func TestRunContext(t *testing.T) {
	t.Run("outputs skill content without frontmatter", func(t *testing.T) {
		ios := iostreams.Test()
		output := format.New(ios)

		opts := &ContextOpts{
			IO:     ios,
			Output: output,
		}

		err := runContext(opts)
		if err != nil {
			t.Fatalf("runContext returned error: %v", err)
		}

		out := ios.Output.String()

		// Should contain actual skill content
		if !strings.Contains(out, "tfctl") {
			t.Errorf("expected output to contain 'tfctl', got %q", out)
		}

		// First non-empty line should not be a frontmatter delimiter
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			if line == "---" {
				t.Error("first non-empty line of output is a frontmatter delimiter '---'")
			}
			break
		}

		// Should not contain frontmatter fields
		if strings.Contains(out, "name: tfctl") {
			t.Error("output contains frontmatter field 'name: tfctl'")
		}
		if strings.Contains(out, "description: |") {
			t.Error("output contains frontmatter field 'description: |'")
		}
	})

	t.Run("outputs as markdown format by default", func(t *testing.T) {
		ios := iostreams.Test()
		output := format.New(ios)

		opts := &ContextOpts{
			IO:     ios,
			Output: output,
		}

		err := runContext(opts)
		if err != nil {
			t.Fatalf("runContext returned error: %v", err)
		}

		out := ios.Output.String()
		// Should contain markdown headings from the skill file
		if !strings.Contains(out, "#") {
			t.Error("expected output to contain markdown headings")
		}
	})

	t.Run("outputs JSON when format is set", func(t *testing.T) {
		ios := iostreams.Test()
		output := format.New(ios)
		output.SetFormat(format.JSON)

		opts := &ContextOpts{
			IO:     ios,
			Output: output,
		}

		err := runContext(opts)
		if err != nil {
			t.Fatalf("runContext returned error: %v", err)
		}

		out := ios.Output.String()
		if !strings.Contains(out, `"content"`) {
			t.Errorf("expected JSON output to contain 'content' key, got %q", out[:min(len(out), 200)])
		}
	})
}

func TestRunInstall(t *testing.T) {
	t.Run("dry run project install", func(t *testing.T) {
		ios := iostreams.Test()
		output := format.New(ios)

		tmpDir := t.TempDir()

		agent := skills.AgentSpec{
			Name:        "testagent",
			DisplayName: "Test Agent",
			SkillsDir:   ".testagent/skills",
			GlobalSkillsDir: func() string {
				return path.Join(tmpDir, "global", "testagent", "skills")
			},
		}

		opts := &InstallOpts{
			IO:     ios,
			Output: output,
			Agent:  &agent,
			Global: false,
			DryRun: true,
		}

		err := runInstall(opts)
		if err != nil {
			t.Fatalf("runInstall returned error: %v", err)
		}

		errOut := ios.Error.String()
		if !strings.Contains(errOut, "Would create skill") {
			t.Errorf("expected dry-run message, got %q", errOut)
		}
		if !strings.Contains(errOut, "Test Agent") {
			t.Errorf("expected agent display name in output, got %q", errOut)
		}
		if !strings.Contains(errOut, "project directory") {
			t.Errorf("expected 'project directory' in output, got %q", errOut)
		}
		if !strings.Contains(errOut, ".testagent/skills") {
			t.Errorf("expected skills dir in output, got %q", errOut)
		}
	})

	t.Run("dry run global install", func(t *testing.T) {
		ios := iostreams.Test()
		output := format.New(ios)

		tmpDir := t.TempDir()
		agent := skills.AgentSpec{
			Name:        "testagent",
			DisplayName: "Test Agent",
			SkillsDir:   ".testagent/skills",
			GlobalSkillsDir: func() string {
				return path.Join(tmpDir, "global", "testagent", "skills")
			},
		}

		opts := &InstallOpts{
			IO:     ios,
			Output: output,
			Agent:  &agent,
			Global: true,
			DryRun: true,
		}

		err := runInstall(opts)
		if err != nil {
			t.Fatalf("runInstall returned error: %v", err)
		}

		errOut := ios.Error.String()
		if !strings.Contains(errOut, "Would create skill") {
			t.Errorf("expected dry-run message, got %q", errOut)
		}
		if !strings.Contains(errOut, "global directory") {
			t.Errorf("expected 'global directory' in output, got %q", errOut)
		}
		if !strings.Contains(errOut, path.Join(tmpDir, "global", "testagent", "skills")) {
			t.Errorf("expected global skills dir in output, got %q", errOut)
		}
	})

	t.Run("actual install to project directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		skillsDir := filepath.Join(tmpDir, ".testagent", "skills")

		ios := iostreams.Test()
		output := format.New(ios)

		agent := skills.AgentSpec{
			Name:        "testagent",
			DisplayName: "Test Agent",
			SkillsDir:   skillsDir,
			GlobalSkillsDir: func() string {
				return filepath.Join(tmpDir, "global")
			},
		}

		opts := &InstallOpts{
			IO:     ios,
			Output: output,
			Agent:  &agent,
			Global: false,
			DryRun: false,
		}

		err := runInstall(opts)
		if err != nil {
			t.Fatalf("runInstall returned error: %v", err)
		}

		// Verify the file was created
		installedPath := filepath.Join(skillsDir, skills.TFCTLSkillPath)
		info, err := os.Stat(installedPath)
		if err != nil {
			t.Fatalf("expected installed file at %s, got error: %v", installedPath, err)
		}
		if info.Size() == 0 {
			t.Error("installed file is empty")
		}

		// Verify success message
		errOut := ios.Error.String()
		if !strings.Contains(errOut, "Successfully installed") {
			t.Errorf("expected success message, got %q", errOut)
		}
		if !strings.Contains(errOut, "Test Agent") {
			t.Errorf("expected agent display name in output, got %q", errOut)
		}
	})

	t.Run("actual install to global directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		globalDir := filepath.Join(tmpDir, "global", "skills")

		ios := iostreams.Test()
		output := format.New(ios)

		agent := skills.AgentSpec{
			Name:        "testagent",
			DisplayName: "Test Agent",
			SkillsDir:   filepath.Join(tmpDir, "project"),
			GlobalSkillsDir: func() string {
				return globalDir
			},
		}

		opts := &InstallOpts{
			IO:     ios,
			Output: output,
			Agent:  &agent,
			Global: true,
			DryRun: false,
		}

		err := runInstall(opts)
		if err != nil {
			t.Fatalf("runInstall returned error: %v", err)
		}

		// Verify the file was created in the global directory
		installedPath := filepath.Join(globalDir, skills.TFCTLSkillPath)
		info, err := os.Stat(installedPath)
		if err != nil {
			t.Fatalf("expected installed file at %s, got error: %v", installedPath, err)
		}
		if info.Size() == 0 {
			t.Error("installed file is empty")
		}

		// Verify success message mentions global
		errOut := ios.Error.String()
		if !strings.Contains(errOut, "Successfully installed") {
			t.Errorf("expected success message, got %q", errOut)
		}
		if !strings.Contains(errOut, "global directory") {
			t.Errorf("expected 'global directory' in output, got %q", errOut)
		}
	})
}
