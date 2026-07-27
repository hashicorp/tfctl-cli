// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package versioncmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/checkpoint"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/skills"
	"github.com/hashicorp/tfctl-cli/version"
)

// seedCheckpoint pre-fills the checkpoint channel with nil (disabled) so that
// WaitForVersionCheck returns immediately. Tests that call runVersion or
// runDetectOutdatedVersion must call seedCheckpoint first.
func seedCheckpoint(t *testing.T) {
	t.Helper()
	checkpoint.Run(context.Background(), true)
}

func TestRunVersion_NoToken_PrintsVersionAndAuthHint(t *testing.T) {
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: false}

	runVersion(context.Background(), opts)

	errOut := ios.Error.String()
	if !strings.Contains(errOut, version.Version) {
		t.Errorf("expected version %q in error output, got:\n%s", version.Version, errOut)
	}
	if !strings.Contains(errOut, "auth login") {
		t.Errorf("expected 'auth login' hint in error output, got:\n%s", errOut)
	}
	if !strings.Contains(errOut, version.Name) {
		t.Errorf("expected CLI name %q in error output, got:\n%s", version.Name, errOut)
	}
}

func TestRunVersion_NoToken_VersionOnFirstLine(t *testing.T) {
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: false}

	runVersion(context.Background(), opts)

	// With Testing IOStreams, ColorEnabled() always returns false, so the non-TTY
	// path runs: version.Version is written to Err() as the very first line.
	errOut := ios.Error.String()
	lines := strings.Split(strings.TrimSpace(errOut), "\n")
	if lines[0] != version.Version {
		t.Errorf("expected first line of error output to be %q, got %q", version.Version, lines[0])
	}
}

func TestRunVersion_TokenConfigured_NoAuthHint(t *testing.T) {
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: true}

	runVersion(context.Background(), opts)

	errOut := ios.Error.String()
	if strings.Contains(errOut, "auth login") {
		t.Errorf("expected no 'auth login' hint when token is configured, got:\n%s", errOut)
	}
	if !strings.Contains(errOut, version.Version) {
		t.Errorf("expected version %q in error output, got:\n%s", version.Version, errOut)
	}
}

func TestRunVersion_TokenConfigured_NoSkillInstalled_NoSkillWarning(t *testing.T) {
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: true}

	runVersion(context.Background(), opts)

	errOut := ios.Error.String()
	if strings.Contains(errOut, "reinstall") || strings.Contains(errOut, "harness install") {
		t.Errorf("expected no skill reinstall warning when no skill is installed, got:\n%s", errOut)
	}
}

func TestRunVersion_NoOutput_ToStdout(t *testing.T) {
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: false}

	runVersion(context.Background(), opts)

	if ios.Output.Len() != 0 {
		t.Errorf("expected no output on stdout, got:\n%s", ios.Output.String())
	}
}

func TestRunVersion_Quiet_VersionStillPrinted(t *testing.T) {
	seedCheckpoint(t)

	ios := iostreams.Test()
	ios.SetQuiet(true)
	opts := &VersionOpts{IO: ios, TokenConfigured: false}

	runVersion(context.Background(), opts)

	// Err() is always written (not suppressed by quiet mode) — version must appear.
	errOut := ios.Error.String()
	if !strings.Contains(errOut, version.Version) {
		t.Errorf("expected version %q in error output even in quiet mode, got:\n%s", version.Version, errOut)
	}
}

func TestRunVersion_Quiet_LogoSuppressed(t *testing.T) {
	seedCheckpoint(t)

	ios := iostreams.Test()
	ios.SetQuiet(true)
	opts := &VersionOpts{IO: ios, TokenConfigured: false}

	runVersion(context.Background(), opts)

	// The logo is written to ErrUnessential(), which is discarded in quiet mode.
	// Logo lines are each prefixed with two spaces. Count them to confirm suppression.
	errOut := ios.Error.String()
	for _, line := range strings.Split(errOut, "\n") {
		if strings.HasPrefix(line, "  /") || strings.HasPrefix(line, "  \\") {
			t.Errorf("expected logo to be suppressed in quiet mode, got indented line:\n%s", line)
		}
	}
}

// installSkill writes content into the bob agent's project-relative skill path
// (.bob/skills/tfctl/SKILL.md) under dir, which must be the working directory
// when runVersion is called so that DetectAnyExistingSkill can locate it.
func installSkill(t *testing.T, dir string, content []byte) {
	t.Helper()
	skillDir := filepath.Join(dir, ".bob", "skills", "tfctl")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("installSkill: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0644); err != nil {
		t.Fatalf("installSkill: write: %v", err)
	}
}

// TestRunVersion_TokenConfigured_NoSkillOnDisk_NoSkillWarning verifies that
// runVersion emits no reinstall warning when no skill file exists anywhere on
// disk that DetectAnyExistingSkill would find.
func TestRunVersion_TokenConfigured_NoSkillOnDisk_NoSkillWarning(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: true}

	runVersion(context.Background(), opts)

	errOut := ios.Error.String()
	if strings.Contains(errOut, "reinstall") || strings.Contains(errOut, "harness install") {
		t.Errorf("expected no skill reinstall warning when no skill is on disk, got:\n%s", errOut)
	}
}

// TestRunVersion_TokenConfigured_UnknownHashSkill_NoSkillWarning verifies
// that runVersion emits no reinstall warning when an installed skill has a
// hash not present in the known-versions hashes file.
func TestRunVersion_TokenConfigured_UnknownHashSkill_NoSkillWarning(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	installSkill(t, tmpDir, []byte("# Unknown old skill content"))
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: true}

	runVersion(context.Background(), opts)

	errOut := ios.Error.String()
	if strings.Contains(errOut, "reinstall") || strings.Contains(errOut, "harness install") {
		t.Errorf("expected no reinstall warning for unrecognized skill hash, got:\n%s", errOut)
	}
}

// TestRunVersion_TokenConfigured_CurrentVersionSkill_NoSkillWarning verifies
// that runVersion emits no reinstall warning when the installed skill matches
// the current embedded hash.
func TestRunVersion_TokenConfigured_CurrentVersionSkill_NoSkillWarning(t *testing.T) {
	embedded, err := skills.FS.Open(skills.TFCTLSkillPath)
	if err != nil {
		t.Fatalf("failed to open embedded skill: %v", err)
	}
	embeddedBytes, readErr := io.ReadAll(embedded)
	embedded.Close()
	if readErr != nil {
		t.Fatalf("failed to read embedded skill: %v", readErr)
	}

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	installSkill(t, tmpDir, embeddedBytes)

	// Confirm the skill is found and recognized before proceeding.
	bobAgent, ok := skills.GetAgent("bob")
	if !ok {
		t.Fatal("bob agent not registered")
	}
	installed := bobAgent.DetectExistingSkill()
	if installed == nil {
		t.Fatal("expected DetectExistingSkill to find the test skill")
	}
	match, known := installed.MatchesKnownVersion()
	if !known {
		t.Skip("embedded skill not in hashes file — cannot verify current-version path")
	}
	if match.Hash != skills.EmbeddedSkillHash() {
		t.Skip("embedded hash mismatch — skipping current-version assertion")
	}

	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: true}

	runVersion(context.Background(), opts)

	errOut := ios.Error.String()
	if strings.Contains(errOut, "reinstall") || strings.Contains(errOut, "harness install") {
		t.Errorf("expected no reinstall warning for current-version skill, got:\n%s", errOut)
	}
}

func TestRunVersion_TokenConfigured_OutdatedSkill_PrintsReinstallWarning(t *testing.T) {
	fixtureBytes, err := os.ReadFile("../../../skills/fixtures/0_3_0.md")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	installSkill(t, tmpDir, fixtureBytes)
	seedCheckpoint(t)

	ios := iostreams.Test()
	opts := &VersionOpts{IO: ios, TokenConfigured: true}

	runVersion(context.Background(), opts)

	errOut := ios.Error.String()
	if !strings.Contains(errOut, "re-install") && !strings.Contains(errOut, "harness install bob") {
		t.Errorf("expected re-install warning for outdated skill, got:\n%s", errOut)
	}
}
