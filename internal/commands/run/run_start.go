// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe/v2/api/models"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
	"github.com/hashicorp/tfctl-cli/version"
)

// StartOpts defines the options for the `run start` command.
type StartOpts struct {
	IO           iostreams.IOStreams
	Profile      *profile.Profile
	Output       *format.Outputter
	APIClient    *client.Client
	Workspace    string
	DryRun       bool
	Organization string
	// Wait blocks until the run reaches a terminal state, then prints its
	// status and exits non-zero if the run failed.
	Wait bool
	// Timeout bounds how long Wait polls (0 means wait indefinitely).
	Timeout time.Duration
	// PollInterval overrides the wait poll cadence (0 uses defaultPollInterval).
	// Primarily a test seam.
	PollInterval time.Duration
}

// CreateOpts defines the options for running a run start, which may be shared with other commands.
type CreateOpts struct {
	DebuggingMode   bool
	Message         string
	AllowEmptyApply bool
	PlanOnly        bool
}

// NewCmdRunStart creates the `run start` command.
func NewCmdRunStart(inv *cmd.Invocation) *cmd.Command {
	startOpts := StartOpts{
		IO:      inv.IO,
		Profile: inv.Profile,
		Output:  inv.Output,
	}

	runOpts := CreateOpts{}

	cmd := &cmd.Command{
		Name:      "start",
		ShortHelp: "Start a new run on a workspace.",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s run start" }} command creates a new plan and apply run with the most recent configuration. {{ Bold "If auto-apply is enabled and no errors occur, the plan will be automatically applied." }}

		Use {{ template "mdCodeOrBold" "--plan-only" }} to create a speculative plan-only run that is never applied, regardless of the workspace's auto-apply setting.

		The ID argument can be:
		- A workspace ID ({{ template "mdCodeOrBold" "ws-abc123" }})
		- A workspace name (may require {{ template "mdCodeOrBold" "--organization" }})
		`, version.Name),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "WORKSPACE",
					Documentation: "workspace ID, or workspace name",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:        "organization",
					Description: "Organization name (defaults to profile or terraform cloud config context)",
					Value:       flagvalue.Simple("", &startOpts.Organization),
				},
				{
					Name:          "debugging-mode",
					Description:   "Enables trace logging for this run by setting TF_LOG=trace in the terraform environment for this run.",
					Value:         flagvalue.Simple(false, &runOpts.DebuggingMode),
					IsBooleanFlag: true,
				},
				{
					Name:        "message",
					Description: "A message to attach to the run",
					Value:       flagvalue.Simple("", &runOpts.Message),
				},
				{
					Name:          "allow-empty-apply",
					Description:   "Allow the run to proceed even if the plan has no changes. Useful for applying a side effect such as a terraform upgrade when no other changes are present.",
					Value:         flagvalue.Simple(false, &runOpts.AllowEmptyApply),
					IsBooleanFlag: true,
				},
				{
					Name:          "plan-only",
					Description:   "Create a speculative plan-only run that is never applied, regardless of the workspace's auto-apply setting.",
					Value:         flagvalue.Simple(false, &runOpts.PlanOnly),
					IsBooleanFlag: true,
				},
				{
					Name:          "wait",
					Description:   "Wait for the run to reach a terminal state, printing status as it progresses. Exits non-zero if the run fails.",
					Value:         flagvalue.Simple(false, &startOpts.Wait),
					IsBooleanFlag: true,
				},
				{
					Name:        "timeout",
					Description: "With --wait, the maximum time to wait for the run to finish (e.g. 30m). Defaults to waiting indefinitely.",
					Value:       flagvalue.Duration(0, &startOpts.Timeout),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Start a new run in a workspace by ID",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run start ws-abc123`, version.Name),
			},
			{
				Preamble: "Start a new run in a workspace by name",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run start my-workspace --organization my-org`, version.Name),
			},
			{
				Preamble: "Start a plan-only run that will not be applied",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run start ws-abc123 --plan-only`, version.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			if len(args) != 1 {
				return cmd.ErrDisplayUsage
			}

			if startOpts.Organization == "" {
				startOpts.Organization = inv.Profile.DefaultOrganization
			}
			if startOpts.Organization == "" {
				cfg, err := terraformcfg.FindCloudConfig(".")
				if err == nil && cfg.Organization != "" {
					startOpts.Organization = cfg.Organization
				}
			}

			startOpts.Workspace = args[0]
			startOpts.DryRun = inv.IsDryRun()

			apiClient, err := inv.NewAPIClient()
			if err != nil {
				return fmt.Errorf("unable to create API client: %w", err)
			}

			startOpts.APIClient = apiClient

			return runStart(inv.ShutdownCtx, startOpts, runOpts)
		},
	}

	return cmd
}

func runStart(ctx context.Context, opts StartOpts, runOpts CreateOpts) error {
	io := opts.IO
	cs := io.ColorScheme()

	resolver := client.NewResolver(opts.APIClient, false, opts.DryRun)
	id := opts.Workspace

	wsID := &id
	var ws *models.Workspaces
	var err error
	if !strings.HasPrefix(id, "ws-") {
		ws, err = resolver.Workspace(ctx, opts.Organization, opts.Workspace)
		if err != nil {
			return err
		}
		wsID = ws.GetId()
	} else {
		response, err := opts.APIClient.TFE.API.Workspaces().ByWorkspace_id(id).Get(ctx, nil)
		if err != nil {
			return err
		}
		ws = response.GetData().(*models.Workspaces)
	}

	organizationName := ws.GetRelationships().GetOrganization().GetData().GetId()

	if opts.DryRun {
		fmt.Fprintf(opts.IO.Err(), "%s would create run for workspace ID %s\n",
			cs.DryRunLabel(), *wsID)
		return nil
	}

	response, err := opts.APIClient.TFE.API.Runs().Post(ctx, buildRunsEnvelope(*wsID, runOpts), nil)
	if err != nil {
		return fmt.Errorf("failed to start run: %w", err)
	}

	newRunID := *response.GetData().GetId()

	runURL := fmt.Sprintf("https://%s/app/%s/workspaces/%s/runs/%s",
		opts.Profile.GetHostname(), *organizationName, *ws.GetAttributes().GetName(), newRunID)

	if !opts.Wait {
		fmt.Fprintln(io.Err(), heredoc.New(io).Mustf(`
%s %s created. You can monitor the status of the run using:

{{ Bold "$ %s run status %s" }}

or by visiting {{ Bold "%s" }}
`, cs.SuccessIcon(), newRunID, version.Name, newRunID, runURL))
		fmt.Fprintln(io.Err())
		return nil
	}

	return waitForRunAndReport(ctx, opts, newRunID, runURL)
}

// waitForRunAndReport polls a freshly created run to a terminal state, renders
// its status summary (reusing the same displayer as `run status`), and maps the
// outcome to an exit code: failed runs return cmd.ErrUnderlyingError.
func waitForRunAndReport(ctx context.Context, opts StartOpts, runID, runURL string) error {
	io := opts.IO
	cs := io.ColorScheme()

	start := time.Now()
	fmt.Fprintf(io.Err(), "%s %s created; waiting for it to finish...\n", cs.SuccessIcon(), runID)

	_, outcome, err := pollRunUntilSettled(ctx, opts.APIClient, runID, io, opts.PollInterval, opts.Timeout)
	if err != nil {
		// The wait was interrupted (timeout or cancel), but the run itself keeps
		// running in HCP Terraform. Point the user at it before returning.
		fmt.Fprintf(io.Err(), "%s Stopped waiting; the run may still be running in HCP Terraform:\n  %s\n",
			cs.FailureIcon(), runURL)
		return err
	}

	summary, err := client.NewRunSummary(ctx, opts.APIClient, runID)
	if err != nil {
		return err
	}
	// For an awaiting-confirmation run the raw summary message is the generic
	// "Run status: planned"; replace it with something actionable so the final
	// line reads as cleanly as the succeeded/failed cases.
	if outcome == runAwaitingConfirm {
		summary.Message = "Plan finished; a manual apply is required (auto-apply is off)."
	}
	if err := opts.Output.Display(&summaryDisplayer{summary: summary, io: io}); err != nil {
		return err
	}

	// Report elapsed wait time and always surface the run URL so a waited run
	// stays click-through-able.
	elapsed := time.Since(start).Round(time.Second)
	switch outcome {
	case runFailed:
		fmt.Fprintf(io.Err(), "%s Failed after %s. View the run at %s\n", cs.FailureIcon(), elapsed, runURL)
		return cmd.ErrUnderlyingError
	case runAwaitingConfirm:
		fmt.Fprintf(io.Err(), "%s Planned in %s. Confirm the apply at %s\n", cs.SuccessIcon(), elapsed, runURL)
	default: // runSucceeded
		fmt.Fprintf(io.Err(), "%s Completed in %s. View the run at %s\n", cs.SuccessIcon(), elapsed, runURL)
	}
	return nil
}

func buildRunsEnvelope(wsID string, ro CreateOpts) *models.RunsEnvelope {
	wsType := models.WORKSPACES_WORKSPACESID_DATA_TYPE

	workspaceIDData := models.NewWorkspacesId_data()
	workspaceIDData.SetId(&wsID)
	workspaceIDData.SetTypeEscaped(&wsType)

	workspaceID := models.NewWorkspacesId()
	workspaceID.SetData(workspaceIDData)

	relationships := models.NewRuns_relationships()
	relationships.SetWorkspace(workspaceID)

	attributes := models.NewRuns_attributes()
	attributes.SetMessage(&ro.Message)
	attributes.SetAllowEmptyApply(&ro.AllowEmptyApply)
	attributes.SetPlanOnly(&ro.PlanOnly)

	if ro.DebuggingMode {
		// This attribute is missing from the API spec! If you are reading this, it's been added by now
		// so try this instead
		// attributes.SetDebuggingMode(&debuggingMode)
		attributes.SetAdditionalData(map[string]any{
			"debugging-mode": true,
		})
	}

	runData := &models.Runs{}
	runData.SetRelationships(relationships)
	runData.SetAttributes(attributes)

	result := models.NewRunsEnvelope()
	result.SetData(runData)

	return result
}
