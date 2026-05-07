// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/client"
)

func testAPI(t *testing.T, handler http.Handler) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := client.New(server.URL, "test-token", nil)
	require.NoError(t, err)
	return c
}

func jsonapi(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	_ = json.NewEncoder(w).Encode(payload)
}

func route(r *http.Request) string {
	return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
}

func TestGetRunSummary_Statuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status  string
		message string
	}{
		{"applied", "Run succeeded"},
		{"pending", "Plan in progress"},
		{"planning", "Plan in progress"},
		{"canceled", "Run was canceled"},
		{"discarded", "Run was discarded"},
		{"planned_and_finished", "Plan complete, no apply needed"},
		{"planned_and_saved", "Plan complete, no apply needed"},
		{"policy_override", "Run awaiting policy override"},
		{"policy_soft_failed", "Run has soft-failed policies"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-1", "type": "runs",
						"attributes": map[string]any{"status": tt.status},
					},
				})
			}))

			summary, err := client.GetRunSummary(context.Background(), c.TFE.API, "run-1")
			require.NoError(t, err)
			assert.Equal(t, tt.status, summary.Status)
			assert.Equal(t, tt.message, summary.Message)
			assert.Empty(t, summary.Diagnostics)
		})
	}
}

func TestGetRunSummary_ErroredPlan(t *testing.T) {
	t.Parallel()

	logServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Terraform v1.5.0\non linux_amd64\nInitializing plugins...\n"+
			`{"@level":"error","@message":"Error: Bad resource","type":"diagnostic","diagnostic":{"severity":"error","summary":"Bad resource","detail":"Resource not declared."}}`+"\n")
	}))
	t.Cleanup(logServer.Close)

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/runs/run-1":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "errored"},
				},
			})
		case "GET /api/v2/runs/run-1/plan":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "errored", "log-read-url": logServer.URL},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.GetRunSummary(context.Background(), c.TFE.API, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "errored", summary.Status)
	assert.Equal(t, "plan", summary.Phase)
	require.Len(t, summary.Diagnostics, 1)
	assert.Equal(t, "Bad resource", summary.Diagnostics[0].Summary)
}

func TestGetRunSummary_ErroredApply(t *testing.T) {
	t.Parallel()

	logServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Terraform v1.5.0\non linux_amd64\nApplying...\n"+
			`{"@level":"error","@message":"Error: Provider error","type":"diagnostic","diagnostic":{"severity":"error","summary":"Provider error","detail":"Unexpected error."}}`+"\n")
	}))
	t.Cleanup(logServer.Close)

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/runs/run-1":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes":    map[string]any{"status": "errored"},
					"relationships": map[string]any{"apply": map[string]any{"data": map[string]any{"id": "apply-1", "type": "applies"}}},
				},
			})
		case "GET /api/v2/runs/run-1/plan":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "finished"},
				},
			})
		case "GET /api/v2/applies/apply-1":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "apply-1", "type": "applies",
					"attributes": map[string]any{"status": "errored", "log-read-url": logServer.URL},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.GetRunSummary(context.Background(), c.TFE.API, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "apply", summary.Phase)
	require.Len(t, summary.Diagnostics, 1)
	assert.Equal(t, "Provider error", summary.Diagnostics[0].Summary)
}

func TestGetRunSummary_ErroredNoDiagnostics(t *testing.T) {
	t.Parallel()

	logServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Terraform v1.5.0\non linux_amd64\nInitializing...\nPlain text error output\n")
	}))
	t.Cleanup(logServer.Close)

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/runs/run-1":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "errored"},
				},
			})
		case "GET /api/v2/runs/run-1/plan":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "errored", "log-read-url": logServer.URL},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.GetRunSummary(context.Background(), c.TFE.API, "run-1")
	require.NoError(t, err)

	assert.Empty(t, summary.Diagnostics)
	assert.Contains(t, summary.RawLog, "Plain text error output")
}

func TestResolveRunID(t *testing.T) {
	t.Parallel()

	t.Run("run prefix passthrough", func(t *testing.T) {
		t.Parallel()
		runID, err := client.ResolveRunID(context.Background(), nil, "", "run-abc123")
		require.NoError(t, err)
		assert.Equal(t, "run-abc123", runID)
	})

	t.Run("workspace ID", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jsonapi(w, map[string]any{
				"data": []any{
					map[string]any{"id": "run-latest", "type": "runs", "attributes": map[string]any{"status": "applied"}},
				},
			})
		}))

		runID, err := client.ResolveRunID(context.Background(), c.TFE.API, "", "ws-123")
		require.NoError(t, err)
		assert.Equal(t, "run-latest", runID)
	})

	t.Run("workspace name", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/organizations/my-org/workspaces/my-ws":
				jsonapi(w, map[string]any{"data": map[string]any{"id": "ws-resolved", "type": "workspaces"}})
			case "GET /api/v2/workspaces/ws-resolved/runs":
				jsonapi(w, map[string]any{
					"data": []any{
						map[string]any{"id": "run-from-name", "type": "runs", "attributes": map[string]any{"status": "applied"}},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		runID, err := client.ResolveRunID(context.Background(), c.TFE.API, "my-org", "my-ws")
		require.NoError(t, err)
		assert.Equal(t, "run-from-name", runID)
	})

	t.Run("workspace name missing org", func(t *testing.T) {
		t.Parallel()
		_, err := client.ResolveRunID(context.Background(), nil, "", "my-ws")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--organization is required")
	})

	t.Run("no runs in workspace", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jsonapi(w, map[string]any{"data": []any{}})
		}))

		_, err := client.ResolveRunID(context.Background(), c.TFE.API, "", "ws-empty")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no runs found")
	})
}
