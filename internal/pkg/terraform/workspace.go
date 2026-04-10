// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package terraform reads local Terraform configuration and tfvars inputs.
package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// CloudConfig describes the organization and workspace discovered from Terraform configuration.
type CloudConfig struct {
	// Organization is the configured HCP Terraform organization name.
	Organization string
	// Workspace is the configured HCP Terraform workspace name.
	Workspace string
}

// ImportedVariable describes a variable ready to send to the HCP Terraform API.
type ImportedVariable struct {
	// Key is the variable name.
	Key string
	// Value is the serialized variable value.
	Value string
	// Category is the HCP Terraform variable category, such as terraform or env.
	Category string
	// HCL reports whether Value should be interpreted as HCL.
	HCL bool
	// Sensitive reports whether the variable should be marked sensitive.
	Sensitive bool
}

// FindCloudConfig scans the given directory for Terraform cloud or remote workspace configuration.
func FindCloudConfig(root string) (*CloudConfig, error) {
	parser := hclparse.NewParser()
	var result *CloudConfig

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() != root {
			return filepath.SkipDir
		}
		if filepath.Ext(path) != ".tf" {
			return nil
		}

		file, diags := parser.ParseHCLFile(path)
		if diags.HasErrors() {
			return nil
		}

		cfg := extractCloudConfig(file.Body)
		if cfg != nil {
			result = cfg
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("no terraform cloud/remote workspace configuration found")
	}
	return result, nil
}

// ParseTFVarsFile parses an HCL or JSON tfvars file into importable variables.
func ParseTFVarsFile(path string) ([]ImportedVariable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(path, ".json") {
		return parseTFVarsJSON(data)
	}

	file, diags := hclsyntax.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	attrs, diags := file.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}

	vars := make([]ImportedVariable, 0, len(attrs))
	for name, attr := range attrs {
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluate %s: %s", name, diags.Error())
		}
		imported, err := importedVariableFromCTY(name, value)
		if err != nil {
			return nil, err
		}
		vars = append(vars, imported)
	}
	return vars, nil
}

func extractCloudConfig(body hcl.Body) *CloudConfig {
	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}},
	})
	if diags.HasErrors() {
		return nil
	}

	for _, block := range content.Blocks {
		terraformBody, diags := block.Body.Content(&hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "cloud"},
				{Type: "backend", LabelNames: []string{"type"}},
			},
		})
		if diags.HasErrors() {
			continue
		}

		for _, nested := range terraformBody.Blocks {
			switch nested.Type {
			case "cloud":
				if cfg := extractWorkspaceBlock(nested.Body, "organization"); cfg != nil {
					return cfg
				}
			case "backend":
				if len(nested.Labels) == 1 && nested.Labels[0] == "remote" {
					if cfg := extractWorkspaceBlock(nested.Body, "organization"); cfg != nil {
						return cfg
					}
				}
			}
		}
	}

	return nil
}

func extractWorkspaceBlock(body hcl.Body, organizationAttr string) *CloudConfig {
	content, diags := body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: organizationAttr}},
		Blocks:     []hcl.BlockHeaderSchema{{Type: "workspaces"}},
	})
	if diags.HasErrors() {
		return nil
	}

	org := attrString(content.Attributes[organizationAttr])
	if org == "" {
		return nil
	}

	for _, block := range content.Blocks {
		workspaceBody, diags := block.Body.Content(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{{Name: "name"}},
		})
		if diags.HasErrors() {
			continue
		}
		name := attrString(workspaceBody.Attributes["name"])
		if name != "" {
			return &CloudConfig{Organization: org, Workspace: name}
		}
	}

	return nil
}

func attrString(attr *hcl.Attribute) string {
	if attr == nil {
		return ""
	}
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || value.IsNull() || value.Type() != cty.String {
		return ""
	}
	return value.AsString()
}

func parseTFVarsJSON(data []byte) ([]ImportedVariable, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	vars := make([]ImportedVariable, 0, len(payload))
	for key, value := range payload {
		imported, err := importedVariableFromJSON(key, value)
		if err != nil {
			return nil, err
		}
		vars = append(vars, imported)
	}
	return vars, nil
}

func importedVariableFromCTY(key string, value cty.Value) (ImportedVariable, error) {
	if !value.IsKnown() {
		return ImportedVariable{}, fmt.Errorf("variable %s is unknown", key)
	}

	if !value.IsNull() && value.Type() == cty.String {
		return ImportedVariable{
			Key:       key,
			Value:     value.AsString(),
			Category:  "terraform",
			HCL:       false,
			Sensitive: looksSensitive(key),
		}, nil
	}

	encoded, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		return ImportedVariable{}, err
	}

	return ImportedVariable{
		Key:       key,
		Value:     string(encoded),
		Category:  "terraform",
		HCL:       true,
		Sensitive: looksSensitive(key),
	}, nil
}

func importedVariableFromJSON(key string, value any) (ImportedVariable, error) {
	if str, ok := value.(string); ok {
		return ImportedVariable{
			Key:       key,
			Value:     str,
			Category:  "terraform",
			HCL:       false,
			Sensitive: looksSensitive(key),
		}, nil
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return ImportedVariable{}, err
	}
	return ImportedVariable{
		Key:       key,
		Value:     string(encoded),
		Category:  "terraform",
		HCL:       true,
		Sensitive: looksSensitive(key),
	}, nil
}

func looksSensitive(name string) bool {
	key := strings.ToLower(name)
	for _, needle := range []string{"secret", "token", "password", "passwd", "credential", "private", "access_key", "secret_key", "api_key"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}
