// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

// runOutcome classifies a run's status for the purpose of `run start --wait`.
type runOutcome int

const (
	// runInProgress means the run is still transitioning and should be polled again.
	runInProgress runOutcome = iota
	// runSucceeded means the run reached a successful terminal state.
	runSucceeded
	// runAwaitingConfirm means the plan finished but a manual apply is required
	// (the workspace does not auto-apply). Nothing more happens without a human.
	runAwaitingConfirm
	// runFailed means the run reached a failed or aborted terminal state.
	runFailed
)

// defaultPollInterval is how often `--wait` polls the run when no interval is set.
const defaultPollInterval = 3 * time.Second

// classifyRunStatus maps a run status string, plus whether the run is awaiting a
// manual apply confirmation, to a wait outcome. Statuses not listed are treated
// as in-progress so the poller keeps waiting; auto-apply runs transition through
// planned/confirmed/applying on their own. A run that is confirmable has finished
// planning but will not proceed without a human, so we stop there rather than
// block forever on a non-auto-apply workspace.
func classifyRunStatus(status string, confirmable bool) runOutcome {
	switch status {
	case "applied", "planned_and_finished", "planned_and_saved":
		return runSucceeded
	case "errored", "canceled", "discarded", "policy_soft_failed", "policy_override":
		return runFailed
	}
	if confirmable {
		return runAwaitingConfirm
	}
	return runInProgress
}

// pollRunUntilSettled polls the run until it reaches a settled state (finished,
// failed, or awaiting manual confirmation), printing each status transition to
// stderr. It returns the final status string and its classified outcome. It
// stops early if ctx is canceled or the optional timeout elapses.
func pollRunUntilSettled(ctx context.Context, c *client.Client, runID string, io iostreams.IOStreams, interval, timeout time.Duration) (string, runOutcome, error) {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	cs := io.ColorScheme()

	last := ""
	for {
		resp, err := c.TFE.API.Runs().ById(runID).Get(ctx, nil)
		if err != nil {
			return "", runInProgress, fmt.Errorf("polling run %s: %w", runID, err)
		}
		attrs := resp.GetData().GetAttributes()
		if attrs == nil || attrs.GetStatus() == nil {
			return "", runInProgress, fmt.Errorf("run %s has no status", runID)
		}
		status := attrs.GetStatus().String()

		confirmable := false
		if a := attrs.GetActions(); a != nil && a.GetIsConfirmable() != nil {
			confirmable = *a.GetIsConfirmable()
		}

		if status != last {
			fmt.Fprintln(io.Err(), cs.String("  ⋯ "+status).Faint().String())
			last = status
		}

		if outcome := classifyRunStatus(status, confirmable); outcome != runInProgress {
			return status, outcome, nil
		}

		if !deadline.IsZero() && time.Now().After(deadline) {
			return status, runInProgress, fmt.Errorf("timed out after %s waiting for run %s (last status: %s)", timeout, runID, status)
		}

		select {
		case <-ctx.Done():
			return status, runInProgress, ctx.Err()
		case <-time.After(interval):
		}
	}
}
