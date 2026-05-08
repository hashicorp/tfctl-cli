// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package variable

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-tfe/api"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
)

type variableTargetKind string

type variableTarget struct {
	apiClient *api.ApiClient
	Kind      variableTargetKind
	ID        string
	Name      string
}

type existingVariable struct {
	ID       string
	Key      string
	Category string
}

var (
	kindWorkspace   variableTargetKind = "workspace"
	kindVariableSet variableTargetKind = "variable set"
)

func newVariableSetVariableTarget(apiClient *client.Client, id, name string) *variableTarget {
	return &variableTarget{
		apiClient: apiClient.TFE.API,
		Kind:      kindVariableSet,
		ID:        id,
		Name:      name,
	}
}

func newWorkspaceVariableTarget(apiClient *client.Client, id, name string) *variableTarget {
	return &variableTarget{
		apiClient: apiClient.TFE.API,
		Kind:      kindWorkspace,
		ID:        id,
		Name:      name,
	}
}

func (t *variableTarget) String() string {
	return fmt.Sprintf("%s %q", t.Kind, t.Name)
}

func (t *variableTarget) listExistingVariables(ctx context.Context) (existingVariables, error) {
	existing := make(existingVariables)
	switch t.Kind {
	case kindWorkspace:
		vars, err := t.apiClient.Workspaces().ByWorkspace_id(t.ID).Vars().Get(ctx, nil)
		if err != nil {
			return nil, err
		}

		for _, item := range vars.GetData() {
			existing.Add(existingVariable{
				ID:       *item.GetId(),
				Key:      *item.GetAttributes().GetKey(),
				Category: item.GetAttributes().GetCategory().String(),
			})
		}
	case kindVariableSet:
		vars, err := t.apiClient.Varsets().ByVarset_id(t.ID).Relationships().Vars().Get(ctx, nil)
		if err != nil {
			return nil, err
		}

		for _, item := range vars.GetData() {
			existing.Add(existingVariable{
				ID:       *item.GetId(),
				Key:      *item.GetAttributes().GetKey(),
				Category: item.GetAttributes().GetCategory().String(),
			})
		}
	}

	return existing, nil
}

func (t *variableTarget) createVariable(ctx context.Context, variable terraformcfg.ImportedVariable) error {
	var err error
	switch t.Kind {
	case kindVariableSet:
		_, err = t.apiClient.Varsets().ByVarset_id(t.ID).Relationships().Vars().Post(ctx, client.NewVar(variable.Key, variable.Value, variable.Category, variable.Sensitive), nil)
	case kindWorkspace:
		_, err = t.apiClient.Workspaces().ByWorkspace_id(t.ID).Vars().Post(ctx, client.NewVar(variable.Key, variable.Value, variable.Category, variable.Sensitive), nil)
	default:
		return fmt.Errorf("unknown variable target kind %s, this is a bug in %s", t.Kind, config.Name)
	}

	return err
}

func (t *variableTarget) updateVariable(ctx context.Context, variableID string, variable terraformcfg.ImportedVariable) error {
	var err error
	switch t.Kind {
	case kindVariableSet:
		_, err = t.apiClient.Varsets().ByVarset_id(t.ID).Relationships().Vars().ById(variableID).Patch(ctx, client.NewVar(variable.Key, variable.Value, variable.Category, variable.Sensitive), nil)
	case kindWorkspace:
		_, err = t.apiClient.Workspaces().ByWorkspace_id(t.ID).Vars().ById(variableID).Patch(ctx, client.NewVar(variable.Key, variable.Value, variable.Category, variable.Sensitive), nil)
	default:
		return fmt.Errorf("unknown variable target kind %s, this is a bug in %s", t.Kind, config.Name)
	}

	return err
}
