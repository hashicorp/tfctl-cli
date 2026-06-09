// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package create implements the tfctl create command.
package create

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/go-hclog"
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
	Organization string
	Attributes   map[string]string
	InputBody    string
	DryRun       bool
	Quiet        bool
}

// NewCmdCreate creates the `create` command.
func NewCmdCreate(ctx *cmd.Context) *cmd.Command {
	opts := &Opts{}

	return &cmd.Command{
		Name:      "create",
		ShortHelp: "Create a resource",
		LongHelp: heredoc.New(ctx.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s create" }} command creates a new resource via the API.

		Provide attributes using {{ template "mdCodeOrBold" "-a key=value" }} (repeatable) or a raw request body with {{ template "mdCodeOrBold" "-i" }}.
		The input body can be inline JSON, a file path, {{ template "mdCodeOrBold" "@filename" }} to read from a file, or {{ template "mdCodeOrBold" "-" }} for stdin.

		Note: {{ template "mdCodeOrBold" "-a" }} only sets data.attributes. Resources that require a relationships block
		(e.g. variable sets, policy sets, variables) must use {{ template "mdCodeOrBold" "-i" }} with a full JSON:API request body.
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
					Description:  "Raw JSON request body, file path, @filename, or - for stdin",
					Value:        flagvalue.Simple("", &opts.InputBody),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Create a workspace using attributes",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s create workspace -a name=my-workspace`, version.Name),
			},
			{
				Preamble: "Create a workspace from a JSON file",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s create workspace -i @workspace.json`, version.Name),
			},
			{
				Preamble: "Create a project with inline JSON",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s create project -i '{"data":{"type":"projects","attributes":{"name":"my-project"}}}'`, version.Name),
			},
		},
		RunF: func(c *cmd.Command, args []string) error {
			opts.DryRun = ctx.IsDryRun()
			opts.Quiet = ctx.Profile.IsQuiet()
			logger := c.Logger(ctx)
			return runCreate(ctx, opts, logger, args)
		},
	}
}

func runCreate(ctx *cmd.Context, opts *Opts, logger hclog.Logger, args []string) error {
	if len(args) == 0 {
		return cmd.ErrDisplayUsage
	}

	resourceArg := args[0]
	res := resource.ByName(resourceArg)
	if res == nil {
		return fmt.Errorf("unknown resource type: %q\nAvailable resources: %s",
			resourceArg, strings.Join(resource.Names(), ", "))
	}

	if res.PathCreate == "" {
		return fmt.Errorf("create is not supported for %s", res.Type)
	}

	if len(opts.Attributes) == 0 && opts.InputBody == "" {
		return fmt.Errorf("provide attributes with -a key=value or a request body with -i")
	}

	if len(opts.Attributes) > 0 && opts.InputBody != "" {
		return fmt.Errorf("cannot use both -a (attributes) and -i (input body); choose one")
	}

	org := cmdutil.ResolveOrganization(ctx, opts.Organization)
	path, err := cmdutil.ResolvePath(res.PathCreate, org)
	if err != nil {
		return err
	}

	apiClient, err := ctx.NewAPIClient(logger)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	resolvedURL, err := client.ResolveURL(*apiClient.BaseURL, path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}

	// Handle @filename syntax: strip the @ and let RunAPI read the file.
	inputBody := strings.TrimPrefix(opts.InputBody, "@")

	apiOpts := api.NewOpts(ctx.ShutdownCtx, ctx.IO, ctx.Output, logger, apiClient)
	apiOpts.URL = resolvedURL
	apiOpts.Method = http.MethodPost
	apiOpts.Quiet = opts.Quiet
	apiOpts.DryRun = opts.DryRun
	apiOpts.ResourceType = res.Type
	apiOpts.InputRequest = inputBody
	if opts.Attributes != nil {
		apiOpts.Attributes = opts.Attributes
	}

	return api.RunAPI(apiOpts)
}
