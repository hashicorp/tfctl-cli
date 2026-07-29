// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package destroy implements the tfctl destroy command.
package destroy

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/commands/api"
	"github.com/hashicorp/tfctl-cli/internal/commands/cmdutil"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/execsession"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/resource"
	"github.com/hashicorp/tfctl-cli/version"
)

// Opts defines the options for the `destroy` command.
type Opts struct {
	api.Opts
	ProfileOrganization string
	Organization        string
	Args                []string
	client              *client.Client
}

// NewCmdDestroy creates the `destroy` command.
func NewCmdDestroy(inv *cmd.Invocation) *cmd.Command {
	opts := &Opts{}
	opts.IO = inv.IO
	opts.Output = inv.Output

	return &cmd.Command{
		Name:      "destroy",
		ShortHelp: "Destroy a resource",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s destroy" }} command destroys (deletes) a resource via the API.

		Provide the resource type and its ID. The delete path is derived from the
		resource registry, so the resource type determines the correct API endpoint
		regardless of path structure.
		`, version.Name),
		Args: cmd.PositionalArguments{
			Autocomplete: complete.PredictSet(resource.DestroyableNames()...),
			Args: []cmd.PositionalArgument{
				{
					Name:          "RESOURCE",
					Documentation: "resource type to destroy (e.g. workspace, project)",
				},
				{
					Name:          "ID",
					Documentation: "the resource ID to destroy",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:        "organization",
					Shorthand:   "o",
					Description: "Organization name (defaults to profile or terraform cloud config context)",
					Value:       flagvalue.Simple("", &opts.Organization),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Destroy a workspace",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s destroy workspace ws-abc123`, version.Name),
			},
			{
				Preamble: "Destroy an explorer saved query",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s destroy explorer-saved-queries sq-abc123 -o my-org`, version.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			opts.DryRun = inv.IsDryRun()
			opts.Quiet = inv.IsQuiet()
			opts.ProfileOrganization = inv.Profile.DefaultOrganization
			opts.Args = args

			apiClient, err := inv.NewAPIClient()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}
			opts.client = apiClient

			return runDestroy(inv.ShutdownCtx, opts)
		},
	}
}

func runDestroy(ctx context.Context, opts *Opts) error {
	if len(opts.Args) < 2 {
		return cmd.ErrDisplayUsage
	}

	resourceArg := opts.Args[0]
	id := opts.Args[1]

	res := resource.ByNameOrAlias(resourceArg)
	if res == nil {
		return fmt.Errorf("unknown resource type: %q\nAvailable resources: %s",
			resourceArg, strings.Join(resource.Names(), ", "))
	}

	if res.Destroyable == resource.NotDestroyable {
		return fmt.Errorf("destroy is not supported for %s", res.Type)
	}

	if res.PathGet == "" {
		return fmt.Errorf("destroy path is not available for %s", res.Type)
	}

	if res.IDPrefix != "" && !strings.HasPrefix(id, res.IDPrefix) {
		return fmt.Errorf("ID %q does not look like a %s resource (expected prefix %q)", id, res.Type, res.IDPrefix)
	}

	path := strings.ReplaceAll(res.PathGet, "{id}", id)

	if strings.Contains(path, "{organization_name}") {
		org := cmdutil.ResolveOrganization(opts.ProfileOrganization, opts.Organization)
		var err error
		path, err = cmdutil.ResolvePath(path, org)
		if err != nil {
			return err
		}
	}

	resolvedURL, err := client.ResolveURL(*opts.client.BaseURL, path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}

	apiOpts := api.NewOpts(opts.IO, opts.Output, opts.client)
	apiOpts.URL = resolvedURL
	apiOpts.Method = http.MethodDelete
	apiOpts.ResourceType = res.Type
	apiOpts.Quiet = opts.Quiet
	apiOpts.DryRun = opts.DryRun

	if store, err := execsession.DefaultStore(); err == nil {
		apiOpts.Authorizer = &execsession.EnvAuthorizer{Store: store}
	}

	return api.RunAPI(ctx, apiOpts)
}
