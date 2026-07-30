// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package run implements the eval command.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/tfctl-cli/evals/internal/provider"
	"github.com/hashicorp/tfctl-cli/evals/internal/results"
	"github.com/hashicorp/tfctl-cli/evals/internal/tasks"
)

const (
	defaultTaskTimeout = 10 * time.Minute
	defaultProvider    = "bedrock"
	defaultModel       = "openai.gpt-5.6-luna"
	defaultToolPath    = "tfctl"
)

var errMaximumTurns = errors.New("maximum turns exhausted")

type evalOptions struct {
	Provider    string
	Model       string
	OutputPath  string
	Tags        []string
	JSON        bool
	ComparePath string
	TaskGlob    string
	TasksDir    string
	SkillPath   string
	Stdout      io.Writer
	Responder   provider.Responder
	Now         func() time.Time
	TaskTimeout time.Duration
	ToolPath    string
}

func parseOptions(args []string, getenv func(string) string, stderr io.Writer) (evalOptions, error) {
	opts := evalOptions{
		Provider:    envOrDefault(getenv, "EVAL_PROVIDER", defaultProvider),
		Model:       envOrDefault(getenv, "EVAL_MODEL", defaultModel),
		OutputPath:  getenv("EVAL_OUTPUT"),
		ComparePath: getenv("EVAL_COMPARE"),
		TaskGlob:    getenv("EVAL_TASK"),
		TasksDir:    "tasks",
		SkillPath:   "../skills/tfctl/SKILL.md",
		Stdout:      io.Discard,
		Now:         time.Now,
		TaskTimeout: defaultTaskTimeout,
		ToolPath:    defaultToolPath,
	}
	tagValue := getenv("EVAL_TAGS")
	jsonValue := getenv("EVAL_JSON")
	if jsonValue != "" {
		parsed, err := strconv.ParseBool(jsonValue)
		if err != nil {
			return evalOptions{}, fmt.Errorf("invalid EVAL_JSON value %q: %w", jsonValue, err)
		}
		opts.JSON = parsed
	}

	flags := flag.NewFlagSet("evals", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.Provider, "provider", opts.Provider, "Responses API provider (bedrock or openai)")
	flags.StringVar(&opts.Model, "model", opts.Model, "model to evaluate")
	flags.StringVar(&opts.OutputPath, "output", opts.OutputPath, "path for the complete JSON result")
	flags.StringVar(&tagValue, "tags", tagValue, "comma-separated task tags")
	flags.BoolVar(&opts.JSON, "json", opts.JSON, "render JSON to stdout")
	flags.StringVar(&opts.ComparePath, "compare", opts.ComparePath, "baseline result to compare")
	flags.StringVar(&opts.TaskGlob, "task", opts.TaskGlob, "task filename glob or substring")
	if err := flags.Parse(args); err != nil {
		return evalOptions{}, err
	}
	if flags.NArg() != 0 {
		return evalOptions{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if opts.Provider != "bedrock" && opts.Provider != "openai" {
		return evalOptions{}, fmt.Errorf("unsupported provider %q", opts.Provider)
	}
	if strings.TrimSpace(opts.Model) == "" {
		return evalOptions{}, fmt.Errorf("model must not be empty")
	}
	opts.Tags = splitTags(tagValue)
	return opts, nil
}

func envOrDefault(getenv func(string) string, key, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}

func splitTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		if tag := strings.TrimSpace(part); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func runEval(ctx context.Context, opts evalOptions) (results.Result, error) {
	if err := ctx.Err(); err != nil {
		return results.Result{}, err
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = defaultTaskTimeout
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.ToolPath == "" {
		opts.ToolPath = defaultToolPath
	}
	started := opts.Now()
	loadedTasks, err := tasks.Load(opts.TasksDir)
	if err != nil {
		return results.Result{}, err
	}
	loadedTasks, err = tasks.Filter(loadedTasks, opts.TaskGlob, opts.Tags)
	if err != nil {
		return results.Result{}, err
	}
	if len(loadedTasks) == 0 {
		return results.Result{}, fmt.Errorf("no tasks matched the configured filters")
	}
	instructions, err := os.ReadFile(opts.SkillPath)
	if err != nil {
		return results.Result{}, fmt.Errorf("read skill instructions: %w", err)
	}
	if opts.Responder == nil {
		opts.Responder, err = provider.New(ctx, opts.Provider)
		if err != nil {
			return results.Result{}, fmt.Errorf("configure provider: %w", err)
		}
	}

	result := results.New(opts.Provider, opts.Model, started, opts.TaskGlob, opts.Tags, len(loadedTasks))
	for _, task := range loadedTasks {
		if err := ctx.Err(); err != nil {
			return results.Result{}, err
		}
		taskStarted := opts.Now()
		taskCtx, cancel := context.WithTimeout(ctx, opts.TaskTimeout)
		var response provider.Response
		var responseErr error
		var traceEntries []provider.TraceEntry
		var inputTokens, outputTokens int64
		request := provider.Request{
			Model: opts.Model, Instructions: string(instructions), Input: task.Prompt, Turns: task.Turns,
		}
		tmpDir, err := os.MkdirTemp("", "tfctl-task-*")
		if err != nil {
			cancel()
			return results.Result{}, fmt.Errorf("failed to create temporary directory for task: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		for turn := 0; turn < task.Turns; turn++ {
			response, responseErr = opts.Responder.Respond(taskCtx, request)
			for i := range response.Trace {
				response.Trace[i].Turn = turn + 1
			}
			traceEntries = append(traceEntries, response.Trace...)
			inputTokens += response.InputTokens
			outputTokens += response.OutputTokens
			if responseErr != nil {
				traceEntries = append(traceEntries, provider.TraceEntry{Type: provider.TraceError, Turn: turn + 1, Error: responseErr.Error()})
				break
			}
			if len(response.ToolCalls) == 0 {
				break
			}

			toolResults := make([]provider.ToolResult, len(response.ToolCalls))
			for i, call := range response.ToolCalls {
				toolResults[i] = executeTool(taskCtx, opts.ToolPath, call, tmpDir)
				exitCode := toolResults[i].ExitCode
				traceEntries = append(traceEntries, provider.TraceEntry{
					Type: provider.TraceToolResult, Turn: turn + 1, CallID: call.CallID, Name: call.Name,
					ExitCode: &exitCode, Stdout: toolResults[i].Stdout, Stderr: toolResults[i].Stderr, Error: toolResults[i].Error,
				})
			}
			request.Input = ""
			request.ToolResults = toolResults
			request.State = response.State
			if turn == task.Turns-1 {
				if err := taskCtx.Err(); err != nil {
					responseErr = err
				} else {
					responseErr = fmt.Errorf("%w after %d turns", errMaximumTurns, task.Turns)
				}
				traceEntries = append(traceEntries, provider.TraceEntry{
					Type: provider.TraceError, Turn: turn + 1, Error: responseErr.Error(),
				})
			}
		}
		cancel()
		if err := ctx.Err(); err != nil {
			return results.Result{}, err
		}
		trace := make([]results.TraceEntry, len(traceEntries))
		for i, entry := range traceEntries {
			trace[i] = results.TraceEntry{
				Type: entry.Type, Turn: entry.Turn, ResponseID: entry.ResponseID,
				ItemID: entry.ItemID, ItemType: entry.ItemType, ItemTypes: entry.ItemTypes, Status: entry.Status,
				Summary: entry.Summary, Role: entry.Role, Content: entry.Content, Name: entry.Name, Args: entry.Args, CallID: entry.CallID,
				ExitCode: entry.ExitCode, Stdout: entry.Stdout, Stderr: entry.Stderr, Error: entry.Error,
				InputTokens: entry.InputTokens, OutputTokens: entry.OutputTokens,
			}
		}
		toolUsage := renderToolUsage(traceEntries)
		taskResult := results.TaskResult{
			ID: task.ID, Filename: task.Filename, Tags: task.Tags,
			Output:     response.Output,
			ToolUsage:  toolUsage,
			DurationMS: opts.Now().Sub(taskStarted).Milliseconds(),
			Usage:      results.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
			Trace:      trace,
		}
		taskResult.Checks, _ = tasks.Grade(task, toolUsage)
		result.InputTokens += inputTokens
		result.OutputTokens += outputTokens
		if responseErr != nil {
			taskResult.Error = responseErr.Error()
			if errors.Is(responseErr, errMaximumTurns) {
				taskResult.Status = results.StatusFailed
				result.Failed++
			} else {
				taskResult.Status = results.StatusError
				result.Errors++
			}
		} else {
			if allChecksPassed(taskResult.Checks) {
				taskResult.Status = results.StatusPassed
				result.Passed++
			} else {
				taskResult.Status = results.StatusFailed
				result.Failed++
			}
		}
		result.Tasks = append(result.Tasks, taskResult)
	}
	if result.Total > 0 {
		result.PassRate = float64(result.Passed) / float64(result.Total)
	}
	result.DurationMS = opts.Now().Sub(started).Milliseconds()

	if opts.ComparePath != "" {
		baseline, err := results.Load(opts.ComparePath)
		if err != nil {
			if opts.OutputPath != "" {
				if saveErr := results.Save(opts.OutputPath, result); saveErr != nil {
					return results.Result{}, saveErr
				}
			}
			return results.Result{}, fmt.Errorf("load comparison baseline: %w", err)
		}
		filtered := opts.TaskGlob != "" || len(opts.Tags) > 0
		comparison := results.Compare(baseline, result, filtered)
		result.Comparison = &comparison
	}
	if opts.OutputPath != "" {
		if err := results.Save(opts.OutputPath, result); err != nil {
			return results.Result{}, err
		}
	}
	if err := renderResult(opts.Stdout, result, opts.JSON); err != nil {
		return results.Result{}, err
	}
	return result, nil
}

type toolExecutionOutput struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

func executeTool(ctx context.Context, toolPath string, call provider.ToolCall, tmpDir string) provider.ToolResult {
	result := toolExecutionOutput{ExitCode: -1}
	if call.Name != "tfctl" {
		result.Error = fmt.Sprintf("unsupported tool %q", call.Name)
		encoded, _ := json.Marshal(result)
		return provider.ToolResult{CallID: call.CallID, Output: string(encoded), ExitCode: result.ExitCode, Error: result.Error}
	}

	var stdout, stderr strings.Builder
	command := exec.CommandContext(ctx, toolPath, call.Args...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.Env = []string{"TFCTL_TOKEN=fake-token", fmt.Sprintf("TFCTL_CONFIG_DIR=%s", tmpDir)}
	err := command.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if err == nil {
		result.ExitCode = 0
	} else if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.Error = err.Error()
	}
	encoded, _ := json.Marshal(result)
	return provider.ToolResult{
		CallID: call.CallID, Output: string(encoded), ExitCode: result.ExitCode,
		Stdout: result.Stdout, Stderr: result.Stderr, Error: result.Error,
	}
}

func renderToolUsage(trace []provider.TraceEntry) string {
	var commands []string
	for _, entry := range trace {
		if entry.Type == provider.TraceToolUse {
			commands = append(commands, strings.TrimSpace(entry.Name+" "+strings.Join(entry.Args, " ")))
		}
	}
	return strings.Join(commands, "\n")
}

func allChecksPassed(checks []tasks.CheckResult) bool {
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func renderResult(w io.Writer, result results.Result, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("render JSON result: %w", err)
		}
		return nil
	}
	for _, task := range result.Tasks {
		if _, err := fmt.Fprintf(w, "%-12s %s\n", strings.ToUpper(string(task.Status)), task.ID); err != nil {
			return fmt.Errorf("render result: %w", err)
		}
	}
	if _, err := fmt.Fprintf(w, "\n%d/%d passed (%.1f%%), %d failed, %d errors\n", result.Passed, result.Total, result.PassRate*100, result.Failed, result.Errors); err != nil {
		return fmt.Errorf("render summary: %w", err)
	}
	if result.Comparison != nil {
		if _, err := fmt.Fprintf(w, "Comparison: %d regressions, %d improvements, %d new, %d removed\n", result.Comparison.Regressions, result.Comparison.Improvements, result.Comparison.New, result.Comparison.Removed); err != nil {
			return fmt.Errorf("render comparison: %w", err)
		}
	}
	return nil
}

func resultExitCode(result results.Result, comparing bool) int {
	if result.Errors > 0 {
		return 1
	}
	if comparing {
		if result.Comparison != nil && result.Comparison.Regressions > 0 {
			return 1
		}
		return 0
	}
	if result.Failed > 0 {
		return 1
	}
	return 0
}

// Main executes the eval command and returns its process exit code.
func Main(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, getenv, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "evals: %v\n", err)
		return 2
	}
	opts.Stdout = stdout
	result, err := runEval(ctx, opts)
	if err != nil {
		fmt.Fprintf(stderr, "evals: %v\n", err)
		return 2
	}
	return resultExitCode(result, opts.ComparePath != "")
}
