// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package variable

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
	"github.com/hashicorp/tfctl-cli/version"
)

// ImportOpts stores the options parsed from flags for the variable import command.
type ImportOpts struct {
	IO                 iostreams.IOStreams
	Logger             hclog.Logger
	ShutdownCtx        context.Context
	TFVarsFileToImport string
	Client             *client.Client
	Env                []string
	VariableSetName    string
	Organization       string
	Workspace          string
	Overwrite          bool
	DryRun             bool
}

type existingVariables map[string]existingVariable

func (e existingVariables) Add(v existingVariable) {
	e[v.Category+"\x00"+v.Key] = v
}

func (e existingVariables) Get(category, key string) (existingVariable, bool) {
	result, ok := e[category+"\x00"+key]
	return result, ok
}

// NewCmdVariableImport creates the `variable import` command.
func NewCmdVariableImport(ctx *cmd.Context) *cmd.Command {
	opts := &ImportOpts{
		IO:          ctx.IO,
		ShutdownCtx: ctx.ShutdownCtx,
	}

	cmd := &cmd.Command{
		Name:      "import",
		ShortHelp: "Import variables from .tfvars or current env into workspaces or variable sets.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s variable import" }} command lets you import Terraform
		variables from .tfvars files or environment variables from the %s process environment into
		Workspaces or Variable Sets.
		`, version.Name, version.Name),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "TFVARS_FILE",
					Optional:      true,
					Documentation: "The .tfvars file to import variables from",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:        "env",
					Shorthand:   "e",
					Description: "Environment variable to import",
					Repeatable:  true,
					Value:       flagvalue.SimpleSlice(nil, &opts.Env),
				},
				{
					Name:        "variable-set-name",
					Description: "Target Variable Set by name (defaults to workspace if not set)",
					Value:       flagvalue.Simple("", &opts.VariableSetName),
				},
				{
					Name:        "organization",
					Description: "Organization name (defaults to config or terraform cloud config context)",
					Value:       flagvalue.Simple("", &opts.Organization),
				},
				{
					Name:        "workspace",
					Description: "Workspace name override (defaults to terraform cloud config context)",
					Value:       flagvalue.Simple("", &opts.Workspace),
				},
				{
					Name:          "overwrite",
					Description:   "Update matching existing variables instead of erroring",
					Value:         flagvalue.Simple(false, &opts.Overwrite),
					IsBooleanFlag: true,
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Import terraform variables from a .tfvars file into the current workspace",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s variable import variables.tfvars`, version.Name),
			},
			{
				Preamble: fmt.Sprintf("Import environment variables from the %s process into a variable set", version.Name),
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s variable import -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY --variable-set-name my-variable-set`, version.Name),
			},
		},
		RunF: func(c *cmd.Command, args []string) error {
			if len(args) > 1 {
				return cmd.ErrDisplayUsage
			}

			if len(args) == 1 {
				opts.TFVarsFileToImport = args[0]
			}

			if opts.Organization == "" {
				opts.Organization = ctx.Profile.Organization
			}

			if opts.Organization == "" || opts.Workspace == "" {
				cfg, err := terraformcfg.FindCloudConfig(".")
				if err == nil {
					if opts.Organization == "" {
						opts.Organization = cfg.Organization
					}
					if opts.Workspace == "" {
						opts.Workspace = cfg.Workspace
					}
				}
			}

			if opts.VariableSetName != "" && opts.Organization == "" {
				return errors.New("--organization or profile default organization is required when targeting a variable set and no terraform cloud configuration was found")
			}
			if opts.VariableSetName == "" && (opts.Organization == "" || opts.Workspace == "") {
				return errors.New("could not resolve target workspace; set --organization and --workspace or run inside a repository with terraform cloud configuration") // this should be impossible to hit due to the previous block, but we'll check again before API calls just in case
			}

			apiClient, err := ctx.NewAPIClient(c.Logger(ctx))
			if err != nil {
				return fmt.Errorf("unable to create API client: %w", err)
			}

			opts.Client = apiClient
			opts.Logger = c.Logger(ctx)
			opts.DryRun = ctx.IsDryRun()

			return runVariableImport(opts)
		},
	}

	return cmd
}

func runVariableImport(opts *ImportOpts) error {
	var imported []terraformcfg.ImportedVariable
	if opts.TFVarsFileToImport != "" {
		opts.Logger.Debug("parsing tfvars file", "path", opts.TFVarsFileToImport)
		vars, err := terraformcfg.ParseTFVarsFile(opts.TFVarsFileToImport)
		if err != nil {
			return fmt.Errorf("failed parsing tfvars file: %w", err)
		}
		imported = append(imported, vars...)
	}

	for _, name := range opts.Env {
		value, ok := os.LookupEnv(name)
		if !ok {
			return fmt.Errorf("environment variable %q is not set", name)
		}
		imported = append(imported, terraformcfg.ImportedVariable{
			Key:       name,
			Value:     value,
			Category:  "env",
			HCL:       false,
			Sensitive: true,
		})
	}

	if len(imported) == 0 {
		return cmd.ErrDisplayUsage
	}

	opts.Logger.Debug("resolving target", "organization", opts.Organization, "workspace", opts.Workspace, "variable_set", opts.VariableSetName)

	target, err := resolveTarget(opts.ShutdownCtx, opts)
	if err != nil {
		if opts.DryRun && opts.VariableSetName != "" {
			// Variable set doesn't exist yet; report what would happen.
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.Err(), "%s would create variable set %q\n", cs.DryRunLabel(), opts.VariableSetName)
			for _, variable := range imported {
				fmt.Fprintf(opts.IO.Err(), "%s would create %s variable %q in variable set %q\n", cs.DryRunLabel(), variable.Category, variable.Key, opts.VariableSetName)
			}
			return nil
		}
		return err
	}

	opts.Logger.Debug("importing variables", "count", len(imported), "target", target.String(), "overwrite", opts.Overwrite)

	existing, err := target.listExistingVariables(opts.ShutdownCtx)
	if err != nil {
		return err
	}

	duplicates := make([]string, 0)
	for _, variable := range imported {
		if _, ok := existing.Get(variable.Category, variable.Key); ok && !opts.Overwrite {
			duplicates = append(duplicates, fmt.Sprintf("%s (%s)", variable.Key, variable.Category))
		}
	}

	if len(duplicates) > 0 {
		return fmt.Errorf("variables already exist; rerun with --overwrite to update: %s", strings.Join(duplicates, ", "))
	}

	created := 0
	updated := 0
	cs := opts.IO.ColorScheme()
	for _, variable := range imported {
		if current, ok := existing.Get(variable.Category, variable.Key); ok {
			if opts.DryRun {
				fmt.Fprintf(opts.IO.Err(), "%s would update %s variable %q in %s\n", cs.DryRunLabel(), variable.Category, variable.Key, target.String())
				updated++
				continue
			}
			if err := target.updateVariable(opts.ShutdownCtx, current.ID, variable); err != nil {
				return err
			}
			updated++
			continue
		}
		if opts.DryRun {
			fmt.Fprintf(opts.IO.Err(), "%s would create %s variable %q in %s\n", cs.DryRunLabel(), variable.Category, variable.Key, target.String())
			created++
			continue
		}
		if err := target.createVariable(opts.ShutdownCtx, variable); err != nil {
			return err
		}
		created++
	}

	if opts.DryRun {
		return nil
	}

	fmt.Fprintf(opts.IO.Err(), "%s imported %d variables into %s (%d created, %d updated)", opts.IO.ColorScheme().SuccessIcon(), len(imported), target.String(), created, updated)
	return nil
}

func resolveTarget(ctx context.Context, opts *ImportOpts) (*variableTarget, error) {
	resolver := client.NewResolver(opts.Client, opts.VariableSetName != "" && !opts.DryRun, false)

	if opts.VariableSetName != "" {
		result, err := resolver.VariableSet(ctx, opts.Organization, opts.VariableSetName)
		if err != nil {
			return nil, err
		}
		return newVariableSetVariableTarget(opts.Client, *result, opts.VariableSetName), nil
	}

	workspace, err := opts.Client.TFE.API.Organizations().ByOrganization_name(opts.Organization).Workspaces().ByWorkspace_name(opts.Workspace).Get(ctx, nil)
	if err != nil {
		return nil, err
	}
	return newWorkspaceVariableTarget(opts.Client, *workspace.GetData().GetId(), opts.Workspace), nil
}
