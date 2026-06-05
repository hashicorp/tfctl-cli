// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
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
	c, err := client.New(server.URL, "test-token", nil, hclog.NewNullLogger())
	require.NoError(t, err)
	return c
}

func jsonapi(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	_ = json.NewEncoder(w).Encode(payload)
}

// logHandler starts a log server and returns its URL.
func logHandler(t *testing.T, content string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, content)
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func TestStringPayload_PolicyCheckLog(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	log := "Sentinel Result: false\n\n## Policy 1: deny-all (hard-mandatory)\n\nResult: false\n"

	t.Run("pretty", func(t *testing.T) {
		d := &summaryDisplayer{summary: &client.RunSummary{
			Status: "errored", Phase: "policy_check", PolicyCheckLog: log,
			PolicyCheckScope: "organization", PolicyCheckStatus: "hard_failed",
		}, io: io}
		result := d.StringPayload(format.Pretty)
		assert.Contains(t, result, "Organization Policy Check:")
		assert.Contains(t, result, log)
		assert.Contains(t, result, "hard failed")
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
			"All tasks completed: 1 passed, 1 failed",
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
			"All tasks completed: 1 passed, 1 failed",
			"security-scan", "Failed (mandatory)",
			"Security vulnerabilities found", "Details: https://example.com/scan/123",
			"cost-estimate", "Passed",
			"**Error:**", "**Overall Result: Failed**",
		} {
			assert.Contains(t, result, want)
		}
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
				Range:    &client.DiagnosticRange{Filename: "main.tf", Start: client.SourceLocation{Line: 2, Column: 11, Byte: 25}},
				Snippet:  &client.DiagnosticSnippet{Context: ctx, Code: "  value = var.does_not_exist.foo", StartLine: 2},
			}},
		}, io: io}
		result := d.StringPayload(format.Markdown)
		assert.Contains(t, result, "**Error: Reference to undeclared input variable**")
		assert.Contains(t, result, "```hcl")
		assert.NotContains(t, result, "╷")
	})

	t.Run("range without snippet", func(t *testing.T) {
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

	t.Run("snippet no highlight", func(t *testing.T) {
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

func TestStringPayload_MultipleFailuresDivider(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	summary := &client.RunSummary{
		Status: "errored",
		Phase:  "post_plan",
		Diagnostics: []client.Diagnostic{
			{Severity: "error", Summary: "Reference to undeclared variable"},
		},
		PolicyEvaluations: []client.PolicyEvalResult{
			{
				PolicyKind:    "opa",
				PolicySetName: "deny-set",
				Outcomes: []client.PolicyOutcome{
					{PolicyName: "deny-all", EnforcementLevel: "mandatory", Status: "failed"},
				},
			},
		},
		TaskResults: []client.TaskResult{
			{TaskName: "lint", Status: "failed", EnforcementLevel: "mandatory", Stage: "post_plan"},
		},
	}
	d := &summaryDisplayer{summary: summary, io: io}

	t.Run("pretty uses unicode divider between sections", func(t *testing.T) {
		result := d.StringPayload(format.Pretty)
		divider := "――――――――――――"
		assert.Contains(t, result, divider)
		// All three sections should appear.
		assert.Contains(t, result, "Reference to undeclared variable")
		assert.Contains(t, result, "Policy Evaluations:")
		assert.Contains(t, result, "All tasks completed:")
	})

	t.Run("markdown uses horizontal rule between sections", func(t *testing.T) {
		result := d.StringPayload(format.Markdown)
		assert.Contains(t, result, "\n\n---\n\n")
		assert.Contains(t, result, "Reference to undeclared variable")
		assert.Contains(t, result, "## Policy Evaluations:")
		assert.Contains(t, result, "## Run Tasks")
	})
}

func strPtr(s string) *string { return &s }

// --- Integration tests for NewRunSummary ---

func TestNewRunSummary_ErroredPolicyCheckHardFailed(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false\n\n## Policy 1: deny-all (hard-mandatory)\n\nResult: false\n"

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
			jsonapi(w, map[string]any{
				"data": []any{
					map[string]any{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "hard_failed", "scope": "organization"},
					},
				},
			})
		},
		"GET /api/v2/policy-checks/polchk-1/output": func(w http.ResponseWriter, r *http.Request) {
			// Return a 302 redirect to a "presigned" URL on the same server.
			http.Redirect(w, r, "/fake-archivist/polchk-1-log", http.StatusFound)
		},
		"GET /fake-archivist/polchk-1-log": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, sentinelLog)
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Equal(t, "organization", summary.PolicyCheckScope)
	assert.Equal(t, "hard_failed", summary.PolicyCheckStatus)
	assert.Equal(t, sentinelLog, summary.PolicyCheckLog)
}

func TestNewRunSummary_ErroredPolicyCheckSoftFailed(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false (soft)\n"

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
			jsonapi(w, map[string]any{
				"data": []any{
					map[string]any{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "soft_failed", "scope": "workspace"},
					},
				},
			})
		},
		"GET /api/v2/policy-checks/polchk-1/output": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/fake-archivist/polchk-1-log", http.StatusFound)
		},
		"GET /fake-archivist/polchk-1-log": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, sentinelLog)
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Equal(t, "workspace", summary.PolicyCheckScope)
	assert.Equal(t, "soft_failed", summary.PolicyCheckStatus)
	assert.Equal(t, sentinelLog, summary.PolicyCheckLog)
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
				"data": []any{
					map[string]any{
						"id": "ts-1", "type": "task-stages",
						"attributes": map[string]any{"stage": "post_plan", "status": "errored"},
						"relationships": map[string]any{
							"policy-evaluations": map[string]any{
								"data": []any{map[string]any{"id": "poleval-1", "type": "policy-evaluations"}},
							},
							"task-results": map[string]any{"data": []any{}},
						},
					},
				},
			})
		},
		"GET /api/v2/policy-evaluations/poleval-1/policy-set-outcomes": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": []any{
					map[string]any{
						"id": "psout-1", "type": "policy-set-outcomes",
						"attributes": map[string]any{
							"policy-set-name": "deny-all-opa-test",
							"outcomes": []any{
								map[string]any{
									"policy_name":       "deny-all-opa",
									"enforcement_level": "mandatory",
									"status":            "failed",
									"description":       "Denies all resources",
									"output":            []any{"all resources are denied"},
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
	assert.Equal(t, "post_plan", summary.Phase)
	require.Len(t, summary.PolicyEvaluations, 1)
	assert.Equal(t, "deny-all-opa-test", summary.PolicyEvaluations[0].PolicySetName)
	require.Len(t, summary.PolicyEvaluations[0].Outcomes, 1)
	assert.Equal(t, "deny-all-opa", summary.PolicyEvaluations[0].Outcomes[0].PolicyName)
	assert.Equal(t, "mandatory", summary.PolicyEvaluations[0].Outcomes[0].EnforcementLevel)
	assert.Equal(t, "failed", summary.PolicyEvaluations[0].Outcomes[0].Status)
	assert.Equal(t, []string{"all resources are denied"}, summary.PolicyEvaluations[0].Outcomes[0].Output)
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
				"data": []any{
					map[string]any{
						"id": "ts-1", "type": "task-stages",
						"attributes": map[string]any{"stage": "post_plan", "status": "failed"},
						"relationships": map[string]any{
							"policy-evaluations": map[string]any{"data": []any{}},
							"task-results": map[string]any{
								"data": []any{map[string]any{"id": "taskrs-1", "type": "task-results"}},
							},
						},
					},
				},
			})
		},
		"GET /api/v2/task-results/taskrs-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "taskrs-1", "type": "task-results",
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
	assert.Equal(t, "post_plan", summary.Phase)
	require.Len(t, summary.TaskResults, 1)
	assert.Equal(t, "security-scan", summary.TaskResults[0].TaskName)
	assert.Equal(t, "failed", summary.TaskResults[0].Status)
	assert.Equal(t, "Security vulnerabilities found", summary.TaskResults[0].Message)
	assert.Equal(t, "https://example.com/scan/123", summary.TaskResults[0].URL)
	assert.Equal(t, "mandatory", summary.TaskResults[0].EnforcementLevel)
}

func TestNewRunSummary_PolicySoftFailed(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false (soft)\n"

	c := testAPI(t, routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "policy_soft_failed"},
				},
			})
		},
		"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": []any{
					map[string]any{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "soft_failed", "scope": "organization"},
					},
				},
			})
		},
		"GET /api/v2/policy-checks/polchk-1/output": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/fake-archivist/polchk-1-log", http.StatusFound)
		},
		"GET /fake-archivist/polchk-1-log": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, sentinelLog)
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Equal(t, "Run has soft-failed policies", summary.Message)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Equal(t, sentinelLog, summary.PolicyCheckLog)
}

func TestNewRunSummary_PolicyOverride(t *testing.T) {
	t.Parallel()

	sentinelLog := "Sentinel Result: false (overridable)\n"

	c := testAPI(t, routeMap{
		"GET /api/v2/runs/run-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-1", "type": "runs",
					"attributes": map[string]any{"status": "policy_override"},
				},
			})
		},
		"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": []any{
					map[string]any{
						"id": "polchk-1", "type": "policy-checks",
						"attributes": map[string]any{"status": "soft_failed", "scope": "organization"},
					},
				},
			})
		},
		"GET /api/v2/policy-checks/polchk-1/output": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/fake-archivist/polchk-1-log", http.StatusFound)
		},
		"GET /fake-archivist/polchk-1-log": func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, sentinelLog)
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Equal(t, "Run awaiting policy override", summary.Message)
	assert.Equal(t, "policy_check", summary.Phase)
	assert.Equal(t, sentinelLog, summary.PolicyCheckLog)
}

func TestNewRunSummary_ErroredFallsThrough(t *testing.T) {
	t.Parallel()

	logURL := logHandler(t, `Terraform v1.5.0
on linux_amd64
Initializing plugins...
{"@level":"error","@message":"Error: fail","type":"diagnostic","diagnostic":{"severity":"error","summary":"Apply failed","detail":"Something went wrong"}}`)

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
		"GET /api/v2/runs/run-1/policy-checks": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{"data": []any{}})
		},
		"GET /api/v2/runs/run-1/task-stages": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{"data": []any{}})
		},
		"GET /api/v2/applies/apply-1": func(w http.ResponseWriter, _ *http.Request) {
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "apply-1", "type": "applies",
					"attributes": map[string]any{"status": "errored", "log-read-url": logURL},
				},
			})
		},
	})

	summary, err := client.NewRunSummary(context.Background(), c, "run-1")
	require.NoError(t, err)
	assert.Equal(t, "apply", summary.Phase)
	require.Len(t, summary.Diagnostics, 1)
	assert.Equal(t, "Apply failed", summary.Diagnostics[0].Summary)
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

func TestSampleRunSummary_AllFailureTypes(t *testing.T) {
	t.Parallel()

	summary := sampleRunSummary()

	// Verify all failure types are populated.
	assert.NotEmpty(t, summary.Diagnostics)
	assert.NotEmpty(t, summary.PolicyCheckLog)
	assert.NotEmpty(t, summary.PolicyEvaluations)
	assert.NotEmpty(t, summary.TaskResults)

	// Verify the displayer renders without error and includes all sections.
	io := iostreams.Test()
	d := &summaryDisplayer{summary: summary, io: io}
	result := d.StringPayload(format.Pretty)
	assert.Contains(t, result, "Error:")
	assert.Contains(t, result, "Organization Policy Check:")
	assert.Contains(t, result, "Policy Evaluations:")
	assert.Contains(t, result, "All tasks completed:")
	// Should have dividers between all four sections.
	divider := "――――――――――――"
	assert.GreaterOrEqual(t, strings.Count(result, divider), 3)
}
