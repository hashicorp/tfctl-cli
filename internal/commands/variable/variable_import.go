// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package variable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/hashicorp/tfcloud/internal/pkg/client"
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	terraformcfg "github.com/hashicorp/tfcloud/internal/pkg/terraform"
)

// ImportOpts stores the options parsed from flags for the variable import command.
type ImportOpts struct {
	IO              iostreams.IOStreams
	Env             []string
	VariableSetName string
	Organization    string
	Workspace       string
	Overwrite       bool
}

// NewCmdVariableImport creates the `tfcloud variable import` command.
func NewCmdVariableImport(ctx *cmd.Context) *cmd.Command {
	opts := &ImportOpts{
		IO: ctx.IO,
	}

	cmd := &cmd.Command{
		Name:      "import",
		ShortHelp: "Import variables from .tfvars or current env into workspaces or variable sets.",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud variable import" }} command lets you import Terraform
		variables from .tfvars files or environment variables from the tfcloud process environment into
		Workspaces or Variable Sets.
		`),
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
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud variable import variables.tfvars`),
			},
			{
				Preamble: "Import environment variables from the tfcloud process into a variable set",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud variable import -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY --variable-set-name my-variable-set`),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			var imported []terraformcfg.ImportedVariable
			if len(args) > 1 {
				return cmd.ErrDisplayUsage
			}

			if len(args) == 1 {
				vars, err := terraformcfg.ParseTFVarsFile(args[0])
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

			target, err := resolveTarget(ctx.ShutdownCtx, ctx.APIClient, opts)
			if err != nil {
				return err
			}

			existing, err := listExistingVariables(ctx.ShutdownCtx, ctx.APIClient, target)
			if err != nil {
				return err
			}

			duplicates := make([]string, 0)
			for _, variable := range imported {
				key := existingKey(variable.Key, variable.Category)
				if _, ok := existing[key]; ok && !opts.Overwrite {
					duplicates = append(duplicates, fmt.Sprintf("%s (%s)", variable.Key, variable.Category))
				}
			}
			if len(duplicates) > 0 {
				return fmt.Errorf("variables already exist; rerun with --overwrite to update: %s", strings.Join(duplicates, ", "))
			}

			created := 0
			updated := 0
			for _, variable := range imported {
				key := existingKey(variable.Key, variable.Category)
				if current, ok := existing[key]; ok {
					if err := updateVariable(ctx.ShutdownCtx, ctx.APIClient, target, current.ID, variable); err != nil {
						return err
					}
					updated++
					continue
				}
				if err := createVariable(ctx.ShutdownCtx, ctx.APIClient, target, variable); err != nil {
					return err
				}
				created++
			}

			fmt.Fprintf(ctx.IO.Err(), "%s imported %d variables into %s (%d created, %d updated)", opts.IO.ColorScheme().SuccessIcon(), len(imported), target.DisplayName, created, updated)
			return nil
		},
	}

	return cmd
}

type variableTarget struct {
	Kind        string
	ID          string
	DisplayName string
	Path        string
	ItemPath    string
}

type existingVariable struct {
	ID       string
	Key      string
	Category string
}

func resolveTarget(ctx context.Context, apiClient *client.Client, opts *ImportOpts) (*variableTarget, error) {
	if opts.VariableSetName != "" {
		id, err := resolveVariableSet(ctx, apiClient, opts)
		if err != nil {
			return nil, err
		}
		return &variableTarget{
			Kind:        "variable set",
			ID:          id,
			DisplayName: fmt.Sprintf("variable set %q", opts.VariableSetName),
			Path:        fmt.Sprintf("/varsets/%s/relationships/vars", url.PathEscape(id)),
			ItemPath:    fmt.Sprintf("/varsets/%s/relationships/vars/%%s", url.PathEscape(id)),
		}, nil
	}

	workspaceID, err := resolveWorkspace(ctx, apiClient, opts)
	if err != nil {
		return nil, err
	}
	return &variableTarget{
		Kind:        "workspace",
		ID:          workspaceID,
		DisplayName: fmt.Sprintf("workspace %q", opts.Workspace),
		Path:        fmt.Sprintf("/workspaces/%s/vars", url.PathEscape(workspaceID)),
		ItemPath:    fmt.Sprintf("/workspaces/%s/vars/%%s", url.PathEscape(workspaceID)),
	}, nil
}

func resolveWorkspace(ctx context.Context, apiClient *client.Client, opts *ImportOpts) (string, error) {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, fmt.Sprintf("/organizations/%s/workspaces/%s", url.PathEscape(opts.Organization), url.PathEscape(opts.Workspace)))
	if err != nil {
		return "", err
	}
	resp, err := apiClient.RawRequest(ctx, &client.Request{Method: http.MethodGet, URL: endpoint, Headers: jsonAPIHeaders()})
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", resp.Status, client.SummarizeAPIErrors(resp.Body))
	}
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return "", err
	}
	if payload.Data.ID == "" {
		return "", fmt.Errorf("workspace %q returned no id", opts.Workspace)
	}
	return payload.Data.ID, nil
}

func resolveVariableSet(ctx context.Context, apiClient *client.Client, opts *ImportOpts) (string, error) {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, fmt.Sprintf("/organizations/%s/varsets", url.PathEscape(opts.Organization)))
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("q", opts.VariableSetName)
	endpoint.RawQuery = query.Encode()

	resp, err := apiClient.RawRequest(ctx, &client.Request{Method: http.MethodGet, URL: endpoint, Headers: jsonAPIHeaders()})
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", resp.Status, client.SummarizeAPIErrors(resp.Body))
	}

	var payload struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return "", err
	}
	for _, item := range payload.Data {
		if item.Attributes.Name == opts.VariableSetName {
			return item.ID, nil
		}
	}

	body := map[string]any{
		"data": map[string]any{
			"type": "varsets",
			"attributes": map[string]any{
				"name": opts.VariableSetName,
			},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	createResp, err := apiClient.RawRequest(ctx, &client.Request{Method: http.MethodPost, URL: endpoint, Headers: jsonAPIHeaders(), Body: encoded})
	if err != nil {
		return "", err
	}
	if createResp.StatusCode < 200 || createResp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", createResp.Status, client.SummarizeAPIErrors(createResp.Body))
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createResp.Body, &created); err != nil {
		return "", err
	}
	if created.Data.ID == "" {
		return "", fmt.Errorf("created variable set %q returned no id", opts.VariableSetName)
	}
	return created.Data.ID, nil
}

func listExistingVariables(ctx context.Context, apiClient *client.Client, target *variableTarget) (map[string]existingVariable, error) {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, target.Path)
	if err != nil {
		return nil, err
	}
	resp, err := apiClient.RawRequest(ctx, &client.Request{Method: http.MethodGet, URL: endpoint, Headers: jsonAPIHeaders()})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s", resp.Status, client.SummarizeAPIErrors(resp.Body))
	}
	var payload struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Key      string `json:"key"`
				Category string `json:"category"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return nil, err
	}
	existing := make(map[string]existingVariable, len(payload.Data))
	for _, item := range payload.Data {
		existing[existingKey(item.Attributes.Key, item.Attributes.Category)] = existingVariable{ID: item.ID, Key: item.Attributes.Key, Category: item.Attributes.Category}
	}
	return existing, nil
}

func createVariable(ctx context.Context, apiClient *client.Client, target *variableTarget, variable terraformcfg.ImportedVariable) error {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, target.Path)
	if err != nil {
		return err
	}
	body, err := json.Marshal(variablePayload(variable))
	if err != nil {
		return err
	}
	resp, err := apiClient.RawRequest(ctx, &client.Request{Method: http.MethodPost, URL: endpoint, Headers: jsonAPIHeaders(), Body: body})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, client.SummarizeAPIErrors(resp.Body))
	}
	return nil
}

func updateVariable(ctx context.Context, apiClient *client.Client, target *variableTarget, variableID string, variable terraformcfg.ImportedVariable) error {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, fmt.Sprintf(target.ItemPath, url.PathEscape(variableID)))
	if err != nil {
		return err
	}
	body, err := json.Marshal(variablePayload(variable))
	if err != nil {
		return err
	}
	resp, err := apiClient.RawRequest(ctx, &client.Request{Method: http.MethodPatch, URL: endpoint, Headers: jsonAPIHeaders(), Body: body})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, client.SummarizeAPIErrors(resp.Body))
	}
	return nil
}

func variablePayload(variable terraformcfg.ImportedVariable) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"type": "vars",
			"attributes": map[string]any{
				"key":       variable.Key,
				"value":     variable.Value,
				"category":  variable.Category,
				"hcl":       variable.HCL,
				"sensitive": variable.Sensitive,
			},
		},
	}
}

func existingKey(key, category string) string {
	return category + "\x00" + key
}

func jsonAPIHeaders() http.Header {
	return http.Header{
		"Accept":       []string{"application/vnd.api+json"},
		"Content-Type": []string{"application/vnd.api+json"},
	}
}
