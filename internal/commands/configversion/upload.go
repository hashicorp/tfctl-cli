// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package configversion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdutil"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/configupload"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/version"
)

type uploadOpts struct {
	io            iostreams.IOStreams
	output        *format.Outputter
	resolver      *client.Resolver
	upload        func(context.Context, string, string, configupload.Options) (configupload.Result, error)
	workspace     string
	organization  string
	configDir     string
	uploadOptions configupload.Options
	dryRun        bool
}

func newCmdUpload(inv *cmd.Invocation) *cmd.Command {
	opts := uploadOpts{io: inv.IO, output: inv.Output}
	return &cmd.Command{
		Name:      "upload",
		ShortHelp: "Package and upload Terraform config to a workspace.",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s config-version upload" }} command packages a
		local Terraform config and upload it to a workspace. The command applies
		the default exclusions and any rules in .terraformignore at the root of
		the config directory.

		The command waits until the config version is ready unless you set
		{{ template "mdCodeOrBold" "--no-wait" }}. It does not queue a run unless you set
		{{ template "mdCodeOrBold" "--auto-queue-run" }}.

		Use a workspace ID or name. For a workspace name, the command uses the
		organization from {{ template "mdCodeOrBold" "--organization" }}, then the active
		profile, then Terraform config in the current directory.
		`, version.Name),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "WORKSPACE",
					Documentation: "Workspace ID or name.",
				},
			},
			Autocomplete: complete.PredictNothing, // Do not make API requests during completion.
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:         "config-dir",
					Description:  "Terraform config directory. Defaults to the current directory.",
					Value:        flagvalue.Simple(".", &opts.configDir),
					Autocomplete: complete.PredictDirs("*"),
				},
				{
					Name:         "organization",
					Description:  "Organization name. Defaults to the active profile or Terraform config in the current directory.",
					Value:        flagvalue.Simple("", &opts.organization),
					Autocomplete: complete.PredictNothing,
				},
				{
					Name:          "speculative",
					Description:   "Restrict the config version to speculative runs.",
					Value:         flagvalue.Simple(false, &opts.uploadOptions.Speculative),
					IsBooleanFlag: true,
				},
				{
					Name:          "provisional",
					Description:   "Do not set the workspace's current config when creating the version.",
					Value:         flagvalue.Simple(false, &opts.uploadOptions.Provisional),
					IsBooleanFlag: true,
				},
				{
					Name:          "auto-queue-run",
					Description:   "Let the server queue a run after upload.",
					Value:         flagvalue.Simple(false, &opts.uploadOptions.AutoQueueRun),
					IsBooleanFlag: true,
				},
				{
					Name:          "no-wait",
					Description:   "Return after upload without waiting for the server to process the config.",
					Value:         flagvalue.Simple(false, &opts.uploadOptions.NoWait),
					IsBooleanFlag: true,
				},
				{
					Name:         "timeout",
					Description:  "Maximum time to wait for the config version to be ready after upload.",
					Value:        flagvalue.Duration(configupload.DefaultWaitTimeout, &opts.uploadOptions.Timeout),
					Autocomplete: complete.PredictNothing,
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Upload a config directory and print only the config version ID:",
				Command:  fmt.Sprintf(`$ %s config-version upload ws-abc123 --config-dir ./production --jq '.id'`, version.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			run := opts
			run.workspace = args[0]
			run.organization = cmdutil.ResolveOrganization(inv.Profile.DefaultOrganization, opts.organization)
			run.dryRun = inv.IsDryRun()
			api, err := inv.NewAPIClient()
			if err != nil {
				return fmt.Errorf("create API client: %w", err)
			}
			run.resolver = client.NewResolver(api, false, run.dryRun)
			run.upload = configupload.NewUploader(api.TFE).Upload
			return runUpload(inv.ShutdownCtx, run)
		},
	}
}

func runUpload(ctx context.Context, opts uploadOpts) error {
	if opts.configDir == "" {
		return errors.New("--config-dir must not be empty")
	}
	if opts.uploadOptions.Timeout < 0 {
		return errors.New("--timeout must not be negative")
	}
	workspaceID, err := resolveWorkspace(ctx, opts)
	if err != nil {
		return err
	}
	if opts.dryRun {
		// Check only the directory itself so dry-run does not read config files.
		info, err := os.Lstat(filepath.Clean(opts.configDir))
		if err != nil {
			return fmt.Errorf("inspect config directory: %w", err)
		}
		if !info.IsDir() {
			return errors.New("config path must be a directory, not a file or symbolic link")
		}
		label := opts.io.ColorScheme().DryRunLabel()
		fmt.Fprintf(opts.io.Err(), "%s would package %q and upload it as a config version for workspace %s\n", label, opts.configDir, workspaceID)
		fmt.Fprintf(opts.io.Err(), "%s auto-queue-runs=%t, speculative=%t, provisional=%t\n", label, opts.uploadOptions.AutoQueueRun, opts.uploadOptions.Speculative, opts.uploadOptions.Provisional)
		return nil
	}
	fmt.Fprintf(opts.io.ErrUnessential(), "Packaging and uploading config from %q...\n", opts.configDir)
	opts.uploadOptions.OnStatus = func(id, status string) {
		fmt.Fprintf(opts.io.ErrUnessential(), "Config version %s: %s\n", id, status)
	}
	result, err := opts.upload(ctx, workspaceID, opts.configDir, opts.uploadOptions)
	if err != nil {
		// The shared cancellation handler does not print the wrapped error.
		if result.ID != "" && errors.Is(err, context.Canceled) {
			fmt.Fprintf(opts.io.Err(), "Config version %s was created.\n", result.ID)
		}
		return err
	}
	if err := opts.output.Display(uploadDisplayer{result: result}); err != nil {
		return fmt.Errorf("config version %s was uploaded, but output failed: %w", result.ID, err)
	}
	return nil
}

func resolveWorkspace(ctx context.Context, opts uploadOpts) (string, error) {
	if strings.HasPrefix(opts.workspace, "ws-") {
		return opts.workspace, nil
	}
	if opts.organization == "" {
		return "", errors.New("organization is required for a workspace name; use --organization or set a profile default")
	}
	workspace, err := opts.resolver.Workspace(ctx, opts.organization, opts.workspace)
	if err != nil {
		return "", err
	}
	if workspace.GetId() == nil || *workspace.GetId() == "" {
		return "", errors.New("resolved workspace has no ID")
	}
	return *workspace.GetId(), nil
}
