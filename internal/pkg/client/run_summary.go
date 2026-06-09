// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/hashicorp/go-tfe/v2/api/models"
	"github.com/microsoft/kiota-abstractions-go/serialization"
)

// RunSummary is the result of inspecting a Terraform Cloud run.
type RunSummary struct {
	RunID             string             `json:"run_id"`
	Status            string             `json:"status"`
	Message           string             `json:"message"`
	Phase             string             `json:"phase,omitempty"`
	Diagnostics       []Diagnostic       `json:"diagnostics,omitempty"`
	RawLog            string             `json:"raw_log,omitempty"`
	PolicyCheckLog    string             `json:"policy_check_log,omitempty"`
	PolicyCheckScope  string             `json:"policy_check_scope,omitempty"`  // "organization" or "workspace"
	PolicyCheckStatus string             `json:"policy_check_status,omitempty"` // "hard_failed", "soft_failed", "errored"
	PolicyEvaluations []PolicyEvalResult `json:"policy_evaluations,omitempty"`
	TaskResults       []TaskResult       `json:"task_results,omitempty"`
}

// PolicyEvalResult holds the outcome of a policy evaluation (OPA/Sentinel via task stages).
type PolicyEvalResult struct {
	PolicyKind    string          `json:"policy_kind"` // "opa" or "sentinel"
	Status        string          `json:"status"`
	PolicySetName string          `json:"policy_set_name"`
	Outcomes      []PolicyOutcome `json:"outcomes,omitempty"`
	Error         string          `json:"error,omitempty"`
}

// PolicyOutcome represents a single policy's result within a policy set.
type PolicyOutcome struct {
	PolicyName       string   `json:"policy_name"`
	EnforcementLevel string   `json:"enforcement_level"` // "advisory", "mandatory", "hard-mandatory", "soft-mandatory"
	Status           string   `json:"status"`            // "passed", "failed"
	Description      string   `json:"description,omitempty"`
	Output           []string `json:"output,omitempty"` // denial reason strings
}

// TaskResult holds the outcome of a run task.
type TaskResult struct {
	TaskName         string `json:"task_name"`
	Status           string `json:"status"`
	Message          string `json:"message,omitempty"`
	URL              string `json:"url,omitempty"`
	EnforcementLevel string `json:"enforcement_level"` // "advisory" or "mandatory"
	Stage            string `json:"stage"`             // "pre_plan", "post_plan", "pre_apply", "post_apply"
}

// Diagnostic represents a Terraform diagnostic message.
type Diagnostic struct {
	Severity string             `json:"severity"`
	Summary  string             `json:"summary"`
	Detail   string             `json:"detail"`
	Address  string             `json:"address,omitempty"`
	Range    *DiagnosticRange   `json:"range,omitempty"`
	Snippet  *DiagnosticSnippet `json:"snippet,omitempty"`
}

// DiagnosticRange represents the source location of a diagnostic.
type DiagnosticRange struct {
	Filename string         `json:"filename"`
	Start    SourceLocation `json:"start"`
	End      SourceLocation `json:"end"`
}

// SourceLocation represents a position in a source file.
type SourceLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Byte   int `json:"byte"`
}

// DiagnosticSnippet contains the source code context for a diagnostic.
type DiagnosticSnippet struct {
	Context              *string `json:"context"`
	Code                 string  `json:"code"`
	StartLine            int     `json:"start_line"`
	HighlightStartOffset int     `json:"highlight_start_offset"`
	HighlightEndOffset   int     `json:"highlight_end_offset"`
}

type jsonLog struct {
	Level      string      `json:"@level"`
	Message    string      `json:"@message"`
	Type       string      `json:"type"`
	Diagnostic *Diagnostic `json:"diagnostic,omitempty"`
}

// NewRunSummary fetches a run and returns a summary of its status. If the run
// has errored, it fetches the relevant log and extracts diagnostics. Additionally,
// it probes policy checks and run task stages for failures. All failures are surfaced
// in the RunSummary result.
func NewRunSummary(ctx context.Context, c *Client, runID string) (*RunSummary, error) {
	run, err := c.TFE.API.Runs().ById(runID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching run %s: %w", runID, err)
	}

	status := run.GetData().GetAttributes().GetStatus()
	if status == nil {
		return nil, fmt.Errorf("run %s has no status", runID)
	}

	return buildRunSummary(ctx, c, runID, *status)
}

func buildRunSummary(ctx context.Context, c *Client, runID string, status models.Runs_attributes_status) (*RunSummary, error) {
	result := &RunSummary{
		RunID:  runID,
		Status: status.String(),
	}

	switch status {
	case models.PENDING_RUNS_ATTRIBUTES_STATUS,
		models.FETCHING_RUNS_ATTRIBUTES_STATUS,
		models.QUEUING_RUNS_ATTRIBUTES_STATUS,
		models.PLAN_QUEUED_RUNS_ATTRIBUTES_STATUS,
		models.PLANNING_RUNS_ATTRIBUTES_STATUS,
		models.PRE_PLAN_RUNNING_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Plan in progress"

	case models.PLANNED_AND_FINISHED_RUNS_ATTRIBUTES_STATUS,
		models.PLANNED_AND_SAVED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Plan complete, no apply needed"

	case models.APPLIED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run succeeded"

	case models.CANCELED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run was canceled"
	case models.DISCARDED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run was discarded"

	case models.POLICY_OVERRIDE_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run awaiting policy override"
		if err := populatePolicySummary(ctx, c, runID, result); err != nil {
			return nil, err
		}
	case models.POLICY_SOFT_FAILED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run has soft-failed policies"
		if err := populatePolicySummary(ctx, c, runID, result); err != nil {
			return nil, err
		}

	case models.ERRORED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run errored"
		if err := populateErroredSummary(ctx, c, runID, result); err != nil {
			return nil, err
		}

	default:
		result.Message = fmt.Sprintf("Run status: %s", status.String())
	}

	return result, nil
}

// populatePolicySummary fetches policy failure details for runs in
// policy_soft_failed or policy_override status. It tries legacy Sentinel
// policy checks first, then falls back to task stages (OPA evaluations).
func populatePolicySummary(ctx context.Context, c *Client, runID string, result *RunSummary) error {
	result.Phase = "policy_check"

	if err := populatePolicyCheckSummary(ctx, c, runID, result); err != nil {
		return err
	}
	if result.PolicyCheckLog != "" {
		return nil
	}

	return populateTaskStageSummary(ctx, c, runID, result)
}

func populateErroredSummary(ctx context.Context, c *Client, runID string, result *RunSummary) error {
	plan, err := c.TFE.API.Runs().ById(runID).Plan().Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching plan for run %s: %w", runID, err)
	}

	planStatus := plan.GetData().GetAttributes().GetStatus()
	if planStatus != nil && *planStatus == models.ERRORED_PLANS_ATTRIBUTES_STATUS {
		result.Phase = "plan"
		logURL := plan.GetData().GetAttributes().GetLogReadUrl()
		if logURL == nil {
			return fmt.Errorf("plan for run %s has no log URL", runID)
		}
		return populateLogDiagnostics(result, *logURL)
	}

	// Check legacy policy checks.
	if err := populatePolicyCheckSummary(ctx, c, runID, result); err != nil {
		return err
	}
	if result.PolicyCheckLog != "" {
		return nil
	}

	// Check task stages (OPA policy evaluations + run tasks).
	if err := populateTaskStageSummary(ctx, c, runID, result); err != nil {
		return err
	}
	if len(result.PolicyEvaluations) > 0 || len(result.TaskResults) > 0 {
		return nil
	}

	result.Phase = "apply"
	runData, err := c.TFE.API.Runs().ById(runID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching run %s: %w", runID, err)
	}

	applyRel := runData.GetData().GetRelationships().GetApply()
	if applyRel == nil || applyRel.GetData() == nil || applyRel.GetData().GetId() == nil {
		return fmt.Errorf("run %s has no apply relationship", runID)
	}
	applyID := *applyRel.GetData().GetId()

	apply, err := c.TFE.API.Applies().ById(applyID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching apply %s: %w", applyID, err)
	}

	logURL := apply.GetData().GetAttributes().GetLogReadUrl()
	if logURL == nil {
		return fmt.Errorf("apply %s has no log URL", applyID)
	}
	return populateLogDiagnostics(result, *logURL)
}

// populatePolicyCheckSummary handles the legacy Sentinel policy check path.
// If a policy check has hard_failed, soft_failed, or errored, it fetches the
// Sentinel log output via the policy check output endpoint (which returns a
// 302 redirect to a presigned URL).
func populatePolicyCheckSummary(ctx context.Context, c *Client, runID string, result *RunSummary) error {
	resp, err := c.TFE.API.Runs().ById(runID).PolicyChecks().Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching policy checks for run %s: %w", runID, err)
	}

	for _, pc := range resp.GetData() {
		status := pc.GetAttributes().GetStatus()
		if status == nil {
			continue
		}

		switch *status {
		case models.HARD_FAILED_POLICYCHECKS_ATTRIBUTES_STATUS,
			models.SOFT_FAILED_POLICYCHECKS_ATTRIBUTES_STATUS,
			models.ERRORED_POLICYCHECKS_ATTRIBUTES_STATUS:
		default:
			continue
		}

		// Found a failed policy check — fetch its output log.
		result.Phase = "policy_check"
		result.PolicyCheckStatus = status.String()

		if scope := pc.GetAttributes().GetScope(); scope != nil {
			result.PolicyCheckScope = *scope
		}

		pcID := pc.GetId()
		if pcID == nil {
			return fmt.Errorf("policy check has no ID")
		}

		content, err := c.TFE.API.PolicyChecks().ById(*pcID).Output().Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("fetching policy check output for %s: %w", *pcID, err)
		}

		result.PolicyCheckLog = string(content)
		return nil // Stop after the first failed policy check (they run sequentially).
	}

	return nil
}

// stageOrder defines the lifecycle order for task stages.
var stageOrder = map[string]int{
	"pre_plan":   0,
	"post_plan":  1,
	"pre_apply":  2,
	"post_apply": 3,
}

// populateTaskStageSummary handles OPA policy evaluations and run tasks via
// the task stages API. It finds the first failed/errored stage in lifecycle
// order and populates the result with policy evaluation outcomes and/or task
// result details.
func populateTaskStageSummary(ctx context.Context, c *Client, runID string, result *RunSummary) error {
	resp, err := c.TFE.API.Runs().ById(runID).TaskStages().Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching task stages for run %s: %w", runID, err)
	}

	stages := resp.GetData()
	if len(stages) == 0 {
		return nil
	}

	// Sort stages by lifecycle order.
	sort.Slice(stages, func(i, j int) bool {
		oi, oj := 99, 99
		if si := stages[i].GetAttributes().GetStage(); si != nil {
			if o, ok := stageOrder[*si]; ok {
				oi = o
			}
		}
		if sj := stages[j].GetAttributes().GetStage(); sj != nil {
			if o, ok := stageOrder[*sj]; ok {
				oj = o
			}
		}
		return oi < oj
	})

	for _, ts := range stages {
		status := ts.GetAttributes().GetStatus()
		if status == nil {
			continue
		}

		switch *status {
		case models.FAILED_TASKSTAGES_ATTRIBUTES_STATUS,
			models.ERRORED_TASKSTAGES_ATTRIBUTES_STATUS:
		default:
			continue
		}

		// Found a failed/errored task stage.
		stage := ts.GetAttributes().GetStage()
		if stage != nil {
			result.Phase = *stage
		}

		// Fetch policy evaluations.
		if peRel := ts.GetRelationships().GetPolicyEvaluations(); peRel != nil {
			for _, pe := range peRel.GetData() {
				peID := relationshipID(pe.GetAdditionalData())
				if peID == "" {
					continue
				}

				// Fetch the policy evaluation to get its kind.
				policyKind := ""
				peResp, err := c.TFE.API.PolicyEvaluations().ByPolicy_evaluation_id(peID).Get(ctx, nil)
				if err == nil && peResp.GetData() != nil && peResp.GetData().GetAttributes() != nil {
					if k := peResp.GetData().GetAttributes().GetPolicyKind(); k != nil {
						policyKind = k.String()
					}
				}

				evals, err := fetchPolicySetOutcomes(ctx, c, peID, policyKind)
				if err != nil {
					return err
				}
				result.PolicyEvaluations = append(result.PolicyEvaluations, evals...)
			}
		}

		// Fetch task results.
		if trRel := ts.GetRelationships().GetTaskResults(); trRel != nil {
			for _, tr := range trRel.GetData() {
				trID := relationshipID(tr.GetAdditionalData())
				if trID == "" {
					continue
				}
				taskResult, err := fetchTaskResult(ctx, c, trID)
				if err != nil {
					return err
				}
				if taskResult != nil {
					result.TaskResults = append(result.TaskResults, *taskResult)
				}
			}
		}

		// Stop after the first failed stage in lifecycle order.
		return nil
	}

	return nil
}

// fetchPolicySetOutcomes fetches policy set outcomes for a policy evaluation
// and converts them to PolicyEvalResult structs. policyKind is set on each
// result since the outcomes endpoint does not return it.
func fetchPolicySetOutcomes(ctx context.Context, c *Client, peID, policyKind string) ([]PolicyEvalResult, error) {
	resp, err := c.TFE.API.PolicyEvaluations().ByPolicy_evaluation_id(peID).PolicySetOutcomes().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching policy set outcomes for %s: %w", peID, err)
	}

	var results []PolicyEvalResult
	for _, pso := range resp.GetData() {
		attrs := pso.GetAttributes()
		if attrs == nil {
			continue
		}

		eval := PolicyEvalResult{PolicyKind: policyKind}
		if name := attrs.GetPolicySetName(); name != nil {
			eval.PolicySetName = *name
		}
		if e := attrs.GetError(); e != nil {
			eval.Error = *e
		}

		for _, outcome := range attrs.GetOutcomes() {
			po := PolicyOutcome{}
			if name := outcome.GetPolicyName(); name != nil {
				po.PolicyName = *name
			}
			if level := outcome.GetEnforcementLevel(); level != nil {
				po.EnforcementLevel = *level
			}
			if s := outcome.GetStatus(); s != nil {
				po.Status = *s
			}
			if desc := outcome.GetDescription(); desc != nil {
				po.Description = *desc
			}
			po.Output = extractStringSlice(outcome.GetOutput())
			eval.Outcomes = append(eval.Outcomes, po)
		}

		results = append(results, eval)
	}

	return results, nil
}

// relationshipID extracts the "id" string from a Kiota relationship object's
// additional data. The generated types store flat JSON:API {id, type}
// relationship fields in AdditionalData rather than typed accessors.
func relationshipID(ad map[string]any) string {
	if id, ok := ad["id"].(*string); ok && id != nil {
		return *id
	}
	return ""
}

// extractStringSlice extracts a []string from a Kiota UntypedNodeable that
// represents a JSON array of strings.
func extractStringSlice(node serialization.UntypedNodeable) []string {
	if node == nil {
		return nil
	}
	arr, ok := node.(*serialization.UntypedArray)
	if !ok {
		return nil
	}
	var result []string
	for _, elem := range arr.GetValue() {
		str, ok := elem.(*serialization.UntypedString)
		if !ok {
			continue
		}
		if v := str.GetValue(); v != nil {
			result = append(result, *v)
		}
	}
	return result
}

// fetchTaskResult fetches a single task result by ID and converts it to a
// TaskResult struct.
func fetchTaskResult(ctx context.Context, c *Client, trID string) (*TaskResult, error) {
	resp, err := c.TFE.API.TaskResults().ByTask_result_id(trID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching task result %s: %w", trID, err)
	}

	data := resp.GetData()
	if data == nil {
		return nil, nil
	}
	attrs := data.GetAttributes()
	if attrs == nil {
		return nil, nil
	}

	tr := &TaskResult{}
	if name := attrs.GetTaskName(); name != nil {
		tr.TaskName = *name
	}
	if s := attrs.GetStatus(); s != nil {
		tr.Status = s.String()
	}
	if msg := attrs.GetMessage(); msg != nil {
		tr.Message = *msg
	}
	if u := attrs.GetUrl(); u != nil {
		tr.URL = *u
	}
	if level := attrs.GetWorkspaceTaskEnforcementLevel(); level != nil {
		tr.EnforcementLevel = *level
	}
	if stage := attrs.GetStage(); stage != nil {
		tr.Stage = *stage
	}

	return tr, nil
}

func populateLogDiagnostics(result *RunSummary, logURL string) error {
	logContent, err := FetchLog(logURL)
	if err != nil {
		return err
	}

	diags := ParseDiagnostics(logContent)
	if len(diags) > 0 {
		result.Diagnostics = diags
	} else {
		result.RawLog = logContent
	}
	return nil
}

// FetchLog fetches the raw log content from an archivist log-read-url.
// The URL is self-authenticating so no additional auth headers are needed.
func FetchLog(logURL string) (string, error) {
	resp, err := http.Get(logURL) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("fetching log: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching log: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading log body: %w", err)
	}

	return string(body), nil
}

// ParseDiagnostics attempts to extract diagnostics from log output.
// It detects structured logs (JSON lines after the first 3 lines) and parses them.
func ParseDiagnostics(logContent string) []Diagnostic {
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	if len(lines) <= 3 {
		return nil
	}

	structured := false
	for _, line := range lines[3:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if json.Valid([]byte(line)) {
			structured = true
			break
		}
		break
	}

	if !structured {
		return nil
	}

	var diags []Diagnostic
	for _, line := range lines[3:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry jsonLog
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.Type == "diagnostic" && entry.Diagnostic != nil {
			diags = append(diags, *entry.Diagnostic)
		}
	}

	return diags
}
