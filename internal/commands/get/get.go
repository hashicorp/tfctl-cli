// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package get implements the tfctl get command.
package get

import (
	"fmt"
	"net/http"
	"regexp"
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

// reIDShape matches strings that look like prefixed IDs: one or more letters
// followed by a dash and at least one alphanumeric character (e.g. "ws-abc123").
var reIDShape = regexp.MustCompile(`^[a-zA-Z]+-[a-zA-Z0-9]`)

// Opts defines the options for the `get` command.
type Opts struct {
	Organization string
	All          bool
	PageSize     int
	PageNumber   int
	DryRun       bool
	Quiet        bool
}

// NewCmdGet creates the `get` command.
func NewCmdGet(ctx *cmd.Context) *cmd.Command {
	opts := &Opts{}

	return &cmd.Command{
		Name:      "get",
		ShortHelp: "Get or list a resource",
		LongHelp: heredoc.New(ctx.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s get" }} command fetches a single resource by ID or lists resources by type.

		When given a resource type name (e.g. {{ template "mdCodeOrBold" "workspaces" }}), the command lists all resources of that type.
		When given an ID (e.g. {{ template "mdCodeOrBold" "ws-abc123" }}), it fetches the single resource.
		A two-argument form allows specifying both type and ID: {{ template "mdCodeOrBold" "get workspace ws-abc123" }}.
		`, version.Name),
		Args: cmd.PositionalArguments{
			Autocomplete: complete.PredictSet(resource.CompletionNames()...),
			Args: []cmd.PositionalArgument{
				{
					Name:          "RESOURCE_OR_ID",
					Documentation: "resource type name or resource ID",
				},
				{
					Name:          "ID",
					Documentation: "resource ID (when first argument is the resource type)",
					Optional:      true,
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
					Name:          "all",
					Description:   "Paginate through all results",
					Value:         flagvalue.Simple(false, &opts.All),
					IsBooleanFlag: true,
				},
				{
					Name:         "page-size",
					Shorthand:    "s",
					Description:  "Number of results per page",
					DisplayValue: "SIZE",
					Value:        flagvalue.Simple(0, &opts.PageSize),
				},
				{
					Name:         "page-number",
					Shorthand:    "n",
					Description:  "Page number to fetch (1-indexed)",
					DisplayValue: "NUMBER",
					Value:        flagvalue.Simple(0, &opts.PageNumber),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "List all workspaces in the active organization",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get workspaces`, version.Name),
			},
			{
				Preamble: "Fetch a single workspace by ID",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get ws-abc123`, version.Name),
			},
			{
				Preamble: "Fetch a workspace by type and ID",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get workspace ws-abc123`, version.Name),
			},
			{
				Preamble: "List all workspaces (paginate through all pages)",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get workspaces --all`, version.Name),
			},
		},
		RunF: func(c *cmd.Command, args []string) error {
			opts.DryRun = ctx.IsDryRun()
			opts.Quiet = ctx.Profile.IsQuiet()
			logger := c.Logger(ctx)
			return runGet(ctx, opts, logger, args)
		},
	}
}

func runGet(ctx *cmd.Context, opts *Opts, logger hclog.Logger, args []string) error {
	if len(args) == 0 {
		return cmd.ErrDisplayUsage
	}

	if len(args) == 1 {
		return runGetSingleArg(ctx, opts, logger, args[0])
	}

	return runGetTwoArgs(ctx, opts, logger, args[0], args[1])
}

func runGetSingleArg(ctx *cmd.Context, opts *Opts, logger hclog.Logger, arg string) error {
	// Check if arg is a known resource type name (list mode).
	if res := resource.ByName(arg); res != nil {
		return runList(ctx, opts, logger, res)
	}

	// Check if arg looks like a known resource ID (get by ID mode).
	if res := resource.ByIDPrefix(arg); res != nil {
		return runGetByID(ctx, opts, logger, res, arg)
	}

	// Distinguish "looks like an ID but prefix is unknown" from "not a type name".
	if reIDShape.MatchString(arg) {
		return fmt.Errorf("unrecognized ID prefix in %q\nKnown prefixes resolve automatically — use `api /path/{id}` for other resource types", arg)
	}

	return fmt.Errorf("unknown resource type: %q\nAvailable resources: %s",
		arg, strings.Join(resource.Names(), ", "))
}

func runGetTwoArgs(ctx *cmd.Context, opts *Opts, logger hclog.Logger, resourceArg, id string) error {
	res := resource.ByName(resourceArg)
	if res == nil {
		return fmt.Errorf("unknown resource type: %q\nAvailable resources: %s",
			resourceArg, strings.Join(resource.Names(), ", "))
	}

	// Defensive: all current registry entries have PathGet, but future ones may not.
	if res.PathGet == "" {
		return fmt.Errorf("get is not supported for %s", res.Type)
	}

	// Validate that the ID prefix matches the resource type, if the resource has a known prefix.
	if res.IDPrefix != "" && !strings.HasPrefix(id, res.IDPrefix) {
		return fmt.Errorf("ID %q does not look like a %s resource (expected prefix %q)", id, resourceArg, res.IDPrefix)
	}

	path := strings.ReplaceAll(res.PathGet, "{id}", id)
	return executeGetRequest(ctx, opts, logger, path)
}

func runList(ctx *cmd.Context, opts *Opts, logger hclog.Logger, res *resource.Resource) error {
	if res.PathList == "" {
		return fmt.Errorf("listing is not supported for %s", res.Type)
	}

	org := cmdutil.ResolveOrganization(ctx, opts.Organization)
	path, err := cmdutil.ResolvePath(res.PathList, org)
	if err != nil {
		return err
	}

	return executeGetRequest(ctx, opts, logger, path)
}

func runGetByID(ctx *cmd.Context, opts *Opts, logger hclog.Logger, res *resource.Resource, id string) error {
	if res.PathGet == "" {
		return fmt.Errorf("get is not supported for %s", res.Type)
	}

	path := strings.ReplaceAll(res.PathGet, "{id}", id)
	return executeGetRequest(ctx, opts, logger, path)
}

func executeGetRequest(ctx *cmd.Context, opts *Opts, logger hclog.Logger, path string) error {
	apiClient, err := ctx.NewAPIClient(logger)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	resolvedURL, err := client.ResolveURL(*apiClient.BaseURL, path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}

	apiOpts := api.NewOpts(ctx.ShutdownCtx, ctx.IO, ctx.Output, logger, apiClient)
	apiOpts.URL = resolvedURL
	apiOpts.Method = http.MethodGet
	apiOpts.Quiet = opts.Quiet
	apiOpts.DryRun = opts.DryRun
	apiOpts.All = opts.All
	apiOpts.PageSize = opts.PageSize
	apiOpts.PageNumber = opts.PageNumber

	return api.RunAPI(apiOpts)
}
