// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe/api/models"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
)

// StartOpts defines the options for the `run retry` command.
type StartOpts struct {
	IO           iostreams.IOStreams
	Profile      *profile.Profile
	Output       *format.Outputter
	APIClient    *client.Client
	Workspace    string
	DryRun       bool
	Organization string
}

// NewCmdRunStart creates the `run start` command.
func NewCmdRunStart(ctx *cmd.Context) *cmd.Command {
	startOpts := StartOpts{
		IO:      ctx.IO,
		Profile: ctx.Profile,
		Output:  ctx.Output,
	}

	cmd := &cmd.Command{
		Name:      "start",
		ShortHelp: "Start a new run on a workspace.",
		LongHelp: heredoc.New(ctx.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s run start" }} command creates a new plan and apply run with the most recent configuration. {{ Bold "If auto-apply is enabled and no errors occur, the plan will be automatically applied." }}

		The ID argument can be:
		- A workspace ID ({{ template "mdCodeOrBold" "ws-abc123" }})
		- A workspace name (may require {{ template "mdCodeOrBold" "--organization" }})
		`, config.Name),
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
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Start a new run in a workspace by ID",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run start ws-abc123`, config.Name),
			},
			{
				Preamble: "Start a new run in a workspace by name",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run start my-workspace --organization my-org`, config.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			if len(args) != 1 {
				return cmd.ErrDisplayUsage
			}

			if startOpts.Organization == "" {
				startOpts.Organization = ctx.Profile.Organization
			}
			if startOpts.Organization == "" {
				cfg, err := terraformcfg.FindCloudConfig(".")
				if err == nil && cfg.Organization != "" {
					startOpts.Organization = cfg.Organization
				}
			}

			startOpts.Workspace = args[0]
			startOpts.DryRun = ctx.IsDryRun()

			apiClient, err := ctx.NewAPIClient()
			if err != nil {
				return fmt.Errorf("unable to create API client: %w", err)
			}

			startOpts.APIClient = apiClient

			return runStart(ctx.ShutdownCtx, startOpts)
		},
	}

	return cmd
}

func runStart(ctx context.Context, opts StartOpts) error {
	io := opts.IO
	cs := io.ColorScheme()

	resolver := client.NewResolver(opts.APIClient, false, false)
	id := opts.Workspace

	wsID := &id
	var err error
	if !strings.HasPrefix(id, "ws-") {
		wsID, err = resolver.Workspace(ctx, opts.Organization, opts.Workspace)
		if err != nil {
			return err
		}
	}

	if opts.DryRun {
		fmt.Fprintf(opts.IO.Err(), "%s would create run for workspace ID %s\n",
			cs.DryRunLabel(), *wsID)
		return nil
	}

	response, err := opts.APIClient.TFE.API.Runs().Post(ctx, buildRunsEnvelope(wsID), nil)
	if err != nil {
		return fmt.Errorf("failed to start run: %w", err)
	}

	newRunID := *response.GetData().GetId()

	fmt.Fprintln(io.Err(), heredoc.New(io).Mustf(`
%s %s created. You can monitor the status of the run using:

{{ Bold "$ %s run status %s" }}
`, cs.SuccessIcon(), newRunID, config.Name, newRunID))
	fmt.Fprintln(io.Err())
	return nil
}

func buildRunsEnvelope(wsID *string) *models.RunsEnvelope {
	wsType := models.WORKSPACES_WORKSPACESID_DATA_TYPE

	workspaceIDData := models.NewWorkspacesId_data()
	workspaceIDData.SetId(wsID)
	workspaceIDData.SetTypeEscaped(&wsType)

	workspaceID := models.NewWorkspacesId()
	workspaceID.SetData(workspaceIDData)

	relationships := models.NewRuns_relationships()
	relationships.SetWorkspace(workspaceID)

	runData := &models.Runs{}
	runData.SetRelationships(relationships)

	result := models.NewRunsEnvelope()
	result.SetData(runData)

	return result
}
