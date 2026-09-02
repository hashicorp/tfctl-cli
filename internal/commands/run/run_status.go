// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
	"github.com/hashicorp/tfctl-cli/version"
)

// StatusOpts stores the options parsed from flags for the run status command.
type StatusOpts struct {
	IO           iostreams.IOStreams
	Output       *format.Outputter
	Client       *client.Client
	Organization string
	ID           string
}

// NewCmdRunStatus creates the `run status` command.
func NewCmdRunStatus(inv *cmd.Invocation) *cmd.Command {
	opts := &StatusOpts{
		IO: inv.IO,
	}
	var organization string

	cmd := &cmd.Command{
		Name:      "status",
		ShortHelp: "Show the status of a run, printing diagnostics if it failed.",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s run status" }} command inspects an HCP Terraform run and prints its current status. If the run has errored, it renders all failures, in the following order: terraform diagnostics, policy failures, and run task failures.

		The ID argument can be:
		- A run ID ({{ template "mdCodeOrBold" "run-..." }})
		- A workspace ID ({{ template "mdCodeOrBold" "ws-..." }}) to get the latest run
		- A workspace name to get the latest run (may require {{ template "mdCodeOrBold" "--organization" }})
		`, version.Name),
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
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run status run-abc123`, version.Name),
			},
			{
				Preamble: "Check the latest run in a workspace by name",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run status my-workspace --organization my-org`, version.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			if len(args) != 1 {
				return cmd.ErrDisplayUsage
			}

			org := organization
			if org == "" {
				org = inv.Profile.DefaultOrganization
			}
			if org == "" {
				cfg, err := terraformcfg.FindCloudConfig(".")
				if err == nil && cfg.Organization != "" {
					org = cfg.Organization
				}
			}

			apiClient, err := inv.NewAPIClient()
			if err != nil {
				return fmt.Errorf("unable to create API client: %w", err)
			}

			opts.Output = inv.Output
			opts.Client = apiClient
			opts.Organization = org
			opts.ID = args[0]

			return runStatus(inv.ShutdownCtx, opts)
		},
	}

	return cmd
}

// runStatus displays the status of a run, including diagnostics if it failed.
// Several problems can contribute to a single failed run, and all are displayed in order of
// severity: terraform diagnostics, policy check failures, policy evaluation failures, and
// task failures.
func runStatus(ctx context.Context, opts *StatusOpts) error {
	resolver := client.NewResolver(opts.Client, false, false)

	id := opts.ID
	resourceType := "workspaces"
	switch {
	case strings.HasPrefix(id, "run-"):
		resourceType = "runs"
	case strings.HasPrefix(id, "ws-"):
		resourceType = "workspaces"
	default:
		if opts.Organization == "" {
			return fmt.Errorf("--organization is required when specifying a workspace name")
		}
	}

	runID, err := resolver.RunOrCurrentRun(ctx, opts.Organization, resourceType, id)
	if err != nil {
		return err
	}

	summary, err := client.NewRunSummary(ctx, opts.Client, runID)
	if err != nil {
		return err
	}

	if err := opts.Output.Display(&summaryDisplayer{summary: summary, io: opts.IO}); err != nil {
		return err
	}

	switch summary.Status {
	case "errored", "policy_soft_failed", "policy_override":
		return cmd.ErrUnderlyingError
	}
	return nil
}
