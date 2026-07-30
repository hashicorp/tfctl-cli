// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package results

import (
	"time"

	"github.com/hashicorp/tfctl-cli/evals/internal/tasks"
)

const (
	resultSchemaVersion = 1
	suiteName           = "tfctl-skill"
)

// TaskStatus describes the outcome of one task.
type TaskStatus string

// Task result statuses.
const (
	StatusPassed TaskStatus = "passed"
	StatusFailed TaskStatus = "failed"
	StatusError  TaskStatus = "error"
)

// Usage records model token consumption.
type Usage struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
}

// TraceEntry records one observable event in a model and tool exchange.
type TraceEntry struct {
	Type         string   `json:"type"`
	Turn         int      `json:"turn,omitempty"`
	ResponseID   string   `json:"response_id,omitempty"`
	ItemID       string   `json:"item_id,omitempty"`
	ItemType     string   `json:"item_type,omitempty"`
	ItemTypes    []string `json:"item_types,omitempty"`
	Status       string   `json:"status,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Role         string   `json:"role,omitempty"`
	Content      string   `json:"content,omitempty"`
	Name         string   `json:"name,omitempty"`
	Args         []string `json:"args,omitempty"`
	CallID       string   `json:"call_id,omitempty"`
	ExitCode     *int     `json:"exit_code,omitempty"`
	Stdout       string   `json:"stdout,omitempty"`
	Stderr       string   `json:"stderr,omitempty"`
	Error        string   `json:"error,omitempty"`
	InputTokens  int64    `json:"input_tokens,omitempty"`
	OutputTokens int64    `json:"output_tokens,omitempty"`
}

// TaskResult records one evaluated task.
type TaskResult struct {
	ID         string              `json:"id"`
	Filename   string              `json:"filename,omitempty"`
	Tags       []string            `json:"tags,omitempty"`
	Output     string              `json:"output,omitempty"`
	ToolUsage  string              `json:"tool_usage,omitempty"`
	DurationMS int64               `json:"duration_ms"`
	Usage      Usage               `json:"usage"`
	Status     TaskStatus          `json:"status"`
	Error      string              `json:"error,omitempty"`
	Checks     []tasks.CheckResult `json:"checks,omitempty"`
	Trace      []TraceEntry        `json:"trace,omitempty"`
}

// ComparisonStatus classifies a task relative to its baseline.
type ComparisonStatus string

// Baseline comparison statuses.
const (
	ComparisonRegression  ComparisonStatus = "regression"
	ComparisonImprovement ComparisonStatus = "improvement"
	ComparisonUnchanged   ComparisonStatus = "unchanged"
	ComparisonNew         ComparisonStatus = "new"
	ComparisonRemoved     ComparisonStatus = "removed"
)

// ComparisonTask records one task's baseline classification.
type ComparisonTask struct {
	ID     string           `json:"id"`
	Status ComparisonStatus `json:"status"`
}

// Comparison summarizes changes from a baseline result.
type Comparison struct {
	BaselineProvider string           `json:"baseline_provider"`
	BaselineModel    string           `json:"baseline_model"`
	ProviderChanged  bool             `json:"provider_changed"`
	ModelChanged     bool             `json:"model_changed"`
	Regressions      int              `json:"regressions"`
	Improvements     int              `json:"improvements"`
	Unchanged        int              `json:"unchanged"`
	New              int              `json:"new"`
	Removed          int              `json:"removed"`
	Tasks            []ComparisonTask `json:"tasks"`
}

// Result is the versioned evaluation result document.
type Result struct {
	SchemaVersion int          `json:"schema_version"`
	Suite         string       `json:"suite"`
	Provider      string       `json:"provider"`
	Model         string       `json:"model"`
	StartedAt     time.Time    `json:"started_at"`
	DurationMS    int64        `json:"duration_ms"`
	TaskFilter    string       `json:"task_filter,omitempty"`
	TagFilters    []string     `json:"tag_filters,omitempty"`
	Total         int          `json:"total"`
	Passed        int          `json:"passed"`
	Failed        int          `json:"failed"`
	Errors        int          `json:"errors"`
	PassRate      float64      `json:"pass_rate"`
	InputTokens   int64        `json:"input_tokens,omitempty"`
	OutputTokens  int64        `json:"output_tokens,omitempty"`
	Tasks         []TaskResult `json:"tasks"`
	Comparison    *Comparison  `json:"comparison,omitempty"`
}

// New initializes a result for an evaluation run.
func New(provider, model string, started time.Time, taskFilter string, tagFilters []string, total int) Result {
	return Result{
		SchemaVersion: resultSchemaVersion,
		Suite:         suiteName,
		Provider:      provider,
		Model:         model,
		StartedAt:     started,
		TaskFilter:    taskFilter,
		TagFilters:    tagFilters,
		Total:         total,
		Tasks:         make([]TaskResult, 0, total),
	}
}
