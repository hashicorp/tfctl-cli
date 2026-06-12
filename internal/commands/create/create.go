// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package create implements the tfctl create command.
package create

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
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/resource"
	"github.com/hashicorp/tfctl-cli/version"
)

// Opts defines the options for the `create` command.
type Opts struct {
	api.Opts
	ProfileOrganization string
	Organization        string
	Args                []string
	client              *client.Client
}

// NewCmdCreate creates the `create` command.
func NewCmdCreate(inv *cmd.Invocation) *cmd.Command {
	opts := &Opts{}
	opts.IO = inv.IO
	opts.Output = inv.Output

	return &cmd.Command{
		Name:      "create",
		ShortHelp: "Create a resource",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s create" }} command creates a new resource via the API.

		Provide attributes using {{ template "mdCodeOrBold" "-a key=value" }} (repeatable) or a raw request body with {{ template "mdCodeOrBold" "-i" }}.
		Use {{ template "mdCodeOrBold" "-i -" }} to read the request body from stdin.

		Note: {{ template "mdCodeOrBold" "-a" }} only sets data.attributes. Resources that require a relationships block
		(e.g. variable sets, policy sets) must use {{ template "mdCodeOrBold" "-i" }} with a full JSON:API request body.
		`, version.Name),
		Args: cmd.PositionalArguments{
			Autocomplete: complete.PredictSet(resource.CreatableNames()...),
			Args: []cmd.PositionalArgument{
				{
					Name:          "RESOURCE",
					Documentation: "resource type to create (e.g. workspace, project)",
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
				{
					Name:         "attribute",
					Shorthand:    "a",
					DisplayValue: "KEY=VALUE",
					Description:  "Attribute for the JSON:API request body (repeatable)",
					Repeatable:   true,
					Value:        flagvalue.SimpleMap(nil, &opts.Attributes),
				},
				{
					Name:         "input",
					Shorthand:    "i",
					DisplayValue: "BODY",
					Description:  "Raw JSON request body (or - to read from stdin)",
					Value:        flagvalue.Simple("", &opts.InputRequest),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Create a workspace using attributes",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s create workspace -a name=my-workspace`, version.Name),
			},
			{
				Preamble: "Create a workspace from a JSON file",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s create workspace -i @workspace.json`, version.Name),
			},
			{
				Preamble: "Create a project with inline JSON",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s create project -i '{"data":{"type":"projects","attributes":{"name":"my-project"}}}'`, version.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			opts.DryRun = inv.IsDryRun()
			opts.Quiet = inv.GetGlobalFlags().Quiet
			opts.ProfileOrganization = inv.Profile.DefaultOrganization
			opts.Args = args

			client, err := inv.NewAPIClient()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}
			opts.client = client

			return runCreate(inv.ShutdownCtx, opts)
		},
	}
}

func runCreate(ctx context.Context, opts *Opts) error {
	if len(opts.Args) == 0 {
		return cmd.ErrDisplayUsage
	}

	resourceArg := opts.Args[0]
	res := resource.ByName(resourceArg)
	if res == nil {
		return fmt.Errorf("unknown resource type: %q\nAvailable resources: %s",
			resourceArg, strings.Join(resource.Names(), ", "))
	}

	if res.PathCreate == "" {
		return fmt.Errorf("create is not supported for %s", res.Type)
	}

	if len(opts.Attributes) == 0 && opts.InputRequest == "" {
		return fmt.Errorf("provide attributes with -a key=value or a request body with -i")
	}

	if len(opts.Attributes) > 0 && opts.InputRequest != "" {
		return fmt.Errorf("cannot use both -a (attributes) and -i (input body); choose one")
	}

	org := cmdutil.ResolveOrganization(opts.ProfileOrganization, opts.Organization)
	path, err := cmdutil.ResolvePath(res.PathCreate, org)
	if err != nil {
		return err
	}

	resolvedURL, err := client.ResolveURL(*opts.client.BaseURL, path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}

	apiOpts := api.NewOpts(opts.IO, opts.Output, opts.client)
	apiOpts.URL = resolvedURL
	apiOpts.Method = http.MethodPost
	apiOpts.Quiet = opts.Quiet
	apiOpts.DryRun = opts.DryRun
	apiOpts.InputRequest = opts.InputRequest
	apiOpts.Attributes = opts.Attributes

	// ResourceType is only needed for the attribute path (api builds the JSON:API
	// envelope from it). On the -i branch the user supplies the full body, so the
	// type is unused — but setting it is harmless and keeps diagnostic logging accurate.
	if len(opts.Attributes) > 0 {
		apiOpts.ResourceType = res.Type
	}

	return api.RunAPI(ctx, apiOpts)
}
