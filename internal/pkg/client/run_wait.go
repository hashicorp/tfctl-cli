// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"time"

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

// defaultPollInterval is how often run is polled when no interval is set.
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

// PollRunUntilTerminated polls the run indefinitely until it reaches a settled state
// (finished, failed, or awaiting manual confirmation), notifying on each status transition.
// It returns the final status string and its classified outcome.
func PollRunUntilTerminated(ctx context.Context, c *Client, runID string, io iostreams.IOStreams, interval time.Duration, statusUpdate func(string)) (string, runOutcome, error) {
	if interval <= 0 {
		interval = defaultPollInterval
	}

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
			if statusUpdate != nil {
				statusUpdate(status)
			}
			last = status
		}

		if outcome := classifyRunStatus(status, confirmable); outcome != runInProgress {
			return status, outcome, nil
		}

		select {
		case <-ctx.Done():
			return status, runInProgress, ctx.Err()
		case <-time.After(interval):
		}
	}
}
