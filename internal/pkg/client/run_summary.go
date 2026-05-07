// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/go-tfe/api"
	"github.com/hashicorp/go-tfe/api/models"
	"github.com/hashicorp/go-tfe/api/workspaces"
	abstractions "github.com/microsoft/kiota-abstractions-go"
)

// RunSummary is the result of inspecting a Terraform Cloud run.
type RunSummary struct {
	RunID       string       `json:"run_id"`
	Status      string       `json:"status"`
	Message     string       `json:"message"`
	Phase       string       `json:"phase,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	RawLog      string       `json:"raw_log,omitempty"`
}

// Diagnostic represents a Terraform diagnostic message.
type Diagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
	Address  string `json:"address,omitempty"`
}

type jsonLog struct {
	Level      string      `json:"@level"`
	Message    string      `json:"@message"`
	Type       string      `json:"type"`
	Diagnostic *Diagnostic `json:"diagnostic,omitempty"`
}

// ResolveRunID resolves an identifier to a run ID. The identifier can be a
// run ID (run-*), workspace ID (ws-*), or workspace name (requires org).
func ResolveRunID(ctx context.Context, tfeAPI *api.ApiClient, org, id string) (string, error) {
	switch {
	case strings.HasPrefix(id, "run-"):
		return id, nil

	case strings.HasPrefix(id, "ws-"):
		return getLatestRunForWorkspace(ctx, tfeAPI, id)

	default:
		if org == "" {
			return "", fmt.Errorf("--organization is required when specifying a workspace name")
		}
		ws, err := tfeAPI.Organizations().ByOrganization_name(org).Workspaces().ByWorkspace_name(id).Get(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("resolving workspace %q: %w", id, err)
		}
		wsID := ws.GetData().GetId()
		if wsID == nil {
			return "", fmt.Errorf("workspace %q has no ID", id)
		}
		return getLatestRunForWorkspace(ctx, tfeAPI, *wsID)
	}
}

func getLatestRunForWorkspace(ctx context.Context, tfeAPI *api.ApiClient, wsID string) (string, error) {
	pageSize := int32(1)
	config := &abstractions.RequestConfiguration[workspaces.ItemRunsRequestBuilderGetQueryParameters]{
		QueryParameters: &workspaces.ItemRunsRequestBuilderGetQueryParameters{
			Pagesize: &pageSize,
		},
	}

	runs, err := tfeAPI.Workspaces().ByWorkspace_id(wsID).Runs().Get(ctx, config)
	if err != nil {
		return "", fmt.Errorf("fetching runs for workspace %s: %w", wsID, err)
	}

	data := runs.GetData()
	if len(data) == 0 {
		return "", fmt.Errorf("no runs found for workspace %s", wsID)
	}

	runID := data[0].GetId()
	if runID == nil {
		return "", fmt.Errorf("latest run has no ID")
	}
	return *runID, nil
}

// GetRunSummary fetches a run and returns a summary of its status. If the run
// has errored, it fetches the relevant log and extracts diagnostics.
func GetRunSummary(ctx context.Context, tfeAPI *api.ApiClient, runID string) (*RunSummary, error) {
	run, err := tfeAPI.Runs().ById(runID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching run %s: %w", runID, err)
	}

	status := run.GetData().GetAttributes().GetStatus()
	if status == nil {
		return nil, fmt.Errorf("run %s has no status", runID)
	}

	return buildRunSummary(ctx, tfeAPI, runID, *status)
}

func buildRunSummary(ctx context.Context, tfeAPI *api.ApiClient, runID string, status models.Runs_attributes_status) (*RunSummary, error) {
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
	case models.POLICY_SOFT_FAILED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run has soft-failed policies"

	case models.ERRORED_RUNS_ATTRIBUTES_STATUS:
		result.Message = "Run errored"
		if err := populateErroredSummary(ctx, tfeAPI, runID, result); err != nil {
			return nil, err
		}

	default:
		result.Message = fmt.Sprintf("Run status: %s", status.String())
	}

	return result, nil
}

func populateErroredSummary(ctx context.Context, tfeAPI *api.ApiClient, runID string, result *RunSummary) error {
	plan, err := tfeAPI.Runs().ById(runID).Plan().Get(ctx, nil)
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

	result.Phase = "apply"
	runData, err := tfeAPI.Runs().ById(runID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching run %s: %w", runID, err)
	}

	applyRel := runData.GetData().GetRelationships().GetApply()
	if applyRel == nil || applyRel.GetData() == nil || applyRel.GetData().GetId() == nil {
		return fmt.Errorf("run %s has no apply relationship", runID)
	}
	applyID := *applyRel.GetData().GetId()

	apply, err := tfeAPI.Applies().ById(applyID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching apply %s: %w", applyID, err)
	}

	logURL := apply.GetData().GetAttributes().GetLogReadUrl()
	if logURL == nil {
		return fmt.Errorf("apply %s has no log URL", applyID)
	}
	return populateLogDiagnostics(result, *logURL)
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
