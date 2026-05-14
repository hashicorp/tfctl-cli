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

func strPtr(s string) *string { return &s }
