// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"fmt"
	"strings"

	wordwrap "github.com/mitchellh/go-wordwrap"

	"github.com/hashicorp/go-tfe/api/models"
	"github.com/hashicorp/tfcloud/internal/pkg/client"
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	terraformcfg "github.com/hashicorp/tfcloud/internal/pkg/terraform"
)

type StatusOpts struct {
	IO           iostreams.IOStreams
	ShutdownCtx  context.Context
	Client       *client.Client
	Output       *format.Outputter
	Organization string
	RunID        string
}

type statusResult struct {
	RunID       string       `json:"run_id"`
	Status      string       `json:"status"`
	Message     string       `json:"message"`
	Phase       string       `json:"phase,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	RawLog      string       `json:"raw_log,omitempty"`
}

// NewCmdRunStatus creates the `tfcloud run status` command.
func NewCmdRunStatus(ctx *cmd.Context) *cmd.Command {
	opts := &StatusOpts{
		IO:          ctx.IO,
		ShutdownCtx: ctx.ShutdownCtx,
	}

	var organization string

	cmd := &cmd.Command{
		Name:      "status",
		ShortHelp: "Show the status of a run, printing diagnostics if it failed.",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud run status" }} command inspects a Terraform Cloud run
		and prints its current status. If the run has errored, it fetches the plan or apply log and
		extracts diagnostic messages.

		The ID argument can be:
		- A run ID ({{ template "mdCodeOrBold" "run-..." }})
		- A workspace ID ({{ template "mdCodeOrBold" "ws-..." }}) to get the latest run
		- A workspace name to get the latest run (requires {{ template "mdCodeOrBold" "--organization" }})
		`),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "ID",
					Documentation: "Run ID, workspace ID, or workspace name",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:        "organization",
					Description: "Organization name (defaults to profile or terraform cloud config context)",
					Value:       flagvalue.Simple("", &organization),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Check status of a run by ID",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud run status run-abc123`),
			},
			{
				Preamble: "Check the latest run in a workspace by name",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud run status my-workspace --organization my-org`),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			if len(args) != 1 {
				return cmd.ErrDisplayUsage
			}

			opts.Organization = organization
			if opts.Organization == "" {
				opts.Organization = ctx.Profile.Organization
			}
			if opts.Organization == "" {
				cfg, err := terraformcfg.FindCloudConfig(".")
				if err == nil && cfg.Organization != "" {
					opts.Organization = cfg.Organization
				}
			}

			apiClient, err := ctx.NewAPIClient()
			if err != nil {
				return fmt.Errorf("unable to create API client: %w", err)
			}
			opts.Client = apiClient
			opts.Output = ctx.Output

			runID, err := resolveRunID(opts, args[0])
			if err != nil {
				return err
			}
			opts.RunID = runID

			return runStatus(opts)
		},
	}

	return cmd
}

func runStatus(opts *StatusOpts) error {
	ctx := opts.ShutdownCtx

	run, err := opts.Client.TFE.API.Runs().ById(opts.RunID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching run %s: %w", opts.RunID, err)
	}

	status := run.GetData().GetAttributes().GetStatus()
	if status == nil {
		return fmt.Errorf("run %s has no status", opts.RunID)
	}

	result, err := buildStatusResult(opts, *status)
	if err != nil {
		return err
	}

	if err := opts.Output.Display(&statusDisplayer{result: result, io: opts.IO}); err != nil {
		return err
	}

	if result.Status == "errored" {
		return cmd.ErrUnderlyingError
	}
	return nil
}

func buildStatusResult(opts *StatusOpts, status models.Runs_attributes_status) (*statusResult, error) {
	result := &statusResult{
		RunID:  opts.RunID,
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
		if err := populateErroredResult(opts, result); err != nil {
			return nil, err
		}

	default:
		result.Message = fmt.Sprintf("Run status: %s", status.String())
	}

	return result, nil
}

func populateErroredResult(opts *StatusOpts, result *statusResult) error {
	ctx := opts.ShutdownCtx

	plan, err := opts.Client.TFE.API.Runs().ById(opts.RunID).Plan().Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching plan for run %s: %w", opts.RunID, err)
	}

	planStatus := plan.GetData().GetAttributes().GetStatus()
	if planStatus != nil && *planStatus == models.ERRORED_PLANS_ATTRIBUTES_STATUS {
		result.Phase = "plan"
		logURL := plan.GetData().GetAttributes().GetLogReadUrl()
		if logURL == nil {
			return fmt.Errorf("plan for run %s has no log URL", opts.RunID)
		}
		return populateLogDiagnostics(result, *logURL)
	}

	result.Phase = "apply"
	runData, err := opts.Client.TFE.API.Runs().ById(opts.RunID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching run %s: %w", opts.RunID, err)
	}

	applyRel := runData.GetData().GetRelationships().GetApply()
	if applyRel == nil || applyRel.GetData() == nil || applyRel.GetData().GetId() == nil {
		return fmt.Errorf("run %s has no apply relationship", opts.RunID)
	}
	applyID := *applyRel.GetData().GetId()

	apply, err := opts.Client.TFE.API.Applies().ById(applyID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching apply %s: %w", applyID, err)
	}

	logURL := apply.GetData().GetAttributes().GetLogReadUrl()
	if logURL == nil {
		return fmt.Errorf("apply %s has no log URL", applyID)
	}
	return populateLogDiagnostics(result, *logURL)
}

func populateLogDiagnostics(result *statusResult, logURL string) error {
	logContent, err := fetchLog(logURL)
	if err != nil {
		return err
	}

	diags := parseDiagnostics(logContent)
	if len(diags) > 0 {
		result.Diagnostics = diags
	} else {
		result.RawLog = logContent
	}
	return nil
}

// statusDisplayer implements format.Displayer and format.StringPayload.
type statusDisplayer struct {
	result *statusResult
	io     iostreams.IOStreams
}

var _ format.Displayer = (*statusDisplayer)(nil)
var _ format.StringPayload = (*statusDisplayer)(nil)

func (d *statusDisplayer) DefaultFormat() format.Format { return format.Pretty }
func (d *statusDisplayer) Payload() any                 { return d.result }
func (d *statusDisplayer) FieldTemplates() []format.Field {
	return nil
}

// StringPayload returns pre-formatted output tailored to the given format.
func (d *statusDisplayer) StringPayload(f format.Format) string {
	if len(d.result.Diagnostics) > 0 {
		return d.formatDiagnostics(f)
	}
	if d.result.RawLog != "" {
		return d.result.RawLog
	}
	return d.result.Message
}

func (d *statusDisplayer) formatDiagnostics(f format.Format) string {
	switch f {
	case format.Markdown:
		return d.formatDiagnosticsMarkdown()
	default:
		return d.formatDiagnosticsPretty()
	}
}

// formatDiagnosticsPretty renders diagnostics in terraform-style box-drawing
// format with ANSI colors. Uses the same two-pass approach as Terraform:
// build the body first, then prepend the colored left-rule to each line.
func (d *statusDisplayer) formatDiagnosticsPretty() string {
	cs := d.io.ColorScheme()
	const leftRuleWidth = 2
	wrapWidth := d.io.TerminalWidth() - leftRuleWidth

	var out strings.Builder
	for i, diag := range d.result.Diagnostics {
		if i > 0 {
			out.WriteString("\n")
		}

		color := cs.Red()
		label := "Error"
		if diag.Severity == "warning" {
			color = cs.Orange()
			label = "Warning"
		}

		// Pass 1: build body without left rule.
		var body strings.Builder
		body.WriteString(cs.String(fmt.Sprintf("%s: ", label)).Color(color).Bold().String())
		body.WriteString(cs.String(diag.Summary).Bold().String())
		body.WriteString("\n")
		if diag.Detail != "" {
			body.WriteString("\n")
			for _, line := range strings.Split(diag.Detail, "\n") {
				if wrapWidth > 0 && line != "" && line[0] != ' ' {
					line = wordwrap.WrapString(line, uint(wrapWidth))
				}
				body.WriteString(line)
				body.WriteString("\n")
			}
		}

		// Pass 2: prepend colored left rule to each line.
		rule := cs.String("│").Color(color).String()
		out.WriteString(cs.String("╷").Color(color).String())
		out.WriteString("\n")
		for _, line := range strings.Split(strings.TrimRight(body.String(), "\n"), "\n") {
			out.WriteString(rule)
			if line != "" {
				out.WriteString(" ")
				out.WriteString(line)
			}
			out.WriteString("\n")
		}
		out.WriteString(cs.String("╵").Color(color).String())
	}
	return out.String()
}

// formatDiagnosticsMarkdown renders diagnostics as markdown.
func (d *statusDisplayer) formatDiagnosticsMarkdown() string {
	var out strings.Builder
	for _, diag := range d.result.Diagnostics {
		label := "Error"
		if diag.Severity == "warning" {
			label = "Warning"
		}
		fmt.Fprintf(&out, "**%s:** %s\n", label, diag.Summary)
		if diag.Detail != "" {
			fmt.Fprintf(&out, "\n%s\n", diag.Detail)
		}
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}
