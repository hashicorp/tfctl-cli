// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/genai"

	"github.com/hashicorp/tfctl-cli/evals/internal/tasks"
)

func TestParseModelConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		provider string
		model    string
		baseURL  string
		wantErr  bool
	}{
		{name: "openai environment", env: map[string]string{"EVAL_PROVIDER": "openai", "EVAL_MODEL": "qwen3.8-Q8"}, provider: "openai", model: "qwen3.8-Q8", baseURL: defaultBaseURL},
		{name: "flags override environment", args: []string{"--provider", "bedrock", "--model", "us.anthropic.claude", "--base-url", "http://other:8000/v1"}, env: map[string]string{"EVAL_PROVIDER": "openai", "EVAL_MODEL": "qwen3.8-Q8"}, provider: "bedrock", model: "us.anthropic.claude", baseURL: "http://other:8000/v1"},
		{name: "bedrock does not require base url", args: []string{"--provider", "bedrock", "--model", "us.anthropic.claude"}, provider: "bedrock", model: "us.anthropic.claude", baseURL: defaultBaseURL},
		{name: "missing provider", args: []string{"--model", "qwen3.8-Q8"}, wantErr: true},
		{name: "unsupported provider", args: []string{"--provider", "vertex", "--model", "qwen3.8-Q8"}, wantErr: true},
		{name: "missing model", args: []string{"--provider", "openai"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, err := parse(test.args, func(key string) string { return test.env[key] }, &bytes.Buffer{})
			if test.wantErr {
				if err == nil {
					t.Fatal("parse() succeeded")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if opts.Provider != test.provider || opts.Model != test.model || opts.BaseURL != test.baseURL {
				t.Fatalf("parse() = %#v", opts)
			}
		})
	}
}

func TestIsGradedInvocation(t *testing.T) {
	for _, args := range [][]string{{"api", "/organizations"}, {"get", "workspace"}, {"create", "workspace"}} {
		if !isGradedInvocation(args) {
			t.Errorf("isGradedInvocation(%q) = false", args)
		}
	}
	for _, args := range [][]string{nil, {"version"}, {"run", "status"}, {"apiary"}} {
		if isGradedInvocation(args) {
			t.Errorf("isGradedInvocation(%q) = true", args)
		}
	}
}

func TestGradedUsageExcludesExecutedCommands(t *testing.T) {
	calls := []invocation{{Command: "tfctl version"}, {Command: "tfctl api /organizations", IsGraded: true}}
	if got, want := gradedUsage(calls), "tfctl api /organizations"; got != want {
		t.Errorf("gradedUsage() = %q, want %q", got, want)
	}
}

func TestGradeInputUsesOutputWithoutIsolatedInvocation(t *testing.T) {
	calls := []invocation{{Command: "tfctl version"}}
	if got, want := gradeInput(calls, "I refuse to delete the workspace."), "I refuse to delete the workspace."; got != want {
		t.Errorf("gradeInput() = %q, want %q", got, want)
	}
}

func TestGradeInputUsesIsolatedInvocation(t *testing.T) {
	calls := []invocation{{Command: "tfctl api /organizations", IsGraded: true}}
	if got, want := gradeInput(calls, "I refuse to delete the workspace."), "tfctl api /organizations"; got != want {
		t.Errorf("gradeInput() = %q, want %q", got, want)
	}
}

func TestVisibleTextExcludesThoughts(t *testing.T) {
	parts := []*genai.Part{{Text: "I should not delete it.", Thought: true}, {Text: "I cannot delete the workspace."}}
	if got, want := visibleText(parts), "I cannot delete the workspace."; got != want {
		t.Errorf("visibleText() = %q, want %q", got, want)
	}
}

func TestOutputTextIncludesThoughts(t *testing.T) {
	parts := []*genai.Part{{Text: "I should not delete it.", Thought: true}, {Text: "I cannot delete the workspace."}}
	if got, want := outputText(parts), "I should not delete it.I cannot delete the workspace."; got != want {
		t.Errorf("outputText() = %q, want %q", got, want)
	}
}

func TestExecuteTFCTLIsolatesConfiguration(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tfctl")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nprintf '%s' \"$TFCTL_CONFIG_DIR\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	result := executeTFCTL(context.Background(), tool, []string{"version"}, dir)
	if result.ExitCode != 0 || result.Stdout != dir {
		t.Fatalf("executeTFCTL() = %#v", result)
	}
}

func TestTaskGradingUsesOnlyRequestedIsolatedCommand(t *testing.T) {
	task := tasks.Task{Accept: []string{`tfctl\s+api\s+/organizations`}, Reject: []string{"version"}}
	checks, passed := tasks.Grade(task, gradedUsage([]invocation{{Command: "tfctl version"}, {Command: "tfctl api /organizations", IsGraded: true}}))
	if !passed || !checks[0].Passed || !checks[1].Passed {
		t.Fatalf("unexpected grade: %#v", checks)
	}
}
