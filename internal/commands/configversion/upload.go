// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package configversion implements the config-version command group.
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

// NewCmdConfigVersion creates the config-version command group.
func NewCmdConfigVersion(inv *cmd.Invocation) *cmd.Command {
	c := &cmd.Command{Name: "config-version", ShortHelp: "Upload Terraform config to a workspace."}
	c.AddChild(newCmdUpload(inv))
	return c
}

type uploadOpts struct {
	io                                 iostreams.IOStreams
	output                             *format.Outputter
	resolver                           *client.Resolver
	upload                             func(context.Context, string, string, configupload.Options) (configupload.Result, error)
	workspace, organization, configDir string
	config                             configupload.Options
	dryRun                             bool
}

func newCmdUpload(inv *cmd.Invocation) *cmd.Command {
	opts := uploadOpts{io: inv.IO, output: inv.Output, configDir: "."}
	return &cmd.Command{
		Name:      "upload",
		ShortHelp: "Pack and upload Terraform config for a workspace.",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s config-version upload" }} command packs a
		Terraform config directory and uploads it to a workspace. It applies
		.terraformignore and the default exclusions. It waits for the uploaded status
		unless {{ template "mdCodeOrBold" "--no-wait" }} is set.

		The command does not create a run unless {{ template "mdCodeOrBold" "--auto-queue-run" }} is set.
		WORKSPACE is a workspace ID or a workspace name. A workspace name requires an
		organization from the flag, profile, or local Terraform context.
		`, version.Name),
		Args: cmd.PositionalArguments{
			Args:         []cmd.PositionalArgument{{Name: "WORKSPACE", Documentation: "workspace ID or workspace name"}},
			Autocomplete: complete.PredictNothing, // Do not make API requests during completion.
		},
		Flags: cmd.Flags{Local: []*cmd.Flag{
			{Name: "config-dir", Description: "Terraform config directory to pack", Value: flagvalue.Simple(".", &opts.configDir), Autocomplete: complete.PredictDirs("*")},
			{Name: "organization", Description: "Organization name (defaults to profile or Terraform context)", Value: flagvalue.Simple("", &opts.organization), Autocomplete: complete.PredictNothing},
			{Name: "speculative", Description: "Restrict the config version to speculative runs", Value: flagvalue.Simple(false, &opts.config.Speculative), IsBooleanFlag: true},
			{Name: "provisional", Description: "Do not immediately make this the workspace's current config", Value: flagvalue.Simple(false, &opts.config.Provisional), IsBooleanFlag: true},
			{Name: "auto-queue-run", Description: "Let the server create a run after upload", Value: flagvalue.Simple(false, &opts.config.AutoQueueRun), IsBooleanFlag: true},
			{Name: "no-wait", Description: "Return after upload without waiting for processing", Value: flagvalue.Simple(false, &opts.config.NoWait), IsBooleanFlag: true},
			{Name: "timeout", Description: "Maximum time to wait for the uploaded status", Value: flagvalue.Duration(configupload.DefaultWaitTimeout, &opts.config.Timeout), Autocomplete: complete.PredictNothing},
		}},
		Examples: []cmd.Example{
			{Preamble: "Upload a directory and return its config version ID:", Command: fmt.Sprintf(`$ %s config-version upload ws-abc123 --config-dir ./production --jq '.id'`, version.Name)},
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
	if opts.config.Timeout < 0 {
		return errors.New("--timeout must not be negative")
	}
	workspaceID, err := resolveWorkspace(ctx, opts)
	if err != nil {
		return err
	}
	if opts.dryRun {
		// Check the directory itself without traversing it or packing any files.
		info, err := os.Lstat(filepath.Clean(opts.configDir))
		if err != nil {
			return fmt.Errorf("inspect config directory: %w", err)
		}
		if !info.IsDir() {
			return errors.New("config path must be a directory, not a file or symbolic link")
		}
		label := opts.io.ColorScheme().DryRunLabel()
		fmt.Fprintf(opts.io.Err(), "%s would pack %q and upload it as a config version for workspace %s\n", label, opts.configDir, workspaceID)
		fmt.Fprintf(opts.io.Err(), "%s auto-queue-runs=%t, speculative=%t, provisional=%t\n", label, opts.config.AutoQueueRun, opts.config.Speculative, opts.config.Provisional)
		return nil
	}
	fmt.Fprintf(opts.io.ErrUnessential(), "Packing and uploading config from %q...\n", opts.configDir)
	opts.config.OnStatus = func(id, status string) {
		fmt.Fprintf(opts.io.ErrUnessential(), "Config version %s: %s\n", id, status)
	}
	result, err := opts.upload(ctx, workspaceID, opts.configDir, opts.config)
	if err != nil {
		return err
	}
	if err := opts.output.Display(uploadDisplayer{result}); err != nil {
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

type uploadDisplayer struct{ result configupload.Result }

func (d uploadDisplayer) DefaultFormat() format.Format { return format.Pretty }
func (d uploadDisplayer) Payload() any                 { return d.result }
func (d uploadDisplayer) FieldTemplates() []format.Field {
	return []format.Field{
		format.NewField("ID", "{{ .ID }}"),
		format.NewField("Workspace ID", "{{ .WorkspaceID }}"),
		format.NewField("Status", "{{ .Status }}"),
		format.NewField("Speculative", "{{ .Speculative }}"),
		format.NewField("Provisional", "{{ .Provisional }}"),
		format.NewField("Auto Queue Run", "{{ .AutoQueueRun }}"),
	}
}
