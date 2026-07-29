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

func setupMigrationTest(t *testing.T, names ...string) {
	t.Helper()

	// Isolate tests from users' globally installed skills
	t.Setenv("HOME", t.TempDir())

	// Register just the named agents in the specified order
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

	ctx := context.Background()
	migration := StartMigration(ctx)
	results, err := migration.Wait(ctx)
	require.NoError(t, err)
	return results
}

func TestMigrateInstalled_MultipleMigrations(t *testing.T) {
	setupMigrationTest(t, "bob", "pi", "codex")
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

	require.Len(t, results, 2)
	absBobPath, err := filepath.Abs(bobPath)
	require.NoError(t, err)
	absPiPath, err := filepath.Abs(piPath)
	require.NoError(t, err)
	require.Equal(t, absBobPath, results[0].SkillPath)
	require.Equal(t, absPiPath, results[1].SkillPath)

	require.NoError(t, err)
	require.Equal(t, "v0.3.0", results[0].PreviousVersion)
	require.Equal(t, "v0.3.0", results[1].PreviousVersion)
	require.NoError(t, results[0].FailedReason)
	require.NoError(t, results[1].FailedReason)

	bobUpgradedContents, err := os.ReadFile(absBobPath)
	require.NoError(t, err)

	piUpgradedContents, err := os.ReadFile(absPiPath)
	require.NoError(t, err)

	require.Equal(t, embeddedSkillContents(t), bobUpgradedContents)
	require.Equal(t, embeddedSkillContents(t), piUpgradedContents)
}

func TestMigrateInstalled_SkillFileIsSymlink(t *testing.T) {
	setupMigrationTest(t, "pi")
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
	setupMigrationTest(t, "pi")
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
