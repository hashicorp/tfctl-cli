// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestClassifyRunStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status      string
		confirmable bool
		want        runOutcome
	}{
		{"applied", false, runSucceeded},
		{"planned_and_finished", false, runSucceeded},
		{"planned_and_saved", false, runSucceeded},
		{"errored", false, runFailed},
		{"canceled", false, runFailed},
		{"discarded", false, runFailed},
		{"policy_soft_failed", false, runFailed},
		{"policy_override", false, runFailed},
		{"planning", false, runInProgress},
		{"applying", false, runInProgress},
		{"pending", false, runInProgress},
		// A confirmable plan is done but needs a manual apply; not in-progress.
		{"planned", true, runAwaitingConfirm},
		// Confirmable must not override a terminal failure state.
		{"errored", true, runFailed},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, classifyRunStatus(tc.status, tc.confirmable),
			"status=%q confirmable=%v", tc.status, tc.confirmable)
	}
}

// runGetResponder returns a handler for GET /api/v2/runs/{id} that emits the
// given statuses in order, repeating the last one for any further calls.
func runGetResponder(runID string, statuses ...string) http.HandlerFunc {
	var n int32
	return func(w http.ResponseWriter, _ *http.Request) {
		i := int(atomic.AddInt32(&n, 1)) - 1
		if i >= len(statuses) {
			i = len(statuses) - 1
		}
		jsonapi(w, map[string]any{
			"data": map[string]any{
				"id": runID, "type": "runs",
				"attributes": map[string]any{"status": statuses[i]},
			},
		})
	}
}

func TestPollRunUntilSettled_Errored(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	c := testAPI(t, runGetResponder("run-x", "planning", "errored"))

	status, outcome, err := pollRunUntilSettled(context.Background(), c, "run-x", io, time.Millisecond, 0)
	require.NoError(t, err)
	assert.Equal(t, "errored", status)
	assert.Equal(t, runFailed, outcome)
	// Transitions are streamed to stderr.
	assert.Contains(t, io.Error.String(), "planning")
	assert.Contains(t, io.Error.String(), "errored")
}

func TestPollRunUntilSettled_AwaitingConfirm(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonapi(w, map[string]any{
			"data": map[string]any{
				"id": "run-x", "type": "runs",
				"attributes": map[string]any{
					"status":  "planned",
					"actions": map[string]any{"is-confirmable": true},
				},
			},
		})
	}))

	status, outcome, err := pollRunUntilSettled(context.Background(), c, "run-x", io, time.Millisecond, 0)
	require.NoError(t, err)
	assert.Equal(t, "planned", status)
	assert.Equal(t, runAwaitingConfirm, outcome)
}

func TestPollRunUntilSettled_Timeout(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	// Never settles.
	c := testAPI(t, runGetResponder("run-x", "planning"))

	_, outcome, err := pollRunUntilSettled(context.Background(), c, "run-x", io, time.Millisecond, 10*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Equal(t, runInProgress, outcome)
}

func TestRunStart_Wait_Success(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	var runGets int32
	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-waited", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-waited":
			status := "planning"
			if atomic.AddInt32(&runGets, 1) > 1 {
				status = "planned_and_finished"
			}
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-waited", "type": "runs",
					"attributes": map[string]any{"status": status},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
	}, CreateOpts{})

	require.NoError(t, err)
	assert.Contains(t, io.Error.String(), "waiting for it to finish")
	assert.Contains(t, io.Error.String(), "planned_and_finished")
	// The final run summary is rendered to stdout, same as `run status`.
	assert.Contains(t, io.Output.String(), "Plan complete, no apply needed")
	// Elapsed time and the run URL are always surfaced on completion.
	assert.Contains(t, io.Error.String(), "Completed in")
	assert.Contains(t, io.Error.String(), "View the run at")
	assert.Contains(t, io.Error.String(), "workspaces/foobar/runs/run-waited")
}

func TestRunStart_Wait_Timeout(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	// The run never settles; --wait must give up and still surface the URL.
	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-slow", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-slow":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-slow", "type": "runs",
					"attributes": map[string]any{"status": "planning"},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
		Timeout:      10 * time.Millisecond,
	}, CreateOpts{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Contains(t, io.Error.String(), "still be running")
	assert.Contains(t, io.Error.String(), "workspaces/foobar/runs/run-slow")
}

func TestRunStart_Wait_AwaitingConfirm(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-confirm", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-confirm":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-confirm", "type": "runs",
					"attributes": map[string]any{
						"status":  "planned",
						"actions": map[string]any{"is-confirmable": true},
					},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
	}, CreateOpts{})

	require.NoError(t, err)
	// The generic "Run status: planned" line is replaced with actionable text,
	// and the apply URL is surfaced.
	assert.Contains(t, io.Output.String(), "manual apply is required")
	assert.NotContains(t, io.Output.String(), "Run status: planned")
	assert.Contains(t, io.Error.String(), "Confirm the apply at")
	assert.Contains(t, io.Error.String(), "workspaces/foobar/runs/run-confirm")
}

func TestRunStart_Wait_Failure(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	// canceled is a failure state that NewRunSummary renders without any extra
	// API calls, so it exercises the full wait-then-exit-code path cleanly.
	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-cancel", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-cancel":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-cancel", "type": "runs",
					"attributes": map[string]any{"status": "canceled"},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
	}, CreateOpts{})

	require.ErrorIs(t, err, cmd.ErrUnderlyingError)
	assert.Contains(t, io.Output.String(), "Run was canceled")
	assert.Contains(t, io.Error.String(), "Failed after")
	assert.Contains(t, io.Error.String(), "View the run at")
	assert.Contains(t, io.Error.String(), "workspaces/foobar/runs/run-cancel")
}
