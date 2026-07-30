// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/evals/internal/provider"
	"github.com/hashicorp/tfctl-cli/evals/internal/results"
	"github.com/hashicorp/tfctl-cli/evals/internal/tasks"
)

type fakeResponder struct {
	responses []provider.Response
	errors    []error
	requests  []provider.Request
}

type responderFunc func(context.Context, provider.Request) (provider.Response, error)

func (f responderFunc) Respond(ctx context.Context, request provider.Request) (provider.Response, error) {
	return f(ctx, request)
}

func (f *fakeResponder) Respond(_ context.Context, req provider.Request) (provider.Response, error) {
	f.requests = append(f.requests, req)
	i := len(f.requests) - 1
	if i < len(f.errors) && f.errors[i] != nil {
		return provider.Response{}, f.errors[i]
	}
	return f.responses[i], nil
}

func TestParseOptionsFlagsOverrideEnvironment(t *testing.T) {
	env := map[string]string{
		"EVAL_PROVIDER": "openai", "EVAL_MODEL": "env-model", "EVAL_OUTPUT": "env.json",
		"EVAL_TAGS": "env-tag", "EVAL_JSON": "false", "EVAL_COMPARE": "base.json", "EVAL_TASK": "env-*",
	}
	getenv := func(key string) string { return env[key] }

	opts, err := parseOptions([]string{"--provider", "bedrock", "--model", "flag-model", "--output", "flag.json", "--tags", "one,two", "--json", "--compare", "other.json", "--task", "flag-*"}, getenv, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, "bedrock", opts.Provider)
	require.Equal(t, "flag-model", opts.Model)
	require.Equal(t, "flag.json", opts.OutputPath)
	require.Equal(t, []string{"one", "two"}, opts.Tags)
	require.True(t, opts.JSON)
	require.Equal(t, "other.json", opts.ComparePath)
	require.Equal(t, "flag-*", opts.TaskGlob)
	require.Equal(t, "tasks", opts.TasksDir)
	require.Equal(t, "../skills/tfctl/SKILL.md", opts.SkillPath)
}

func TestRunEvalContinuesAfterProviderError(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "tasks")
	require.NoError(t, os.Mkdir(taskDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "01-first.yaml"), []byte("task: first\naccept: [ok]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "02-second.yaml"), []byte("task: second\naccept: ['tfctl\\s+api\\s+/(?:account/details|organizations)']\n"), 0o600))
	skillPath := filepath.Join(root, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("instructions"), 0o600))

	fake := &fakeResponder{
		responses: []provider.Response{{}, {
			Output: "OK", InputTokens: 2, OutputTokens: 1,
			Trace: []provider.TraceEntry{{Type: provider.TraceToolUse, Name: "tfctl", Args: []string{"api", "/account/details"}}},
		}},
		errors: []error{errors.New("provider unavailable")},
	}
	var stdout bytes.Buffer
	result, err := runEval(context.Background(), evalOptions{
		Provider: "bedrock", Model: "model", TasksDir: taskDir, SkillPath: skillPath,
		Stdout: &stdout, Responder: fake, Now: time.Now, TaskTimeout: time.Second,
	})
	require.NoError(t, err)
	require.Len(t, fake.requests, 2)
	require.Equal(t, "instructions", fake.requests[0].Instructions)
	require.Equal(t, "first", fake.requests[0].Input)
	require.Equal(t, 10, fake.requests[0].Turns)
	require.Equal(t, 1, result.Errors)
	require.Equal(t, 1, result.Passed)
	require.Equal(t, []tasks.CheckResult{{Type: tasks.CheckAccept, Expected: "ok", Passed: false}}, result.Tasks[0].Checks)
	require.Equal(t, int64(2), result.InputTokens)
	require.Equal(t, int64(1), result.OutputTokens)
	require.Equal(t, []results.TraceEntry{{Type: "tool_use", Turn: 1, Name: "tfctl", Args: []string{"api", "/account/details"}}}, result.Tasks[1].Trace)
	require.Equal(t, "tfctl api /account/details", result.Tasks[1].ToolUsage)
}

func TestRunEvalExecutesToolAndReturnsResultToAssistant(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "tasks")
	require.NoError(t, os.Mkdir(taskDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "01-task.yaml"), []byte("task: inspect\naccept: [version]\nturns: 3\n"), 0o600))
	skillPath := filepath.Join(root, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("instructions"), 0o600))
	toolPath := filepath.Join(root, "tfctl")
	require.NoError(t, os.WriteFile(toolPath, []byte("#!/bin/sh\nprintf 'version output'\nprintf 'warning' >&2\nexit 7\n"), 0o700))

	fake := &fakeResponder{responses: []provider.Response{
		{
			ID: "resp_1", InputTokens: 2, OutputTokens: 3, State: "state_1",
			ToolCalls: []provider.ToolCall{{CallID: "call_1", Name: "tfctl", Args: []string{"version"}}},
			Trace: []provider.TraceEntry{
				{Type: provider.TraceMessage, Role: "user", Content: "inspect"},
				{Type: provider.TraceResponse, ResponseID: "resp_1", Status: "completed", InputTokens: 2, OutputTokens: 3, ItemTypes: []string{"function_call"}},
				{Type: provider.TraceToolUse, ItemID: "item_1", CallID: "call_1", Name: "tfctl", Args: []string{"version"}},
			},
		},
		{
			ID: "resp_2", Output: "finished", InputTokens: 5, OutputTokens: 1,
			Trace: []provider.TraceEntry{
				{Type: provider.TraceResponse, ResponseID: "resp_2", Status: "completed", InputTokens: 5, OutputTokens: 1, ItemTypes: []string{"message"}},
				{Type: provider.TraceMessage, ItemID: "item_2", Role: "assistant", Content: "finished"},
			},
		},
	}}

	result, err := runEval(context.Background(), evalOptions{
		Provider: "bedrock", Model: "model", TasksDir: taskDir, SkillPath: skillPath,
		ToolPath: toolPath, Stdout: &bytes.Buffer{}, Responder: fake, Now: time.Now,
	})
	require.NoError(t, err)
	require.Len(t, fake.requests, 2)
	require.Equal(t, "inspect", fake.requests[0].Input)
	require.Empty(t, fake.requests[0].ToolResults)
	require.Equal(t, "state_1", fake.requests[1].State)
	require.Equal(t, []provider.ToolResult{{
		CallID: "call_1", Output: `{"exit_code":7,"stdout":"version output","stderr":"warning"}`,
		ExitCode: 7, Stdout: "version output", Stderr: "warning",
	}}, fake.requests[1].ToolResults)
	require.Equal(t, int64(7), result.InputTokens)
	require.Equal(t, int64(4), result.OutputTokens)
	require.Equal(t, "finished", result.Tasks[0].Output)
	require.Equal(t, "tfctl version", result.Tasks[0].ToolUsage)
	require.Equal(t, results.StatusPassed, result.Tasks[0].Status)
	exitCode := 7
	require.Equal(t, []results.TraceEntry{
		{Type: "message", Turn: 1, Role: "user", Content: "inspect"},
		{Type: "response", Turn: 1, ResponseID: "resp_1", Status: "completed", InputTokens: 2, OutputTokens: 3, ItemTypes: []string{"function_call"}},
		{Type: "tool_use", Turn: 1, ItemID: "item_1", CallID: "call_1", Name: "tfctl", Args: []string{"version"}},
		{Type: "tool_result", Turn: 1, CallID: "call_1", Name: "tfctl", ExitCode: &exitCode, Stdout: "version output", Stderr: "warning"},
		{Type: "response", Turn: 2, ResponseID: "resp_2", Status: "completed", InputTokens: 5, OutputTokens: 1, ItemTypes: []string{"message"}},
		{Type: "message", Turn: 2, ItemID: "item_2", Role: "assistant", Content: "finished"},
	}, result.Tasks[0].Trace)
}

func TestRunEvalFailsWhenMaximumTurnsAreExhausted(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "tasks")
	require.NoError(t, os.Mkdir(taskDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "01-task.yaml"), []byte("task: inspect\naccept: [version]\nturns: 1\n"), 0o600))
	skillPath := filepath.Join(root, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("instructions"), 0o600))

	fake := &fakeResponder{responses: []provider.Response{{
		ID:        "resp_1",
		ToolCalls: []provider.ToolCall{{CallID: "call_1", Name: "tfctl", Args: []string{"version"}}},
	}}}
	result, err := runEval(context.Background(), evalOptions{
		Provider: "bedrock", Model: "model", TasksDir: taskDir, SkillPath: skillPath,
		ToolPath: "/bin/true", Stdout: &bytes.Buffer{}, Responder: fake, Now: time.Now,
	})
	require.NoError(t, err)
	require.Len(t, fake.requests, 1)
	require.Equal(t, results.StatusFailed, result.Tasks[0].Status)
	require.ErrorContains(t, errors.New(result.Tasks[0].Error), "maximum turns exhausted")
	require.Equal(t, "error", result.Tasks[0].Trace[len(result.Tasks[0].Trace)-1].Type)
	require.Equal(t, 1, result.Tasks[0].Trace[len(result.Tasks[0].Trace)-1].Turn)
	require.Contains(t, result.Tasks[0].Trace[len(result.Tasks[0].Trace)-1].Error, "maximum turns exhausted")
}

func TestRunEvalHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runEval(ctx, evalOptions{TasksDir: t.TempDir(), SkillPath: "missing", Stdout: &bytes.Buffer{}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunEvalSavesCurrentResultWhenBaselineIsInvalid(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "tasks")
	require.NoError(t, os.Mkdir(taskDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "01-task.yaml"), []byte("task: prompt\naccept: [ok]\n"), 0o600))
	skillPath := filepath.Join(root, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("instructions"), 0o600))
	baselinePath := filepath.Join(root, "baseline.json")
	require.NoError(t, os.WriteFile(baselinePath, []byte(`{"schema_version":99}`), 0o600))
	outputPath := filepath.Join(root, "current.json")

	_, err := runEval(context.Background(), evalOptions{
		Provider: "bedrock", Model: "model", TasksDir: taskDir, SkillPath: skillPath,
		Stdout: &bytes.Buffer{}, Responder: &fakeResponder{responses: []provider.Response{{
			Trace: []provider.TraceEntry{{Type: provider.TraceToolUse, Name: "tfctl", Args: []string{"ok"}}},
		}}},
		ComparePath: baselinePath, OutputPath: outputPath, Now: time.Now,
	})
	require.ErrorContains(t, err, "unsupported result schema")
	result, loadErr := results.Load(outputPath)
	require.NoError(t, loadErr)
	require.Equal(t, 1, result.Passed)
	require.Nil(t, result.Comparison)
}

func TestRunEvalContinuesAfterTaskTimeout(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "tasks")
	require.NoError(t, os.Mkdir(taskDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "01-first.yaml"), []byte("task: first\naccept: [ok]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "02-second.yaml"), []byte("task: second\naccept: [ok]\n"), 0o600))
	skillPath := filepath.Join(root, "SKILL.md")
	require.NoError(t, os.WriteFile(skillPath, []byte("instructions"), 0o600))
	calls := 0
	responder := responderFunc(func(ctx context.Context, _ provider.Request) (provider.Response, error) {
		calls++
		if calls == 1 {
			<-ctx.Done()
			return provider.Response{}, ctx.Err()
		}
		return provider.Response{Trace: []provider.TraceEntry{{Type: provider.TraceToolUse, Name: "tfctl", Args: []string{"ok"}}}}, nil
	})

	result, err := runEval(context.Background(), evalOptions{
		Provider: "bedrock", Model: "model", TasksDir: taskDir, SkillPath: skillPath,
		Stdout: &bytes.Buffer{}, Responder: responder, Now: time.Now, TaskTimeout: time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, result.Errors)
	require.Equal(t, 1, result.Passed)
}

func TestExitCodeSemantics(t *testing.T) {
	require.Equal(t, 1, resultExitCode(results.Result{Failed: 1}, false))
	require.Equal(t, 0, resultExitCode(results.Result{Failed: 1, Comparison: &results.Comparison{}}, true))
	require.Equal(t, 1, resultExitCode(results.Result{Comparison: &results.Comparison{Regressions: 1}}, true))
	require.Equal(t, 1, resultExitCode(results.Result{Errors: 1, Comparison: &results.Comparison{}}, true))
}
