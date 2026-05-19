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

func strPtr(s string) *string { return &s }
