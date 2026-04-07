package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/brandonc/tfcloud/internal/client"
	"github.com/brandonc/tfcloud/internal/config"
	terraformcfg "github.com/brandonc/tfcloud/internal/terraform"
)

// VariableImportCommand imports variables into a workspace or variable set.
type VariableImportCommand struct {
	// Meta provides UI and stream access for command execution.
	Meta       *Meta
	loadConfig func() (*config.Config, error)
}

// Synopsis returns a short summary of the command.
func (c *VariableImportCommand) Synopsis() string {
	return "Import workspace or variable set variables"
}

// Help returns the command help text.
func (c *VariableImportCommand) Help() string {
	return strings.TrimSpace(`Usage: tfcloud variable import [tfvars-file] [flags]

Import variables into the current workspace or a variable set.

  -e name                   Import an environment variable (repeatable)
  -variable-set-name name   Target variable set by name
  -organization string      Organization name
  -workspace string         Workspace name override
  -overwrite                Update matching existing variables`)
}

// Run executes the variable import command.
func (c *VariableImportCommand) Run(args []string) int {
	var envNames multiFlag
	var variableSetName string
	var organization string
	var workspaceName string
	var overwrite bool

	fs := flag.NewFlagSet("variable import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(&envNames, "e", "env")
	fs.StringVar(&variableSetName, "variable-set-name", "", "variable set")
	fs.StringVar(&organization, "organization", "", "organization")
	fs.StringVar(&workspaceName, "workspace", "", "workspace")
	fs.BoolVar(&overwrite, "overwrite", false, "overwrite")

	if err := fs.Parse(args); err != nil {
		c.Meta.UI.Error(err.Error())
		return 1
	}

	var imported []terraformcfg.ImportedVariable
	if fs.NArg() > 1 {
		c.Meta.UI.Error("usage: tfcloud variable import [tfvars-file]")
		return 1
	}
	if fs.NArg() == 1 {
		vars, err := terraformcfg.ParseTFVarsFile(fs.Arg(0))
		if err != nil {
			c.Meta.UI.Error(err.Error())
			return 1
		}
		imported = append(imported, vars...)
	}
	for _, name := range envNames {
		value, ok := os.LookupEnv(name)
		if !ok {
			c.Meta.UI.Error(fmt.Sprintf("environment variable %q is not set", name))
			return 1
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
		c.Meta.UI.Error("provide a tfvars file, -e entries, or both")
		return 1
	}

	loadConfig := c.loadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}

	cfg, err := loadConfig()
	if err != nil {
		c.Meta.UI.Error(err.Error())
		return 1
	}

	if organization == "" {
		organization = cfg.DefaultOrganization
	}

	if organization == "" || workspaceName == "" {
		cfg, err := terraformcfg.FindCloudConfig(".")
		if err == nil {
			if organization == "" {
				organization = cfg.Organization
			}
			if workspaceName == "" {
				workspaceName = cfg.Workspace
			}
		}
	}

	if variableSetName != "" && organization == "" {
		c.Meta.UI.Error("-organization is required when targeting a variable set and no HCP Terraform configuration was found")
		return 1
	}
	if variableSetName == "" && (organization == "" || workspaceName == "") {
		c.Meta.UI.Error("could not resolve target workspace; set -organization and -workspace or run inside a repository with HCP Terraform workspace configuration")
		return 1
	}

	apiClient, err := client.New(cfg)
	if err != nil {
		c.Meta.UI.Error(err.Error())
		return 1
	}

	target, err := c.resolveTarget(apiClient, organization, workspaceName, variableSetName)
	if err != nil {
		c.Meta.UI.Error(err.Error())
		return 1
	}

	existing, err := c.listExistingVariables(apiClient, target)
	if err != nil {
		c.Meta.UI.Error(err.Error())
		return 1
	}

	duplicates := make([]string, 0)
	for _, variable := range imported {
		key := existingKey(variable.Key, variable.Category)
		if _, ok := existing[key]; ok && !overwrite {
			duplicates = append(duplicates, fmt.Sprintf("%s (%s)", variable.Key, variable.Category))
		}
	}
	if len(duplicates) > 0 {
		c.Meta.UI.Error("variables already exist; rerun with -overwrite to update: " + strings.Join(duplicates, ", "))
		return 1
	}

	created := 0
	updated := 0
	for _, variable := range imported {
		key := existingKey(variable.Key, variable.Category)
		if current, ok := existing[key]; ok {
			if err := c.updateVariable(apiClient, target, current.ID, variable); err != nil {
				c.Meta.UI.Error(err.Error())
				return 1
			}
			updated++
			continue
		}
		if err := c.createVariable(apiClient, target, variable); err != nil {
			c.Meta.UI.Error(err.Error())
			return 1
		}
		created++
	}

	c.Meta.UI.Output(fmt.Sprintf("imported %d variables into %s (%d created, %d updated)", len(imported), target.DisplayName, created, updated))
	return 0
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

func (c *VariableImportCommand) resolveTarget(apiClient *client.Client, organization, workspaceName, variableSetName string) (*variableTarget, error) {
	if variableSetName != "" {
		id, err := c.resolveVariableSet(apiClient, organization, variableSetName)
		if err != nil {
			return nil, err
		}
		return &variableTarget{
			Kind:        "variable set",
			ID:          id,
			DisplayName: fmt.Sprintf("variable set %q", variableSetName),
			Path:        fmt.Sprintf("/varsets/%s/relationships/vars", url.PathEscape(id)),
			ItemPath:    fmt.Sprintf("/varsets/%s/relationships/vars/%%s", url.PathEscape(id)),
		}, nil
	}

	workspaceID, err := c.resolveWorkspace(apiClient, organization, workspaceName)
	if err != nil {
		return nil, err
	}
	return &variableTarget{
		Kind:        "workspace",
		ID:          workspaceID,
		DisplayName: fmt.Sprintf("workspace %q", workspaceName),
		Path:        fmt.Sprintf("/workspaces/%s/vars", url.PathEscape(workspaceID)),
		ItemPath:    fmt.Sprintf("/workspaces/%s/vars/%%s", url.PathEscape(workspaceID)),
	}, nil
}

func (c *VariableImportCommand) resolveWorkspace(apiClient *client.Client, organization, workspace string) (string, error) {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, fmt.Sprintf("/organizations/%s/workspaces/%s", url.PathEscape(organization), url.PathEscape(workspace)))
	if err != nil {
		return "", err
	}
	resp, err := apiClient.RawRequest(c.background(), &client.Request{Method: http.MethodGet, URL: endpoint, Headers: jsonAPIHeaders()})
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", resp.Status, summarizeAPIErrors(resp.Body))
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
		return "", fmt.Errorf("workspace %q returned no id", workspace)
	}
	return payload.Data.ID, nil
}

func (c *VariableImportCommand) resolveVariableSet(apiClient *client.Client, organization, name string) (string, error) {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, fmt.Sprintf("/organizations/%s/varsets", url.PathEscape(organization)))
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("q", name)
	endpoint.RawQuery = query.Encode()

	resp, err := apiClient.RawRequest(c.background(), &client.Request{Method: http.MethodGet, URL: endpoint, Headers: jsonAPIHeaders()})
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", resp.Status, summarizeAPIErrors(resp.Body))
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
		if item.Attributes.Name == name {
			return item.ID, nil
		}
	}

	body := map[string]any{
		"data": map[string]any{
			"type": "varsets",
			"attributes": map[string]any{
				"name": name,
			},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	createResp, err := apiClient.RawRequest(c.background(), &client.Request{Method: http.MethodPost, URL: endpoint, Headers: jsonAPIHeaders(), Body: encoded})
	if err != nil {
		return "", err
	}
	if createResp.StatusCode < 200 || createResp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", createResp.Status, summarizeAPIErrors(createResp.Body))
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
		return "", fmt.Errorf("created variable set %q returned no id", name)
	}
	return created.Data.ID, nil
}

func (c *VariableImportCommand) listExistingVariables(apiClient *client.Client, target *variableTarget) (map[string]existingVariable, error) {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, target.Path)
	if err != nil {
		return nil, err
	}
	resp, err := apiClient.RawRequest(c.background(), &client.Request{Method: http.MethodGet, URL: endpoint, Headers: jsonAPIHeaders()})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s", resp.Status, summarizeAPIErrors(resp.Body))
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

func (c *VariableImportCommand) createVariable(apiClient *client.Client, target *variableTarget, variable terraformcfg.ImportedVariable) error {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, target.Path)
	if err != nil {
		return err
	}
	body, err := json.Marshal(variablePayload(variable))
	if err != nil {
		return err
	}
	resp, err := apiClient.RawRequest(c.background(), &client.Request{Method: http.MethodPost, URL: endpoint, Headers: jsonAPIHeaders(), Body: body})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, summarizeAPIErrors(resp.Body))
	}
	return nil
}

func (c *VariableImportCommand) updateVariable(apiClient *client.Client, target *variableTarget, variableID string, variable terraformcfg.ImportedVariable) error {
	endpoint, err := client.ResolveURL(apiClient.BaseURL, fmt.Sprintf(target.ItemPath, url.PathEscape(variableID)))
	if err != nil {
		return err
	}
	body, err := json.Marshal(variablePayload(variable))
	if err != nil {
		return err
	}
	resp, err := apiClient.RawRequest(c.background(), &client.Request{Method: http.MethodPatch, URL: endpoint, Headers: jsonAPIHeaders(), Body: body})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, summarizeAPIErrors(resp.Body))
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

func (c *VariableImportCommand) background() context.Context { return context.Background() }
