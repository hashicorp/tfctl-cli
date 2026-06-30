// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/execsession"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

// hclFiles returns the names of *.hcl files in dir.
func hclFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.hcl"))
	require.NoError(t, err)
	return matches
}

func TestRunExec_DryRunCreatesNothing(t *testing.T) {
	t.Parallel()

	ios := iostreams.Test()
	store := &execsession.Store{Dir: t.TempDir()}

	ran := false
	opts := &ExecOpts{
		IO:          ios,
		DryRun:      true,
		AllowDelete: []string{"workspaces"},
		Argv:        []string{"echo", "hi"},
		Store:       store,
		PID:         os.Getpid(),
		Run: func(_ context.Context, _, _ []string, _ iostreams.IOStreams) (int, error) {
			ran = true
			return 0, nil
		},
	}

	err := runExec(context.Background(), opts)
	require.NoError(t, err)

	assert.False(t, ran, "child must not run in dry-run")
	assert.Empty(t, hclFiles(t, store.Dir), "no session file should be written in dry-run")
	assert.Contains(t, ios.Error.String(), "would create exec session")
}

func TestRunExec_HappyPathInjectsTokenAndCleansUp(t *testing.T) {
	t.Parallel()

	ios := iostreams.Test()
	store := &execsession.Store{Dir: t.TempDir()}

	var gotArgv []string
	var gotToken string
	var fileExistedDuringRun bool

	opts := &ExecOpts{
		IO:          ios,
		AllowDelete: []string{"workspaces", "runs"},
		Argv:        []string{"mychild", "--flag", "val"},
		Store:       store,
		PID:         os.Getpid(),
		Run: func(_ context.Context, argv, env []string, _ iostreams.IOStreams) (int, error) {
			gotArgv = argv
			for _, kv := range env {
				if strings.HasPrefix(kv, execsession.EnvVar+"=") {
					gotToken = strings.TrimPrefix(kv, execsession.EnvVar+"=")
				}
			}
			// The session file must exist while the child runs.
			if gotToken != "" {
				if _, err := os.Stat(filepath.Join(store.Dir, gotToken+".hcl")); err == nil {
					fileExistedDuringRun = true
				}
			}
			return 0, nil
		},
	}

	err := runExec(context.Background(), opts)
	require.NoError(t, err)

	assert.Equal(t, []string{"mychild", "--flag", "val"}, gotArgv)
	assert.NotEmpty(t, gotToken, "TFCTL_EXEC_SESSION must be set in child env")
	assert.True(t, fileExistedDuringRun, "session file must exist during child run")
	assert.Empty(t, hclFiles(t, store.Dir), "session file must be removed after runExec returns")
}

func TestRunExec_ChildExitCodePropagates(t *testing.T) {
	t.Parallel()

	ios := iostreams.Test()
	store := &execsession.Store{Dir: t.TempDir()}

	opts := &ExecOpts{
		IO:          ios,
		AllowDelete: []string{"workspaces"},
		Argv:        []string{"child"},
		Store:       store,
		PID:         os.Getpid(),
		Run: func(_ context.Context, _, _ []string, _ iostreams.IOStreams) (int, error) {
			return 7, nil
		},
	}

	err := runExec(context.Background(), opts)
	require.Error(t, err)

	var exitErr *cmd.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
	// Even on a non-zero child exit, the session file is cleaned up.
	assert.Empty(t, hclFiles(t, store.Dir))
}

func TestRunExec_EmptyArgvIsUsageError(t *testing.T) {
	t.Parallel()

	ios := iostreams.Test()
	store := &execsession.Store{Dir: t.TempDir()}

	opts := &ExecOpts{
		IO:    ios,
		Argv:  nil,
		Store: store,
		PID:   os.Getpid(),
		Run: func(_ context.Context, _, _ []string, _ iostreams.IOStreams) (int, error) {
			return 0, nil
		},
	}

	err := runExec(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "harness exec")
}

func TestRunExec_UnknownClassWarnsButRuns(t *testing.T) {
	t.Parallel()

	ios := iostreams.Test()
	store := &execsession.Store{Dir: t.TempDir()}

	ran := false
	opts := &ExecOpts{
		IO:          ios,
		AllowDelete: []string{"bogusclass"},
		Argv:        []string{"child"},
		Store:       store,
		PID:         os.Getpid(),
		Run: func(_ context.Context, _, _ []string, _ iostreams.IOStreams) (int, error) {
			ran = true
			return 0, nil
		},
	}

	err := runExec(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, ran, "child should still run despite unknown class")
	assert.Contains(t, ios.Error.String(), "bogusclass")
}
