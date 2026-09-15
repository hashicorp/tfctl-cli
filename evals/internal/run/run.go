// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package run implements the skill evaluation command.
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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"

	"github.com/hashicorp/tfctl-cli/evals/internal/model/bedrockconverse"
	"github.com/hashicorp/tfctl-cli/evals/internal/model/openaichat"
	"github.com/hashicorp/tfctl-cli/evals/internal/tasks"
)

const (
	defaultBaseURL = "http://127.0.0.1:8000/v1"
	defaultTool    = "tfctl"
	defaultTimeout = 10 * time.Minute

	providerOpenAI  = "openai"
	providerBedrock = "bedrock"
)

type options struct {
	Provider  string
	Model     string
	BaseURL   string
	APIKey    string
	Output    string
	Tags      []string
	JSON      bool
	Task      string
	TasksDir  string
	SkillPath string
	ToolPath  string
	Timeout   time.Duration
	Stdout    io.Writer
}

type toolInput struct {
	Args []string `json:"args" jsonschema:"Arguments to pass to tfctl."`
}

type toolOutput struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
	Stopped  bool   `json:"stopped,omitempty"`
}

type invocation struct {
	Args     []string `json:"args"`
	Command  string   `json:"command"`
	IsGraded bool     `json:"is_graded"`
	ExitCode int      `json:"exit_code,omitempty"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type taskResult struct {
	ID          string              `json:"id"`
	Filename    string              `json:"filename"`
	Status      string              `json:"status"`
	Invocations []invocation        `json:"invocations"`
	Checks      []tasks.CheckResult `json:"checks"`
	Output      string              `json:"output,omitempty"`
	Error       string              `json:"error,omitempty"`
}

type result struct {
	Model string       `json:"model"`
	Tasks []taskResult `json:"tasks"`
}

func parse(args []string, getenv func(string) string, stderr io.Writer) (options, error) {
	opts := options{
		BaseURL: defaultBaseURL, TasksDir: "tasks", SkillPath: "../skills/tfctl/SKILL.md",
		ToolPath: defaultTool, Timeout: defaultTimeout,
	}
	if value := getenv("EVAL_PROVIDER"); value != "" {
		opts.Provider = value
	}
	if value := getenv("EVAL_MODEL"); value != "" {
		opts.Model = value
	}
	if value := getenv("EVAL_BASE_URL"); value != "" {
		opts.BaseURL = value
	}
	if value := getenv("EVAL_API_KEY"); value != "" {
		opts.APIKey = value
	}
	if value := getenv("EVAL_OUTPUT"); value != "" {
		opts.Output = value
	}
	if value := getenv("EVAL_TASK"); value != "" {
		opts.Task = value
	}
	var tagValue string
	flags := flag.NewFlagSet("evals", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.Provider, "provider", opts.Provider, `model provider: "openai" (any OpenAI-compatible Chat Completions endpoint) or "bedrock" (AWS Bedrock Converse API)`)
	flags.StringVar(&opts.Model, "model", opts.Model, "model name (openai provider) or Bedrock model/inference-profile ID (bedrock provider)")
	flags.StringVar(&opts.BaseURL, "base-url", opts.BaseURL, "OpenAI-compatible Chat Completions base URL (openai provider only)")
	flags.StringVar(&opts.APIKey, "api-key", opts.APIKey, "API key for the OpenAI-compatible endpoint, if required (openai provider only)")
	flags.StringVar(&opts.Output, "output", opts.Output, "path for the JSON result")
	flags.StringVar(&opts.Task, "task", opts.Task, "task filename glob or substring")
	flags.StringVar(&tagValue, "tags", getenv("EVAL_TAGS"), "comma-separated task tags")
	flags.BoolVar(&opts.JSON, "json", false, "render JSON to stdout")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	opts.Provider = strings.ToLower(strings.TrimSpace(opts.Provider))
	switch opts.Provider {
	case providerOpenAI, providerBedrock:
	case "":
		return options{}, errors.New("provider must be set with --provider or EVAL_PROVIDER (openai or bedrock)")
	default:
		return options{}, fmt.Errorf("unsupported provider %q (want %q or %q)", opts.Provider, providerOpenAI, providerBedrock)
	}
	if strings.TrimSpace(opts.Model) == "" {
		return options{}, errors.New("model must be set with --model or EVAL_MODEL")
	}
	if opts.Provider == providerOpenAI && strings.TrimSpace(opts.BaseURL) == "" {
		return options{}, errors.New("base URL must not be empty for the openai provider")
	}
	for _, tag := range strings.Split(tagValue, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			opts.Tags = append(opts.Tags, tag)
		}
	}
	return opts, nil
}

// Main executes the evaluation command and returns its process exit code.
func Main(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, err := parse(args, getenv, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "evals: %v\n", err)
		return 2
	}
	opts.Stdout = stdout
	if err := evaluate(ctx, opts); err != nil {
		fmt.Fprintf(stderr, "evals: %v\n", err)
		return 2
	}
	return 0
}

func newModel(ctx context.Context, opts options) (model.LLM, error) {
	switch opts.Provider {
	case providerOpenAI:
		return openaichat.NewModel(ctx, opts.Model, &openaichat.ClientConfig{APIKey: opts.APIKey, BaseURL: opts.BaseURL})
	case providerBedrock:
		return bedrockconverse.NewModel(ctx, opts.Model)
	default:
		return nil, fmt.Errorf("unsupported provider %q", opts.Provider)
	}
}

func evaluate(ctx context.Context, opts options) error {
	loaded, err := tasks.Load(opts.TasksDir)
	if err != nil {
		return err
	}
	loaded, err = tasks.Filter(loaded, opts.Task, opts.Tags)
	if err != nil {
		return err
	}
	if len(loaded) == 0 {
		return errors.New("no tasks matched the configured filters")
	}
	instructions, err := os.ReadFile(opts.SkillPath)
	if err != nil {
		return fmt.Errorf("read skill instructions: %w", err)
	}
	model, err := newModel(ctx, opts)
	if err != nil {
		return fmt.Errorf("configure model: %w", err)
	}

	output := result{Model: opts.Model, Tasks: make([]taskResult, 0, len(loaded))}
	for _, task := range loaded {
		taskResult, err := evaluateTask(ctx, task, string(instructions), model, opts)
		if err != nil {
			taskResult.Error = err.Error()
			taskResult.Status = "error"
		}
		output.Tasks = append(output.Tasks, taskResult)
	}
	if opts.Output != "" {
		if err := os.MkdirAll(filepath.Dir(opts.Output), 0o755); err != nil {
			return fmt.Errorf("create result directory: %w", err)
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("encode result: %w", err)
		}
		if err := os.WriteFile(opts.Output, append(data, '\n'), 0o600); err != nil {
			return fmt.Errorf("write result: %w", err)
		}
	}
	if opts.JSON {
		return json.NewEncoder(opts.Stdout).Encode(output)
	}
	for _, task := range output.Tasks {
		if _, err := fmt.Fprintf(opts.Stdout, "%s %s\n", strings.ToUpper(task.Status), task.ID); err != nil {
			return err
		}
	}
	for _, task := range output.Tasks {
		if task.Status != "passed" {
			return fmt.Errorf("task %q %s", task.ID, task.Status)
		}
	}
	return nil
}

func evaluateTask(parent context.Context, task tasks.Task, instructions string, llm model.LLM, opts options) (taskResult, error) {
	result := taskResult{ID: task.ID, Filename: task.Filename}
	taskCtx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()
	tmpDir, err := os.MkdirTemp("", "tfctl-eval-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmpDir)

	var mu sync.Mutex
	var calls []invocation
	var gradeableOutput strings.Builder
	stop := false
	turns := 0
	limitTurns := func(_ agent.Context, _ *model.LLMRequest) (*model.LLMResponse, error) {
		turns++
		if turns > task.Turns {
			return nil, fmt.Errorf("maximum turns exhausted after %d turns", task.Turns)
		}
		return nil, nil
	}
	execute := func(_ agent.Context, input toolInput) (toolOutput, error) {
		graded := isGradedInvocation(input.Args)
		call := invocation{Args: append([]string(nil), input.Args...), Command: "tfctl " + strings.Join(input.Args, " "), IsGraded: graded}
		if graded {
			mu.Lock()
			calls = append(calls, call)
			stop = true
			mu.Unlock()
			cancel()
			return toolOutput{Stopped: true}, nil
		}
		output := executeTFCTL(taskCtx, opts.ToolPath, input.Args, tmpDir)
		call.ExitCode, call.Stdout, call.Stderr, call.Error = output.ExitCode, output.Stdout, output.Stderr, output.Error
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		return output, nil
	}
	tfctlTool, err := functiontool.New(functiontool.Config{Name: "tfctl", Description: "Run a tfctl command with the supplied arguments."}, execute)
	if err != nil {
		return result, fmt.Errorf("create tfctl tool: %w", err)
	}
	bot, err := llmagent.New(llmagent.Config{
		Name: "tfctl_skill_evaluator", Description: "Uses tfctl to complete a task.",
		InstructionProvider:  func(agent.ReadonlyContext) (string, error) { return instructions, nil },
		Model:                llm,
		Tools:                []tool.Tool{tfctlTool},
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{limitTurns},
	})
	if err != nil {
		return result, fmt.Errorf("create evaluator agent: %w", err)
	}
	r, err := runner.NewInMemory("tfctl-skill-evals", bot)
	if err != nil {
		return result, fmt.Errorf("create evaluator runner: %w", err)
	}
	for event, err := range r.Run(taskCtx, "evaluator", task.ID, genai.NewContentFromText(task.Prompt+"\n\n Use tfctl to complete this task.", genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			if stop && errors.Is(err, context.Canceled) {
				break
			}
			return result, err
		}
		if event != nil && event.Content != nil {
			result.Output += outputText(event.Content.Parts)
			gradeableOutput.WriteString(visibleText(event.Content.Parts))
		}
	}
	mu.Lock()
	result.Invocations = append(result.Invocations, calls...)
	mu.Unlock()
	usage := gradeInput(result.Invocations, gradeableOutput.String())
	var passed bool
	result.Checks, passed = tasks.Grade(task, usage)
	if !stop {
		if passed {
			result.Status = "passed"
		} else {
			result.Status = "failed"
		}
		return result, nil
	}
	if passed {
		result.Status = "passed"
	} else {
		result.Status = "failed"
	}
	return result, nil
}

func isGradedInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "api":
		return len(args) > 1 && args[1] != "schema"
	case "get", "create":
		return true
	default:
		return false
	}
}

func gradedUsage(calls []invocation) string {
	var commands []string
	for _, call := range calls {
		if call.IsGraded {
			commands = append(commands, call.Command)
		}
	}
	return strings.Join(commands, "\n")
}

func gradeInput(calls []invocation, output string) string {
	if usage := gradedUsage(calls); usage != "" {
		return usage
	}
	return output
}

func visibleText(parts []*genai.Part) string {
	var output strings.Builder
	for _, part := range parts {
		if !part.Thought {
			output.WriteString(part.Text)
		}
	}
	return output.String()
}

func outputText(parts []*genai.Part) string {
	var output strings.Builder
	for _, part := range parts {
		output.WriteString(part.Text)
	}
	return output.String()
}

func executeTFCTL(ctx context.Context, toolPath string, args []string, configDir string) toolOutput {
	command := exec.CommandContext(ctx, toolPath, args...)
	command.Env = append(os.Environ(), "TFCTL_TOKEN=fake-token", "TFCTL_CONFIG_DIR="+configDir)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	result := toolOutput{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result
	}
	result.ExitCode = -1
	result.Error = err.Error()
	return result
}
