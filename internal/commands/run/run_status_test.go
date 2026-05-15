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
			c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-1", "type": "runs",
						"attributes": map[string]any{"status": tt.status},
					},
				})
			}))

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

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "errored", summary.Status)
	assert.Equal(t, "plan", summary.Phase)
	require.Len(t, summary.Diagnostics, 1)
	assert.Equal(t, "Bad resource", summary.Diagnostics[0].Summary)
}

func TestNewRunSummary_ErroredApply(t *testing.T) {
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
		case "GET /api/v2/runs/run-1/policy-checks":
			jsonapi(w, map[string]any{"data": []any{}})
		case "GET /api/v2/runs/run-1/task-stages":
			jsonapi(w, map[string]any{"data": []any{}})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "apply", summary.Phase)
	require.Len(t, summary.Diagnostics, 1)
	assert.Equal(t, "Provider error", summary.Diagnostics[0].Summary)
}

func TestNewRunSummary_ErroredNoDiagnostics(t *testing.T) {
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

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Empty(t, summary.Diagnostics)
	assert.Contains(t, summary.RawLog, "Plain text error output")
}

func TestNewRunSummary_ErroredPolicyCheckHardFailed(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false\n\nThis result means that one or more Sentinel policies failed.\n\n1 policies evaluated.\n\n## Policy 1: deny-all (hard-mandatory)\n\nResult: false\n"

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
					"attributes": map[string]any{"status": "finished"},
				},
			})
		case "GET /api/v2/runs/run-1/policy-checks":
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "hard_failed"},
						"links":      map[string]any{"output": "/api/v2/policy-checks/polchk-1/output"},
					},
				},
			})
		case "GET /api/v2/policy-checks/polchk-1/output":
			// Simulate the 302 redirect that the real API returns.
			http.Redirect(w, r, "/sentinel-log", http.StatusFound)
		case "GET /sentinel-log":
			fmt.Fprint(w, sentinelLog)
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "errored", summary.Status)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Contains(t, summary.PolicyCheckLog, "deny-all")
	assert.Contains(t, summary.PolicyCheckLog, "hard-mandatory")
}

func TestNewRunSummary_ErroredPolicyCheckSoftFailed(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false\n\n## Policy 1: warn-all (soft-mandatory)\n\nResult: false\n"

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
					"attributes": map[string]any{"status": "finished"},
				},
			})
		case "GET /api/v2/runs/run-1/policy-checks":
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "soft_failed"},
						"links":      map[string]any{"output": "/api/v2/policy-checks/polchk-1/output"},
					},
				},
			})
		case "GET /api/v2/policy-checks/polchk-1/output":
			http.Redirect(w, r, "/sentinel-log", http.StatusFound)
		case "GET /sentinel-log":
			fmt.Fprint(w, sentinelLog)
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "errored", summary.Status)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Contains(t, summary.PolicyCheckLog, "soft-mandatory")
}

func TestNewRunSummary_ErroredTaskStagePolicyEvaluation(t *testing.T) {
	t.Parallel()

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
					"attributes": map[string]any{"status": "finished"},
				},
			})
		case "GET /api/v2/runs/run-1/policy-checks":
			jsonapi(w, map[string]any{"data": []any{}})
		case "GET /api/v2/runs/run-1/task-stages":
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "ts-1", "type": "task-stages",
						"attributes": map[string]any{"stage": "post_plan", "status": "errored"},
						"relationships": map[string]any{
							"policy-evaluations": map[string]any{
								"data": []map[string]any{
									{"id": "poleval-1", "type": "policy-evaluations"},
								},
							},
							"task-results": map[string]any{
								"data": []any{},
							},
						},
					},
				},
			})
		case "GET /api/v2/policy-evaluations/poleval-1/policy-set-outcomes":
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "psout-1", "type": "policy-set-outcomes",
						"attributes": map[string]any{
							"policy-set-name": "deny-all-opa-test",
							"overridable":     false,
							"result-count": map[string]any{
								"advisory-failed":  0,
								"mandatory-failed": 1,
								"passed":           0,
								"errored":          0,
							},
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
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

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
					"attributes": map[string]any{"status": "finished"},
				},
			})
		case "GET /api/v2/runs/run-1/policy-checks":
			jsonapi(w, map[string]any{"data": []any{}})
		case "GET /api/v2/runs/run-1/task-stages":
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "ts-1", "type": "task-stages",
						"attributes": map[string]any{"stage": "post_plan", "status": "failed"},
						"relationships": map[string]any{
							"policy-evaluations": map[string]any{
								"data": []any{},
							},
							"task-results": map[string]any{
								"data": []map[string]any{
									{"id": "tr-1", "type": "task-results"},
								},
							},
						},
					},
				},
			})
		case "GET /api/v2/task-results/tr-1":
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
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

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

func TestNewRunSummary_PolicySoftFailed(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false\n\n## Policy 1: cost-limit (soft-mandatory)\n\nResult: false\n"

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/runs/run-1":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "policy_soft_failed"},
				},
			})
		case "GET /api/v2/runs/run-1/policy-checks":
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "soft_failed"},
						"links":      map[string]any{"output": "/api/v2/policy-checks/polchk-1/output"},
					},
				},
			})
		case "GET /api/v2/policy-checks/polchk-1/output":
			http.Redirect(w, r, "/sentinel-log", http.StatusFound)
		case "GET /sentinel-log":
			fmt.Fprint(w, sentinelLog)
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "policy_soft_failed", summary.Status)
	assert.Equal(t, "Run has soft-failed policies", summary.Message)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Contains(t, summary.PolicyCheckLog, "soft-mandatory")
}

func TestNewRunSummary_PolicyOverride(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false\n\n## Policy 1: cost-limit (soft-mandatory)\n\nResult: false\n"

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/runs/run-1":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "policy_override"},
				},
			})
		case "GET /api/v2/runs/run-1/policy-checks":
			jsonapi(w, map[string]any{
				"data": []map[string]any{
					{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "soft_failed"},
						"links":      map[string]any{"output": "/api/v2/policy-checks/polchk-1/output"},
					},
				},
			})
		case "GET /api/v2/policy-checks/polchk-1/output":
			http.Redirect(w, r, "/sentinel-log", http.StatusFound)
		case "GET /sentinel-log":
			fmt.Fprint(w, sentinelLog)
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)

	assert.Equal(t, "policy_override", summary.Status)
	assert.Equal(t, "Run awaiting policy override", summary.Message)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Contains(t, summary.PolicyCheckLog, "soft-mandatory")
}

func TestStringPayload_PolicyCheckLog(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	log := "Sentinel Result: false\n\n## Policy 1: deny-all (hard-mandatory)\n\nResult: false\n"
	summary := &client.RunSummary{
		Status:         "errored",
		Phase:          "policy_check",
		PolicyCheckLog: log,
	}

	d := &summaryDisplayer{summary: summary, io: io}

	t.Run("pretty", func(t *testing.T) {
		result := d.StringPayload(format.Pretty)
		assert.Equal(t, log, result)
	})

	t.Run("markdown", func(t *testing.T) {
		// PolicyCheckLog may contain ANSI; markdown should strip it.
		summaryWithANSI := &client.RunSummary{
			PolicyCheckLog: "Result: \x1b[31mfalse\x1b[0m\n",
		}
		dm := &summaryDisplayer{summary: summaryWithANSI, io: io}
		result := dm.StringPayload(format.Markdown)
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
					{
						PolicyName:       "deny-all-opa",
						EnforcementLevel: "mandatory",
						Status:           "failed",
						Description:      "Denies all resources",
						Output:           []string{"all resources are denied"},
					},
					{
						PolicyName:       "allow-tags",
						EnforcementLevel: "advisory",
						Status:           "failed",
						Description:      "Requires tags on resources",
					},
					{
						PolicyName:       "cost-check",
						EnforcementLevel: "mandatory",
						Status:           "passed",
					},
				},
			},
		},
	}

	d := &summaryDisplayer{summary: summary, io: io}

	t.Run("pretty", func(t *testing.T) {
		result := d.StringPayload(format.Pretty)
		assert.Contains(t, result, "Policy Evaluations")
		assert.Contains(t, result, "OPA Policy Evaluation")
		assert.Contains(t, result, "Overall Result:")
		assert.Contains(t, result, "FAILED")
		assert.Contains(t, result, "deny-all-opa-test")
		assert.Contains(t, result, "3 policies evaluated")
		// TF CLI-style per-policy format
		assert.Contains(t, result, symbolDownArrow+" Policy name:")
		assert.Contains(t, result, "deny-all-opa")
		assert.Contains(t, result, symbolCross+" Failed")
		assert.Contains(t, result, symbolInfo+" Advisory")
		assert.Contains(t, result, symbolTick+" Passed")
		assert.Contains(t, result, "Denies all resources")
		assert.Contains(t, result, "all resources are denied")
	})

	t.Run("markdown", func(t *testing.T) {
		result := d.StringPayload(format.Markdown)
		assert.Contains(t, result, "## Policy Evaluations")
		assert.Contains(t, result, "### OPA Policy Evaluation")
		assert.Contains(t, result, "**Overall Result: FAILED**")
		assert.Contains(t, result, "3 policies evaluated")
		assert.Contains(t, result, "deny-all-opa-test")
		assert.Contains(t, result, "deny-all-opa")
		assert.Contains(t, result, "Failed")
		assert.Contains(t, result, "Advisory")
		assert.Contains(t, result, "Passed")
		assert.Contains(t, result, "all resources are denied")
	})
}

func TestStringPayload_PolicyEvaluationsError(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Status: "errored",
		Phase:  "post_plan",
		PolicyEvaluations: []client.PolicyEvalResult{
			{
				PolicyKind:    "opa",
				PolicySetName: "deny-all-opa-test",
				Error:         "rego_parse_error: unexpected token",
			},
		},
	}

	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Pretty)
	assert.Contains(t, result, "Overall Result:")
	assert.Contains(t, result, "ERRORED")
	assert.Contains(t, result, "deny-all-opa-test")
	assert.Contains(t, result, "rego_parse_error")
}

func TestStringPayload_TaskResults(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Status: "errored",
		Phase:  "post_plan",
		TaskResults: []client.TaskResult{
			{
				TaskName:         "security-scan",
				Status:           "failed",
				Message:          "Security vulnerabilities found",
				URL:              "https://example.com/scan/123",
				EnforcementLevel: "mandatory",
				Stage:            "post_plan",
			},
			{
				TaskName:         "cost-estimate",
				Status:           "passed",
				EnforcementLevel: "advisory",
				Stage:            "post_plan",
			},
		},
	}

	d := &summaryDisplayer{summary: summary, io: io}

	t.Run("pretty", func(t *testing.T) {
		result := d.StringPayload(format.Pretty)
		assert.Contains(t, result, "All tasks completed! 1 passed, 1 failed")
		assert.Contains(t, result, "security-scan "+symbolDash)
		assert.Contains(t, result, "Failed (Mandatory)")
		assert.Contains(t, result, "Security vulnerabilities found")
		assert.Contains(t, result, "Details: https://example.com/scan/123")
		assert.Contains(t, result, "cost-estimate "+symbolDash)
		assert.Contains(t, result, "Passed")
		assert.Contains(t, result, "Error:")
		assert.Contains(t, result, "security-scan, is required to succeed")
		assert.Contains(t, result, "Overall Result:")
	})

	t.Run("markdown", func(t *testing.T) {
		result := d.StringPayload(format.Markdown)
		assert.Contains(t, result, "All tasks completed! 1 passed, 1 failed")
		assert.Contains(t, result, "security-scan")
		assert.Contains(t, result, "Failed (mandatory)")
		assert.Contains(t, result, "Security vulnerabilities found")
		assert.Contains(t, result, "Details: https://example.com/scan/123")
		assert.Contains(t, result, "cost-estimate")
		assert.Contains(t, result, "Passed")
		assert.Contains(t, result, "**Error:**")
		assert.Contains(t, result, "**Overall Result: Failed**")
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

	t.Run("workspaces type by ID", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/workspaces/ws-123", r.URL.Path, "should use workspace ID endpoint")
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-123", "type": "workspaces",
					"relationships": map[string]any{
						"current-run": map[string]any{
							"data": map[string]any{"id": "run-current", "type": "runs"},
						},
					},
				},
			})
		}))

		resolver := client.NewResolver(c, false, false)
		runID, err := resolver.RunOrCurrentRun(context.Background(), "", "workspaces", "ws-123")
		require.NoError(t, err)
		assert.Equal(t, "run-current", runID)
	})

	t.Run("workspaces type by ID with org set", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/workspaces/ws-abc", r.URL.Path, "ws- prefix should bypass org-based name lookup")
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-abc", "type": "workspaces",
					"relationships": map[string]any{
						"current-run": map[string]any{
							"data": map[string]any{"id": "run-from-id", "type": "runs"},
						},
					},
				},
			})
		}))

		resolver := client.NewResolver(c, false, false)
		runID, err := resolver.RunOrCurrentRun(context.Background(), "my-org", "workspaces", "ws-abc")
		require.NoError(t, err)
		assert.Equal(t, "run-from-id", runID)
	})

	t.Run("workspaces type by name", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/organizations/my-org/workspaces/my-ws", r.URL.Path, "should use org+name endpoint")
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"relationships": map[string]any{
						"current-run": map[string]any{
							"data": map[string]any{"id": "run-from-name", "type": "runs"},
						},
					},
				},
			})
		}))

		resolver := client.NewResolver(c, false, false)
		runID, err := resolver.RunOrCurrentRun(context.Background(), "my-org", "workspaces", "my-ws")
		require.NoError(t, err)
		assert.Equal(t, "run-from-name", runID)
	})

	t.Run("no current run", func(t *testing.T) {
		t.Parallel()
		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-empty", "type": "workspaces",
					"relationships": map[string]any{
						"current-run": map[string]any{
							"data": nil,
						},
					},
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

func TestFormatDiagnosticsPretty_WithSnippet(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	ctx := strPtr("output \"bad\"")
	summary := &client.RunSummary{
		Diagnostics: []client.Diagnostic{
			{
				Severity: "error",
				Summary:  "Reference to undeclared input variable",
				Detail:   "An input variable with the name \"does_not_exist\" has not been declared.",
				Range: &client.DiagnosticRange{
					Filename: "main.tf",
					Start:    client.SourceLocation{Line: 2, Column: 11, Byte: 25},
					End:      client.SourceLocation{Line: 2, Column: 31, Byte: 45},
				},
				Snippet: &client.DiagnosticSnippet{
					Context:              ctx,
					Code:                 "  value = var.does_not_exist.foo",
					StartLine:            2,
					HighlightStartOffset: 10,
					HighlightEndOffset:   30,
				},
			},
		},
	}

	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Pretty)

	assert.Contains(t, result, "Error:")
	assert.Contains(t, result, "Reference to undeclared input variable")
	assert.Contains(t, result, "on main.tf line 2, in output \"bad\":")
	assert.Contains(t, result, "   2:")
	assert.Contains(t, result, "value = var.does_not_exist.foo")
	assert.Contains(t, result, "╷")
	assert.Contains(t, result, "│")
	assert.Contains(t, result, "╵")
}

func TestFormatDiagnosticsMarkdown_WithSnippet(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	ctx := strPtr("output \"bad\"")
	summary := &client.RunSummary{
		Diagnostics: []client.Diagnostic{
			{
				Severity: "error",
				Summary:  "Reference to undeclared input variable",
				Detail:   "An input variable with the name \"does_not_exist\" has not been declared.",
				Range: &client.DiagnosticRange{
					Filename: "main.tf",
					Start:    client.SourceLocation{Line: 2, Column: 11, Byte: 25},
				},
				Snippet: &client.DiagnosticSnippet{
					Context:   ctx,
					Code:      "  value = var.does_not_exist.foo",
					StartLine: 2,
				},
			},
		},
	}

	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Markdown)

	assert.Contains(t, result, "**Error: Reference to undeclared input variable**")
	assert.Contains(t, result, "on main.tf line 2, in output \"bad\":")
	assert.Contains(t, result, "```hcl")
	assert.Contains(t, result, "value = var.does_not_exist.foo")
	assert.Contains(t, result, "```")
	assert.NotContains(t, result, "╷")
	assert.NotContains(t, result, "│")
}

func TestStringPayload_MarkdownStripsANSI(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Status: "errored",
		RawLog: "Terraform v1.13.5\n\x1b[1m\x1b[31m╷\x1b[0m\n\x1b[1m\x1b[31m│\x1b[0m \x1b[1mError: Unsupported version\x1b[0m\n\x1b[1m\x1b[31m╵\x1b[0m\n",
	}

	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Markdown)

	assert.NotContains(t, result, "\x1b[")
	assert.Contains(t, result, "╷")
	assert.Contains(t, result, "Error: Unsupported version")
	assert.Contains(t, result, "╵")
}

func TestStringPayload_PrettyPreservesANSI(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Status: "errored",
		RawLog: "Terraform v1.13.5\n\x1b[31mError\x1b[0m\n",
	}

	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Pretty)

	assert.Contains(t, result, "\x1b[31m")
}

func TestFormatDiagnosticsPretty_RangeWithoutSnippet(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Diagnostics: []client.Diagnostic{
			{
				Severity: "error",
				Summary:  "Some error",
				Range: &client.DiagnosticRange{
					Filename: "main.tf",
					Start:    client.SourceLocation{Line: 5},
				},
			},
		},
	}

	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Pretty)

	assert.Contains(t, result, "on main.tf line 5:")
	assert.NotContains(t, result, "in ")
}

func TestFormatDiagnosticsPretty_SnippetNoHighlight(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Diagnostics: []client.Diagnostic{
			{
				Severity: "error",
				Summary:  "Some error",
				Snippet: &client.DiagnosticSnippet{
					Code:      "resource \"aws_instance\" \"web\" {}",
					StartLine: 1,
				},
			},
		},
	}

	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Pretty)

	assert.Contains(t, result, "   1:")
	assert.Contains(t, result, "resource \"aws_instance\" \"web\" {}")
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

			logServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, `{"@level":"error","@message":"Error: fail","type":"diagnostic","diagnostic":{"severity":"error","summary":"fail"}}`)
			}))
			t.Cleanup(logServer.Close)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch route(r) {
				case "GET /api/v2/runs/run-1":
					jsonapi(w, map[string]any{
						"data": map[string]any{
							"id": "run-1", "type": "runs",
							"attributes": map[string]any{"status": tt.status},
						},
					})
				case "GET /api/v2/runs/run-1/plan":
					jsonapi(w, map[string]any{
						"data": map[string]any{
							"id": "plan-1", "type": "plans",
							"attributes": map[string]any{"status": "errored", "log-read-url": logServer.URL},
						},
					})
				case "GET /api/v2/runs/run-1/policy-checks":
					jsonapi(w, map[string]any{"data": []any{}})
				case "GET /api/v2/runs/run-1/task-stages":
					jsonapi(w, map[string]any{"data": []any{}})
				default:
					http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
				}
			})

			c := testAPI(t, handler)
			io := iostreams.Test()
			out := format.New(io)

			opts := &StatusOpts{
				IO:          io,
				ShutdownCtx: context.Background(),
				Output:      out,
				Client:      c,
				ID:          "run-1",
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
