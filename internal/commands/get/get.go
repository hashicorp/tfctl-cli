// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package get implements the tfctl get command.
package get

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
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

// reIDShape matches strings that look like prefixed IDs: 2-10 lowercase letters,
// a dash, and at least 6 alphanumeric characters (e.g. "ws-abc123def456gh").
var reIDShape = regexp.MustCompile(`^[a-z]{2,10}-[a-zA-Z0-9]{6,}`)

// Opts defines the options for the `get` command.
type Opts struct {
	api.Opts
	ProfileOrganization string
	Args                []string
	Organization        string

	client *client.Client
}

// NewCmdGet creates the `get` command.
func NewCmdGet(inv *cmd.Invocation) *cmd.Command {
	opts := &Opts{}
	opts.IO = inv.IO
	opts.Output = inv.Output

	return &cmd.Command{
		Name:      "get",
		ShortHelp: "Get or list a resource",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
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
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get workspaces`, version.Name),
			},
			{
				Preamble: "Fetch a single workspace by ID",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get ws-abc123`, version.Name),
			},
			{
				Preamble: "Fetch a workspace by type and ID",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get workspace ws-abc123`, version.Name),
			},
			{
				Preamble: "List all workspaces (paginate through all pages)",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s get workspaces --all`, version.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			opts.DryRun = inv.IsDryRun()
			opts.ProfileOrganization = inv.Profile.Organization
			opts.Quiet = inv.Profile.IsQuiet()
			opts.Args = args
			opts.IO = inv.IO
			opts.Output = inv.Output
			opts.Method = http.MethodGet
			client, err := inv.NewAPIClient()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}
			opts.client = client

			return runGet(inv.ShutdownCtx, opts)
		},
	}
}

func runGet(ctx context.Context, opts *Opts) error {
	if len(opts.Args) == 0 {
		return cmd.ErrDisplayUsage
	}

	if len(opts.Args) == 1 {
		return runGetSingleArg(ctx, opts, opts.Args[0])
	}

	return runGetTwoArgs(ctx, opts, opts.Args[0], opts.Args[1])
}

func runGetSingleArg(ctx context.Context, opts *Opts, arg string) error {
	// Check if arg is a known resource type name (list mode).
	if res := resource.ByName(arg); res != nil {
		return runList(ctx, opts, res)
	}

	// Check if arg looks like a known resource ID (get by ID mode).
	if res := resource.ByIDPrefix(arg); res != nil {
		return runGetByID(ctx, opts, res, arg)
	}

	// Distinguish "looks like an ID but prefix is unknown" from "not a type name".
	if reIDShape.MatchString(arg) {
		return fmt.Errorf("unrecognized ID prefix in %q\nKnown prefixes resolve automatically — use `api /path/{id}` for other resource types", arg)
	}

	return fmt.Errorf("unknown resource type: %q\nAvailable resources: %s",
		arg, strings.Join(resource.Names(), ", "))
}

func runGetTwoArgs(ctx context.Context, opts *Opts, resourceArg, id string) error {
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
	return executeGetRequest(ctx, opts, path)
}

func runList(ctx context.Context, opts *Opts, res *resource.Resource) error {
	if res.PathList == "" {
		return fmt.Errorf("listing is not supported for %s", res.Type)
	}

	org := cmdutil.ResolveOrganization(opts.ProfileOrganization, opts.Organization)
	path, err := cmdutil.ResolvePath(res.PathList, org)
	if err != nil {
		return err
	}

	return executeGetRequest(ctx, opts, path)
}

func runGetByID(ctx context.Context, opts *Opts, res *resource.Resource, id string) error {
	if res.PathGet == "" {
		return fmt.Errorf("get is not supported for %s", res.Type)
	}

	path := strings.ReplaceAll(res.PathGet, "{id}", id)
	return executeGetRequest(ctx, opts, path)
}

func executeGetRequest(ctx context.Context, opts *Opts, path string) error {
	resolvedURL, err := client.ResolveURL(*opts.client.BaseURL, path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}

	apiOpts := api.NewOpts(opts.IO, opts.Output, opts.client)
	apiOpts.URL = resolvedURL
	apiOpts.Method = http.MethodGet
	apiOpts.All = opts.All
	apiOpts.PageSize = opts.PageSize
	apiOpts.PageNumber = opts.PageNumber
	apiOpts.DryRun = opts.DryRun
	apiOpts.Quiet = opts.Quiet

	return api.RunAPI(ctx, apiOpts)
}
