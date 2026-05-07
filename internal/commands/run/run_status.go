// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-tfe/api/models"
	"github.com/hashicorp/tfcloud/internal/pkg/client"
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	terraformcfg "github.com/hashicorp/tfcloud/internal/pkg/terraform"
)

// StatusOpts stores the options for the run status command.
type StatusOpts struct {
	IO           iostreams.IOStreams
	ShutdownCtx  context.Context
	Client       *client.Client
	Organization string
	RunID        string // resolved run ID
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

	// Fetch the run.
	run, err := opts.Client.TFE.API.Runs().ById(opts.RunID).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching run %s: %w", opts.RunID, err)
	}

	status := run.GetData().GetAttributes().GetStatus()
	if status == nil {
		return fmt.Errorf("run %s has no status", opts.RunID)
	}

	return handleRunStatus(opts, *status)
}

func handleRunStatus(opts *StatusOpts, status models.Runs_attributes_status) error {
	switch status {
	// Plan in progress
	case models.PENDING_RUNS_ATTRIBUTES_STATUS,
		models.FETCHING_RUNS_ATTRIBUTES_STATUS,
		models.QUEUING_RUNS_ATTRIBUTES_STATUS,
		models.PLAN_QUEUED_RUNS_ATTRIBUTES_STATUS,
		models.PLANNING_RUNS_ATTRIBUTES_STATUS,
		models.PRE_PLAN_RUNNING_RUNS_ATTRIBUTES_STATUS:
		fmt.Fprintln(opts.IO.Out(), "Plan in progress")
		return nil

	// Plan complete, no apply needed
	case models.PLANNED_AND_FINISHED_RUNS_ATTRIBUTES_STATUS,
		models.PLANNED_AND_SAVED_RUNS_ATTRIBUTES_STATUS:
		fmt.Fprintln(opts.IO.Out(), "Plan complete, no apply needed")
		return nil

	// Run succeeded
	case models.APPLIED_RUNS_ATTRIBUTES_STATUS:
		fmt.Fprintln(opts.IO.Out(), "Run succeeded")
		return nil

	// Run was canceled/discarded
	case models.CANCELED_RUNS_ATTRIBUTES_STATUS:
		fmt.Fprintln(opts.IO.Out(), "Run was canceled")
		return nil
	case models.DISCARDED_RUNS_ATTRIBUTES_STATUS:
		fmt.Fprintln(opts.IO.Out(), "Run was discarded")
		return nil

	// Policy status
	case models.POLICY_OVERRIDE_RUNS_ATTRIBUTES_STATUS:
		fmt.Fprintln(opts.IO.Out(), "Run awaiting policy override")
		return nil
	case models.POLICY_SOFT_FAILED_RUNS_ATTRIBUTES_STATUS:
		fmt.Fprintln(opts.IO.Out(), "Run has soft-failed policies")
		return nil

	// Errored — fetch logs and extract diagnostics
	case models.ERRORED_RUNS_ATTRIBUTES_STATUS:
		return handleErroredRun(opts)

	// All other in-progress states
	default:
		fmt.Fprintf(opts.IO.Out(), "Run status: %s\n", status.String())
		return nil
	}
}

// handleErroredRun determines which phase failed and fetches diagnostics.
func handleErroredRun(opts *StatusOpts) error {
	ctx := opts.ShutdownCtx

	// Fetch the plan to determine if it errored.
	plan, err := opts.Client.TFE.API.Runs().ById(opts.RunID).Plan().Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching plan for run %s: %w", opts.RunID, err)
	}

	planStatus := plan.GetData().GetAttributes().GetStatus()
	if planStatus != nil && *planStatus == models.ERRORED_PLANS_ATTRIBUTES_STATUS {
		// Plan failed — fetch plan log.
		logURL := plan.GetData().GetAttributes().GetLogReadUrl()
		if logURL == nil {
			return fmt.Errorf("plan for run %s has no log URL", opts.RunID)
		}
		return fetchAndPrintDiagnostics(opts, *logURL, "plan")
	}

	// Apply failed — get the apply and fetch its log.
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
	return fetchAndPrintDiagnostics(opts, *logURL, "apply")
}

// fetchAndPrintDiagnostics fetches a log from the given URL and prints diagnostics.
func fetchAndPrintDiagnostics(opts *StatusOpts, logURL string, phase string) error {
	logContent, err := fetchLog(logURL)
	if err != nil {
		return err
	}

	diags := parseDiagnostics(logContent)
	if len(diags) > 0 {
		for _, d := range diags {
			fmt.Fprintf(opts.IO.Out(), "%s: %s\n", d.Severity, d.Summary)
			if d.Detail != "" {
				fmt.Fprintf(opts.IO.Out(), "  %s\n", d.Detail)
			}
		}
	} else {
		// Not structured or no diagnostics found — print raw log.
		fmt.Fprintln(opts.IO.Out(), logContent)
	}

	return cmd.NewExitError(1, fmt.Errorf("run %s errored during %s", opts.RunID, phase))
}
