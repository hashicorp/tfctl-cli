// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func jsonapi(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	_ = json.NewEncoder(w).Encode(payload)
}

func testAPI(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := New(context.Background(), server.URL, "test-token", nil)
	require.NoError(t, err)
	return c
}

func noopStatus(status string) {}

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

func TestPollRunUntilTerminated_Errored(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	c := testAPI(t, runGetResponder("run-x", "planning", "errored"))

	status, outcome, err := PollRunUntilTerminated(context.Background(), c, "run-x", io, time.Millisecond, noopStatus)
	require.NoError(t, err)
	assert.Equal(t, "errored", status)
	assert.Equal(t, runFailed, outcome)
}

func TestPollRunUntilTerminated_Status(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	c := testAPI(t, runGetResponder("run-x", "planning", "errored"))

	sawPlanning := false
	sawErrored := false

	status, outcome, err := PollRunUntilTerminated(context.Background(), c, "run-x", io, time.Millisecond, func(s string) {
		if s == "planning" {
			sawPlanning = true
		}
		if s == "errored" {
			sawErrored = true
		}
	})
	require.NoError(t, err)
	assert.Equal(t, "errored", status)
	assert.Equal(t, runFailed, outcome)
	assert.True(t, sawPlanning, "expected to see planning status")
	assert.True(t, sawErrored, "expected to see errored status")
}

func TestPollRunUntilTerminated_AwaitingConfirm(t *testing.T) {
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

	status, outcome, err := PollRunUntilTerminated(context.Background(), c, "run-x", io, time.Millisecond, noopStatus)
	require.NoError(t, err)
	assert.Equal(t, "planned", status)
	assert.Equal(t, runAwaitingConfirm, outcome)
}

func TestPollRunUntilTerminated_Timeout(t *testing.T) {

	t.Parallel()
	io := iostreams.Test()

	// Never settles.
	c := testAPI(t, runGetResponder("run-x", "planning"))

	ctx, cancel := context.WithTimeoutCause(context.Background(), 10*time.Millisecond, errors.New("timed out!"))
	defer cancel()

	_, outcome, err := PollRunUntilTerminated(ctx, c, "run-x", io, time.Millisecond, noopStatus)
	require.Error(t, err)
	assert.Equal(t, "timed out!", context.Cause(ctx).Error())
	assert.Equal(t, runInProgress, outcome)
}
