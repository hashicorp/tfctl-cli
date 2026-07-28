// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package skills

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func resetMigrationState(t *testing.T) {
	t.Helper()

	migrationBegun = false
	migrationResult = make(chan *MigrationResult, 10)
	t.Cleanup(func() {
		migrationBegun = false
		migrationResult = make(chan *MigrationResult, 10)
	})
}

func setupMigrationAgents(t *testing.T, names ...string) {
	t.Helper()

	oldAgents := agents
	oldAgentNames := AgentNames
	agents = registerAgents()
	AgentNames = names
	t.Cleanup(func() {
		agents = oldAgents
		AgentNames = oldAgentNames
	})
}

func writeSkill(t *testing.T, path string, contents []byte) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, contents, 0644))
}

func oldSkillContents(t *testing.T) []byte {
	t.Helper()

	_, sourceFile, _, _ := runtime.Caller(0)
	fixturesDir := filepath.Join(filepath.Dir(sourceFile), "fixtures")

	contents, err := os.ReadFile(filepath.Join(fixturesDir, "0_3_0.md"))
	require.NoError(t, err)
	return contents
}

func embeddedSkillContents(t *testing.T) []byte {
	t.Helper()

	contents, err := FS.ReadFile(TFCTLSkillPath)
	require.NoError(t, err)
	return contents
}

func runMigration(t *testing.T) []MigrationResult {
	t.Helper()

	MigrateInstalled(context.Background())
	return WaitForMigration()
}

func TestMigrateInstalled_MultipleMigrations(t *testing.T) {
	resetMigrationState(t)
	setupMigrationAgents(t, "bob", "pi", "codex")
	tmpDir := t.TempDir()
	oldSkill := oldSkillContents(t)

	t.Chdir(tmpDir)

	bobPath := filepath.Join(".bob", "skills", TFCTLSkillPath)
	piPath := filepath.Join(".agents", "skills", TFCTLSkillPath)
	codexPath := filepath.Join(".codex", "skills", TFCTLSkillPath)

	writeSkill(t, bobPath, oldSkill)
	writeSkill(t, piPath, oldSkill)
	writeSkill(t, codexPath, []byte("Not a known skill"))

	results := runMigration(t)

	require.Len(t, results, 3)
	absBobPath, err := filepath.Abs(bobPath)
	require.NoError(t, err)
	absPiPath, err := filepath.Abs(piPath)
	require.NoError(t, err)
	require.Equal(t, absBobPath, results[0].SkillPath)
	require.Equal(t, absPiPath, results[1].SkillPath)
	absCodexPath, err := filepath.Abs(codexPath)
	require.NoError(t, err)
	require.Equal(t, absCodexPath, results[2].SkillPath)
	require.Equal(t, "v0.3.0", results[0].PreviousVersion)
	require.Equal(t, "v0.3.0", results[1].PreviousVersion)
	require.NoError(t, results[0].FailedReason)
	require.NoError(t, results[1].FailedReason)
	require.ErrorContains(t, results[2].FailedReason, "does not match any known skill")
}

func TestMigrateInstalled_SkillFileIsSymlink(t *testing.T) {
	resetMigrationState(t)
	setupMigrationAgents(t, "pi")
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	linkPath := filepath.Join(".agents", "skills", TFCTLSkillPath)
	targetPath := filepath.Join(tmpDir, "target", "SKILL.md")
	writeSkill(t, targetPath, oldSkillContents(t))
	require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0755))
	require.NoError(t, os.Symlink(targetPath, linkPath))

	results := runMigration(t)
	require.Len(t, results, 1)
	expectedTarget, err := filepath.EvalSymlinks(targetPath)
	require.NoError(t, err)
	require.Equal(t, expectedTarget, results[0].SkillPath)
	require.Equal(t, "v0.3.0", results[0].PreviousVersion)
	require.NoError(t, results[0].FailedReason)

	// Ensure contents were migrated
	linkInfo, err := os.Lstat(linkPath)
	require.NoError(t, err)
	require.True(t, linkInfo.Mode()&os.ModeSymlink != 0)
	actual, err := os.ReadFile(linkPath)
	require.NoError(t, err)
	require.Equal(t, embeddedSkillContents(t), actual)
}

func TestMigrateInstalled_SkillFileIsBrokenSymlink(t *testing.T) {
	resetMigrationState(t)
	setupMigrationAgents(t, "pi")
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	linkPath := filepath.Join(".agents", "skills", TFCTLSkillPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0755))
	require.NoError(t, os.Symlink(filepath.Join(tmpDir, "missing", "SKILL.md"), linkPath))

	results := runMigration(t)
	require.Empty(t, results)
	_, err := os.Lstat(linkPath)
	require.NoError(t, err)
}

func TestMigrateInstalled_MultipleCalls(t *testing.T) {
	resetMigrationState(t)
	setupMigrationAgents(t, "pi")
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	path := filepath.Join(".agents", "skills", TFCTLSkillPath)
	writeSkill(t, path, oldSkillContents(t))

	firstResults := runMigration(t)
	require.Len(t, firstResults, 1)

	MigrateInstalled(context.Background())
	require.Empty(t, WaitForMigration())
}
