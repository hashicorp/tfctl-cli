// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package versioncmd

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/checkpoint"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
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
