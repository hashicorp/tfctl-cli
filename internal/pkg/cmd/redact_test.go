// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/internal/pkg/redact"
)

func newRedactionInvocation(t *testing.T, io iostreams.IOStreams) *Invocation {
	t.Helper()

	inv := &Invocation{
		IO:          io,
		Output:      format.New(io),
		ShutdownCtx: context.Background(),
		Profile:     &profile.Profile{},
	}
	inv.flags.parsed = true
	return inv
}

func TestApplyRedaction(t *testing.T) {
	tests := []struct {
		name          string
		noRedact      bool
		env           string
		profileRedact string
		want          redact.Mode
	}{
		{name: "masking is on by default", want: redact.ModeStrict},
		{name: "--no-redact turns masking off", noRedact: true, want: redact.ModeOff},
		{name: "the environment selects a mode", env: "known", want: redact.ModeKnown},
		{name: "the profile selects a mode", profileRedact: "off", want: redact.ModeOff},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(redact.EnvRedact, tc.env)

			inv := newRedactionInvocation(t, iostreams.Test())
			inv.flags.noRedact = tc.noRedact
			if tc.profileRedact != "" {
				inv.Profile.Redact = &tc.profileRedact
			}

			if err := inv.applyRedaction(); err != nil {
				t.Fatalf("applyRedaction() error = %v", err)
			}

			if got := inv.Output.Redactor().Mode(); got != tc.want {
				t.Errorf("mode = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyRedaction_InvalidValueFallsBackToStrict(t *testing.T) {
	// A bad setting must not stop the command, and it must not quietly turn
	// masking off.
	t.Setenv(redact.EnvRedact, "banana")

	io := iostreams.Test()
	inv := newRedactionInvocation(t, io)

	if err := inv.applyRedaction(); err != nil {
		t.Fatalf("applyRedaction() error = %v, want the command to continue", err)
	}

	if got := inv.Output.Redactor().Mode(); got != redact.ModeStrict {
		t.Errorf("mode = %v, want %v", got, redact.ModeStrict)
	}

	if !strings.Contains(io.Error.String(), `invalid redact value "banana"`) {
		t.Errorf("stderr = %q, want the invalid value reported", io.Error.String())
	}
}

func TestApplyRedaction_WarnsWhenMaskingIsOffAndOutputIsNotATerminal(t *testing.T) {
	t.Setenv(redact.EnvRedact, "")

	io := iostreams.Test()
	io.OutputTTY = false

	inv := newRedactionInvocation(t, io)
	inv.flags.noRedact = true

	if err := inv.applyRedaction(); err != nil {
		t.Fatalf("applyRedaction() error = %v", err)
	}

	if !strings.Contains(io.Error.String(), "redaction is off") {
		t.Errorf("stderr = %q, want a warning that redaction is off", io.Error.String())
	}
}

func TestApplyRedaction_DoesNotWarnOnATerminal(t *testing.T) {
	t.Setenv(redact.EnvRedact, "")

	io := iostreams.Test()
	io.OutputTTY = true

	inv := newRedactionInvocation(t, io)
	inv.flags.noRedact = true

	if err := inv.applyRedaction(); err != nil {
		t.Fatalf("applyRedaction() error = %v", err)
	}

	if io.Error.String() != "" {
		t.Errorf("stderr = %q, want no warning when the user is watching the output", io.Error.String())
	}
}
