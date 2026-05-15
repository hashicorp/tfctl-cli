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

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func route(r *http.Request) string {
	return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
}

// routeMap maps "METHOD /path" to a handler function.
type routeMap map[string]http.HandlerFunc

func (rm routeMap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	if h, ok := rm[key]; ok {
		h(w, r)
		return
	}
	http.Error(w, "unexpected: "+key, http.StatusInternalServerError)
}

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

// simpleRunHandler returns a handler that serves a run with the given status and no extras.
func simpleRunHandler(status string) http.Handler {
	return routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": status},
				},
			})
		},
	}
}

// logHandler starts a log server and returns its URL. Cleanup is registered on t.
func logHandler(t *testing.T, content string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, content)
	}))
	t.Cleanup(s.Close)
	return s.URL
}

// sentinelRoutes returns routes for a sentinel policy check with the given status/log.
func sentinelRoutes(checkStatus, sentinelLog string) routeMap {
	return routeMap{
		"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": checkStatus},
						"links":      map[string]any{"output": "/api/v2/policy-checks/polchk-1/output"},
					},
				},
			})
		},
		"GET /api/v2/policy-checks/polchk-1/output": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/sentinel-log", http.StatusFound)
		},
		"GET /sentinel-log": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, sentinelLog)
		},
	}
}

// mergeRoutes combines multiple routeMaps; later maps override earlier.
func mergeRoutes(maps ...routeMap) routeMap {
	merged := routeMap{}
	for _, m := range maps {
		for k, v := range m {
			merged[k] = v
		}
	}
	return merged
}

func TestNewRunSummary_Statuses(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			c := testAPI(t, simpleRunHandler(tt.status))
			summary, err := client.NewRunSummary(context.Background(), c, "run-1")
			require.NoError(t, err)
			assert.Equal(t, tt.status, summary.Status)
			assert.Equal(t, tt.message, summary.Message)
			assert.Empty(t, summary.Diagnostics)
		})
	}
}

func TestNewRunSummary_ErroredPlan(t *testing.T) {
	t.Parallel()

	logURL := logHandler(t, "Terraform v1.5.0\non linux_amd64\nInitializing plugins...\n"+
		`{"@level":"error","@message":"Error: Bad resource","type":"diagnostic","diagnostic":{"severity":"error","summary":"Bad resource","detail":"Resource not declared."}}`+"\n")

	c := testAPI(t, routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "errored"},
				},
			})
		},
		"GET /api/v2/runs/run-1/plan": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "errored", "log-read-url": logURL},
				},
			})
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Equal(t, "errored", summary.Status)
	assert.Equal(t, "plan", summary.Phase)
	require.Len(t, summary.Diagnostics, 1)
	assert.Equal(t, "Bad resource", summary.Diagnostics[0].Summary)
}

func TestNewRunSummary_ErroredApply(t *testing.T) {
	t.Parallel()

	logURL := logHandler(t, "Terraform v1.5.0\non linux_amd64\nApplying...\n"+
		`{"@level":"error","@message":"Error: Provider error","type":"diagnostic","diagnostic":{"severity":"error","summary":"Provider error","detail":"Unexpected error."}}`+"\n")

	c := testAPI(t, routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes":    map[string]any{"status": "errored"},
					"relationships": map[string]any{"apply": map[string]any{"data": map[string]any{"id": "apply-1", "type": "applies"}}},
				},
			})
		},
		"GET /api/v2/runs/run-1/plan": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "finished"},
				},
			})
		},
		"GET /api/v2/applies/apply-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "apply-1", "type": "applies",
					"attributes": map[string]any{"status": "errored", "log-read-url": logURL},
				},
			})
		},
		"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{"data": []any{}})
		},
		"GET /api/v2/runs/run-1/task-stages": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{"data": []any{}})
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Equal(t, "apply", summary.Phase)
	require.Len(t, summary.Diagnostics, 1)
	assert.Equal(t, "Provider error", summary.Diagnostics[0].Summary)
}

func TestNewRunSummary_ErroredNoDiagnostics(t *testing.T) {
	t.Parallel()

	logURL := logHandler(t, "Terraform v1.5.0\non linux_amd64\nInitializing...\nPlain text error output\n")

	c := testAPI(t, routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "errored"},
				},
			})
		},
		"GET /api/v2/runs/run-1/plan": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "errored", "log-read-url": logURL},
				},
			})
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Empty(t, summary.Diagnostics)
	assert.Contains(t, summary.RawLog, "Plain text error output")
}

func TestNewRunSummary_PolicyCheckFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		runStatus  string
		checkStat  string
		log        string
		wantPhase  string
		wantMsg    string
		wantLogHas string
	}{
		{
			name:       "errored hard_failed",
			runStatus:  "errored",
			checkStat:  "hard_failed",
			log:        "Sentinel Result: false\n\n1 policies evaluated.\n\n## Policy 1: deny-all (hard-mandatory)\n\nResult: false\n",
			wantPhase:  "policy_check",
			wantLogHas: "hard-mandatory",
		},
		{
			name:       "errored soft_failed",
			runStatus:  "errored",
			checkStat:  "soft_failed",
			log:        "Sentinel Result: false\n\n## Policy 1: warn-all (soft-mandatory)\n\nResult: false\n",
			wantPhase:  "policy_check",
			wantLogHas: "soft-mandatory",
		},
		{
			name:       "policy_soft_failed",
			runStatus:  "policy_soft_failed",
			checkStat:  "soft_failed",
			log:        "Sentinel Result: false\n\n## Policy 1: cost-limit (soft-mandatory)\n\nResult: false\n",
			wantPhase:  "policy_check",
			wantMsg:    "Run has soft-failed policies",
			wantLogHas: "soft-mandatory",
		},
		{
			name:       "policy_override",
			runStatus:  "policy_override",
			checkStat:  "soft_failed",
			log:        "Sentinel Result: false\n\n## Policy 1: cost-limit (soft-mandatory)\n\nResult: false\n",
			wantPhase:  "policy_check",
			wantMsg:    "Run awaiting policy override",
			wantLogHas: "soft-mandatory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseRoutes := routeMap{
				"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
					jsonapi(w, map[string]any{
						"data": map[string]any{
							"id": "run-1", "type": "runs",
							"attributes": map[string]any{"status": tt.runStatus},
						},
					})
				},
				"GET /api/v2/runs/run-1/plan": func(w http.ResponseWriter, _ *http.Request) {
					jsonapi(w, map[string]any{
						"data": map[string]any{
							"id": "plan-1", "type": "plans",
							"attributes": map[string]any{"status": "finished"},
						},
					})
				},
			}

			c := testAPI(t, mergeRoutes(baseRoutes, sentinelRoutes(tt.checkStat, tt.log)))
			summary, err := client.NewRunSummary(context.Background(), c, "run-1")
			require.NoError(t, err)

			assert.Equal(t, tt.runStatus, summary.Status)
			assert.Equal(t, tt.wantPhase, summary.Phase)
			assert.Contains(t, summary.PolicyCheckLog, tt.wantLogHas)
			if tt.wantMsg != "" {
				assert.Equal(t, tt.wantMsg, summary.Message)
			}
		})
	}
}

func TestNewRunSummary_ErroredTaskStagePolicyEvaluation(t *testing.T) {
	t.Parallel()

	c := testAPI(t, routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "errored"},
				},
			})
		},
		"GET /api/v2/runs/run-1/plan": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "finished"},
				},
			})
		},
		"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{"data": []any{}})
		},
		"GET /api/v2/runs/run-1/task-stages": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "ts-1", "type": "task-stages",
						"attributes": map[string]any{"stage": "post_plan", "status": "errored"},
						"relationships": map[string]any{
							"policy-evaluations": map[string]any{
								"data": []map[string]any{{"id": "poleval-1", "type": "policy-evaluations"}},
							},
							"task-results": map[string]any{"data": []any{}},
						},
					},
				},
			})
		},
		"GET /api/v2/policy-evaluations/poleval-1/policy-set-outcomes": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "psout-1", "type": "policy-set-outcomes",
						"attributes": map[string]any{
							"policy-set-name": "deny-all-opa-test",
							"overridable":     false,
							"result-count":    map[string]any{"advisory-failed": 0, "mandatory-failed": 1, "passed": 0, "errored": 0},
							"outcomes": []map[string]any{
								{
									"enforcement_level": "mandatory",
									"query":             "data.terraform.deny",
									"status":            "failed",
									"output":            []string{"all resources are denied"},
									"policy_name":       "deny-all-opa",
									"description":       "Denies all resources",
								},
							},
						},
					},
				},
			})
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "errored", summary.Status)
	assert.Equal(t, "post_plan", summary.Phase)
	require.Len(t, summary.PolicyEvaluations, 1)
	pe := summary.PolicyEvaluations[0]
	assert.Equal(t, "deny-all-opa-test", pe.PolicySetName)
	require.Len(t, pe.Outcomes, 1)
	assert.Equal(t, "deny-all-opa", pe.Outcomes[0].PolicyName)
	assert.Equal(t, "mandatory", pe.Outcomes[0].EnforcementLevel)
	assert.Equal(t, "failed", pe.Outcomes[0].Status)
	assert.Equal(t, []string{"all resources are denied"}, pe.Outcomes[0].Output)
	assert.Equal(t, "Denies all resources", pe.Outcomes[0].Description)
}

func TestNewRunSummary_ErroredTaskStageRunTask(t *testing.T) {
	t.Parallel()

	c := testAPI(t, routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "errored"},
				},
			})
		},
		"GET /api/v2/runs/run-1/plan": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "plan-1", "type": "plans",
					"attributes": map[string]any{"status": "finished"},
				},
			})
		},
		"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{"data": []any{}})
		},
		"GET /api/v2/runs/run-1/task-stages": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "ts-1", "type": "task-stages",
						"attributes": map[string]any{"stage": "post_plan", "status": "failed"},
						"relationships": map[string]any{
							"policy-evaluations": map[string]any{"data": []any{}},
							"task-results": map[string]any{
								"data": []map[string]any{{"id": "tr-1", "type": "task-results"}},
							},
						},
					},
				},
			})
		},
		"GET /api/v2/task-results/tr-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "tr-1", "type": "task-results",
					"attributes": map[string]any{
						"task-name":                        "security-scan",
						"status":                           "failed",
						"message":                          "Security vulnerabilities found",
						"url":                              "https://example.com/scan/123",
						"workspace-task-enforcement-level": "mandatory",
						"stage":                            "post_plan",
					},
				},
			})
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "errored", summary.Status)
	assert.Equal(t, "post_plan", summary.Phase)
	require.Len(t, summary.TaskResults, 1)
	tr := summary.TaskResults[0]
	assert.Equal(t, "security-scan", tr.TaskName)
	assert.Equal(t, "failed", tr.Status)
	assert.Equal(t, "Security vulnerabilities found", tr.Message)
	assert.Equal(t, "https://example.com/scan/123", tr.URL)
	assert.Equal(t, "mandatory", tr.EnforcementLevel)
	assert.Equal(t, "post_plan", tr.Stage)
}

func TestStringPayload_PolicyCheckLog(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	log := "Sentinel Result: false\n\n## Policy 1: deny-all (hard-mandatory)\n\nResult: false\n"

	t.Run("pretty", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{
			Status: "errored", Phase: "policy_check", PolicyCheckLog: log,
		}, io: io}
		result := d.StringPayload(format.Pretty)
		assert.Equal(t, log, result)
	})

	t.Run("markdown strips ANSI", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{
			PolicyCheckLog: "Result: \x1b[31mfalse\x1b[0m\n",
		}, io: io}
		result := d.StringPayload(format.Markdown)
		assert.NotContains(t, result, "\x1b[")
		assert.Contains(t, result, "Result: false")
	})
}

func TestStringPayload_PolicyEvaluations(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Status: "errored",
		Phase:  "post_plan",
		PolicyEvaluations: []client.PolicyEvalResult{
			{
				PolicyKind:    "opa",
				PolicySetName: "deny-all-opa-test",
				Outcomes: []client.PolicyOutcome{
					{PolicyName: "deny-all-opa", EnforcementLevel: "mandatory", Status: "failed", Description: "Denies all resources", Output: []string{"all resources are denied"}},
					{PolicyName: "allow-tags", EnforcementLevel: "advisory", Status: "failed", Description: "Requires tags on resources"},
					{PolicyName: "cost-check", EnforcementLevel: "mandatory", Status: "passed"},
				},
			},
		},
	}
	d := &summaryDisplayer{summary: summary, io: io}

	t.Run("pretty", func(t *testing.T) {
		result := d.StringPayload(format.Pretty)
		for _, want := range []string{
			"Policy Evaluations", "OPA Policy Evaluation", "Overall Result:", "FAILED",
			"deny-all-opa-test", "3 policies evaluated",
			symbolDownArrow + " Policy name:", "deny-all-opa",
			symbolCross + " Failed", symbolInfo + " Advisory", symbolTick + " Passed",
			"Denies all resources", "all resources are denied",
		} {
			assert.Contains(t, result, want)
		}
	})

	t.Run("markdown", func(t *testing.T) {
		result := d.StringPayload(format.Markdown)
		for _, want := range []string{
			"## Policy Evaluations", "### OPA Policy Evaluation",
			"**Overall Result: FAILED**", "3 policies evaluated",
			"deny-all-opa-test", "deny-all-opa",
			"Failed", "Advisory", "Passed", "all resources are denied",
		} {
			assert.Contains(t, result, want)
		}
	})
}

func TestStringPayload_PolicyEvaluationsError(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	d := &summaryDisplayer{summary: &client.RunSummary{
		Status: "errored", Phase: "post_plan",
		PolicyEvaluations: []client.PolicyEvalResult{
			{PolicyKind: "opa", PolicySetName: "deny-all-opa-test", Error: "rego_parse_error: unexpected token"},
		},
	}, io: io}

	result := d.StringPayload(format.Pretty)
	assert.Contains(t, result, "ERRORED")
	assert.Contains(t, result, "deny-all-opa-test")
	assert.Contains(t, result, "rego_parse_error")
}

func TestStringPayload_TaskResults(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Status: "errored", Phase: "post_plan",
		TaskResults: []client.TaskResult{
			{TaskName: "security-scan", Status: "failed", Message: "Security vulnerabilities found", URL: "https://example.com/scan/123", EnforcementLevel: "mandatory", Stage: "post_plan"},
			{TaskName: "cost-estimate", Status: "passed", EnforcementLevel: "advisory", Stage: "post_plan"},
		},
	}
	d := &summaryDisplayer{summary: summary, io: io}

	t.Run("pretty", func(t *testing.T) {
		result := d.StringPayload(format.Pretty)
		for _, want := range []string{
			"All tasks completed! 1 passed, 1 failed",
			"security-scan " + symbolDash, "Failed (Mandatory)",
			"Security vulnerabilities found", "Details: https://example.com/scan/123",
			"cost-estimate " + symbolDash, "Passed",
			"Error:", "security-scan, is required to succeed", "Overall Result:",
		} {
			assert.Contains(t, result, want)
		}
	})

	t.Run("markdown", func(t *testing.T) {
		result := d.StringPayload(format.Markdown)
		for _, want := range []string{
			"All tasks completed! 1 passed, 1 failed",
			"security-scan", "Failed (mandatory)",
			"Security vulnerabilities found", "Details: https://example.com/scan/123",
			"cost-estimate", "Passed",
			"**Error:**", "**Overall Result: Failed**",
		} {
			assert.Contains(t, result, want)
		}
	})
}

func TestRunOrCurrentRun(t *testing.T) {
	t.Parallel()

	t.Run("runs type passthrough", func(t *testing.T) {
		t.Parallel()
		resolver := client.NewResolver(nil, false, false)
		runID, err := resolver.RunOrCurrentRun(context.Background(), "", "runs", "run-abc123")
		require.NoError(t, err)
		assert.Equal(t, "run-abc123", runID)
	})

	wsTests := []struct {
		name    string
		org     string
		input   string
		path    string
		wantRun string
	}{
		{"by ID", "", "ws-123", "/api/v2/workspaces/ws-123", "run-current"},
		{"by ID with org", "my-org", "ws-abc", "/api/v2/workspaces/ws-abc", "run-from-id"},
		{"by name", "my-org", "my-ws", "/api/v2/organizations/my-org/workspaces/my-ws", "run-from-name"},
	}

	for _, tt := range wsTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.path, r.URL.Path)
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-x", "type": "workspaces",
						"relationships": map[string]any{
							"current-run": map[string]any{
								"data": map[string]any{"id": tt.wantRun, "type": "runs"},
							},
						},
					},
				})
			}))
			resolver := client.NewResolver(c, false, false)
			runID, err := resolver.RunOrCurrentRun(context.Background(), tt.org, "workspaces", tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRun, runID)
		})
	}

	t.Run("no current run", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-empty", "type": "workspaces",
					"relationships": map[string]any{"current-run": map[string]any{"data": nil}},
				},
			})
		}))
		resolver := client.NewResolver(c, false, false)
		_, err := resolver.RunOrCurrentRun(context.Background(), "", "workspaces", "ws-empty")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no current run")
	})

	t.Run("unsupported type", func(t *testing.T) {
		t.Parallel()
		resolver := client.NewResolver(nil, false, false)
		_, err := resolver.RunOrCurrentRun(context.Background(), "", "plans", "plan-123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported resource type")
	})
}

func TestFormatDiagnostics(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	ctx := strPtr("output \"bad\"")

	t.Run("pretty with snippet", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{
			Diagnostics: []client.Diagnostic{{
				Severity: "error",
				Summary:  "Reference to undeclared input variable",
				Detail:   "An input variable with the name \"does_not_exist\" has not been declared.",
				Range:    &client.DiagnosticRange{Filename: "main.tf", Start: client.SourceLocation{Line: 2, Column: 11, Byte: 25}, End: client.SourceLocation{Line: 2, Column: 31, Byte: 45}},
				Snippet:  &client.DiagnosticSnippet{Context: ctx, Code: "  value = var.does_not_exist.foo", StartLine: 2, HighlightStartOffset: 10, HighlightEndOffset: 30},
			}},
		}, io: io}
		result := d.StringPayload(format.Pretty)
		for _, want := range []string{"Error:", "Reference to undeclared input variable", "on main.tf line 2, in output \"bad\":", "   2:", "value = var.does_not_exist.foo", "╷", "│", "╵"} {
			assert.Contains(t, result, want)
		}
	})

	t.Run("markdown with snippet", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{
			Diagnostics: []client.Diagnostic{{
				Severity: "error",
				Summary:  "Reference to undeclared input variable",
				Detail:   "An input variable with the name \"does_not_exist\" has not been declared.",
				Range:    &client.DiagnosticRange{Filename: "main.tf", Start: client.SourceLocation{Line: 2, Column: 11, Byte: 25}},
				Snippet:  &client.DiagnosticSnippet{Context: ctx, Code: "  value = var.does_not_exist.foo", StartLine: 2},
			}},
		}, io: io}
		result := d.StringPayload(format.Markdown)
		assert.Contains(t, result, "**Error: Reference to undeclared input variable**")
		assert.Contains(t, result, "```hcl")
		assert.NotContains(t, result, "╷")
	})

	t.Run("pretty range without snippet", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{
			Diagnostics: []client.Diagnostic{{
				Severity: "error", Summary: "Some error",
				Range: &client.DiagnosticRange{Filename: "main.tf", Start: client.SourceLocation{Line: 5}},
			}},
		}, io: io}
		result := d.StringPayload(format.Pretty)
		assert.Contains(t, result, "on main.tf line 5:")
		assert.NotContains(t, result, "in ")
	})

	t.Run("pretty snippet no highlight", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{
			Diagnostics: []client.Diagnostic{{
				Severity: "error", Summary: "Some error",
				Snippet: &client.DiagnosticSnippet{Code: "resource \"aws_instance\" \"web\" {}", StartLine: 1},
			}},
		}, io: io}
		result := d.StringPayload(format.Pretty)
		assert.Contains(t, result, "   1:")
		assert.Contains(t, result, "resource \"aws_instance\" \"web\" {}")
	})
}

func TestStringPayload_ANSIHandling(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	rawLog := "Terraform v1.13.5\n\x1b[1m\x1b[31m╷\x1b[0m\n\x1b[1m\x1b[31m│\x1b[0m \x1b[1mError: Unsupported version\x1b[0m\n\x1b[1m\x1b[31m╵\x1b[0m\n"

	t.Run("markdown strips ANSI", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{Status: "errored", RawLog: rawLog}, io: io}
		result := d.StringPayload(format.Markdown)
		assert.NotContains(t, result, "\x1b[")
		assert.Contains(t, result, "Error: Unsupported version")
	})

	t.Run("pretty preserves ANSI", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{Status: "errored", RawLog: rawLog}, io: io}
		result := d.StringPayload(format.Pretty)
		assert.Contains(t, result, "\x1b[31m")
	})
}

func TestRunStatus_ExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"errored exits non-zero", "errored", true},
		{"policy_soft_failed exits non-zero", "policy_soft_failed", true},
		{"policy_override exits non-zero", "policy_override", true},
		{"applied exits zero", "applied", false},
		{"planned_and_finished exits zero", "planned_and_finished", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logURL := logHandler(t, `{"@level":"error","@message":"Error: fail","type":"diagnostic","diagnostic":{"severity":"error","summary":"fail"}}`)

			c := testAPI(t, routeMap{
				"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
					jsonapi(w, map[string]any{
						"data": map[string]any{
							"id": "run-1", "type": "runs",
							"attributes": map[string]any{"status": tt.status},
						},
					})
				},
				"GET /api/v2/runs/run-1/plan": func(w http.ResponseWriter, _ *http.Request) {
					jsonapi(w, map[string]any{
						"data": map[string]any{
							"id": "plan-1", "type": "plans",
							"attributes": map[string]any{"status": "errored", "log-read-url": logURL},
						},
					})
				},
				"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
					jsonapi(w, map[string]any{"data": []any{}})
				},
				"GET /api/v2/runs/run-1/task-stages": func(w http.ResponseWriter, _ *http.Request) {
					jsonapi(w, map[string]any{"data": []any{}})
				},
			})

			io := iostreams.Test()
			out := format.New(io)
			opts := &StatusOpts{
				IO: io, ShutdownCtx: context.Background(),
				Output: out, Client: c, ID: "run-1",
			}

			err := runStatus(opts)
			if tt.wantErr {
				assert.ErrorIs(t, err, cmd.ErrUnderlyingError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
