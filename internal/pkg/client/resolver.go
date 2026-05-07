// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-tfe/api/organizations"
	"github.com/hashicorp/go-tfe/api/workspaces"
	abstractions "github.com/microsoft/kiota-abstractions-go"
)

// Resolver abstracts helpers for resolving resources by name, with options for creating the
// the resource if it does not exist.
type Resolver struct {
	client           *Client
	createIfNotFound bool
	dryRun           bool
}

// NewResolver creates a new Resolver.
func NewResolver(client *Client, createIfNotFound, dryRun bool) *Resolver {
	return &Resolver{
		client:           client,
		createIfNotFound: createIfNotFound,
		dryRun:           dryRun,
	}
}

// VariableSet resolves a variable set by organization + name.
func (r Resolver) VariableSet(ctx context.Context, organization, name string) (*string, error) {
	requestConfig := &abstractions.RequestConfiguration[organizations.ItemVarsetsRequestBuilderGetQueryParameters]{
		QueryParameters: &organizations.ItemVarsetsRequestBuilderGetQueryParameters{
			Q: &name,
		},
	}

	items, err := r.client.TFE.API.Organizations().ByOrganization_name(organization).Varsets().Get(ctx, requestConfig)
	if err != nil {
		return nil, err
	}

	for _, item := range items.GetData() {
		att := item.GetAttributes()
		if *att.GetName() == name {
			return item.GetId(), nil
		}
	}

	if r.createIfNotFound {
		// Create the Variable Set
		created, err := r.client.TFE.API.Organizations().ByOrganization_name(organization).Varsets().Post(ctx, NewVarset(name), nil)
		if err != nil {
			return nil, fmt.Errorf("variable set named %q could not be created: %w", name, err)
		}

		return created.GetData().GetId(), nil
	}

	return nil, fmt.Errorf("variable set named %q was not found", name)
}

// RunOrCurrentRun resolves a run ID. If resourceType is "runs", id is returned
// directly. If resourceType is "workspaces", id is treated as a workspace ID
// or name and the latest run is returned.
func (r Resolver) RunOrCurrentRun(ctx context.Context, organization, resourceType, id string) (string, error) {
	switch resourceType {
	case "runs":
		return id, nil
	case "workspaces":
		return r.latestRunForWorkspace(ctx, organization, id)
	default:
		return "", fmt.Errorf("unsupported resource type %q", resourceType)
	}
}

func (r Resolver) latestRunForWorkspace(ctx context.Context, organization, id string) (string, error) {
	wsID := id
	if organization != "" {
		ws, err := r.client.TFE.API.Organizations().ByOrganization_name(organization).Workspaces().ByWorkspace_name(id).Get(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("resolving workspace %q: %w", id, err)
		}
		resolved := ws.GetData().GetId()
		if resolved == nil {
			return "", fmt.Errorf("workspace %q has no ID", id)
		}
		wsID = *resolved
	}

	pageSize := int32(1)
	config := &abstractions.RequestConfiguration[workspaces.ItemRunsRequestBuilderGetQueryParameters]{
		QueryParameters: &workspaces.ItemRunsRequestBuilderGetQueryParameters{
			Pagesize: &pageSize,
		},
	}

	runs, err := r.client.TFE.API.Workspaces().ByWorkspace_id(wsID).Runs().Get(ctx, config)
	if err != nil {
		return "", fmt.Errorf("fetching runs for workspace %s: %w", wsID, err)
	}

	data := runs.GetData()
	if len(data) == 0 {
		return "", fmt.Errorf("no runs found for workspace %s", wsID)
	}

	runID := data[0].GetId()
	if runID == nil {
		return "", fmt.Errorf("latest run has no ID")
	}
	return *runID, nil
}
