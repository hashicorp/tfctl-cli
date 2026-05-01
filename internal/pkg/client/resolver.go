package client

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-tfe/api/organizations"
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

// Workspace resolves a workspace by organization + name to its internal ID.
func (r Resolver) Workspace(ctx context.Context, organization, name string) (*string, error) {
	workspace, err := r.client.TFE.API.Organizations().ByOrganization_name(organization).Workspaces().ByWorkspace_name(name).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("workspace named %q was not found in organization %q: %w", name, organization, err)
	}

	return workspace.GetData().GetId(), nil
}

// Team resolves a team by organization + name to its internal ID.
func (r Resolver) Team(ctx context.Context, organization, name string) (*string, error) {
	requestConfig := &abstractions.RequestConfiguration[organizations.ItemTeamsRequestBuilderGetQueryParameters]{
		QueryParameters: &organizations.ItemTeamsRequestBuilderGetQueryParameters{
			Filternames: &name,
		},
	}

	items, err := r.client.TFE.API.Organizations().ByOrganization_name(organization).Teams().Get(ctx, requestConfig)
	if err != nil {
		return nil, err
	}

	for _, item := range items.GetData() {
		att := item.GetAttributes()
		if att.GetName() != nil && *att.GetName() == name {
			return item.GetId(), nil
		}
	}

	return nil, fmt.Errorf("team named %q was not found in organization %q", name, organization)
}

// Project resolves a project by organization + name to its internal ID.
func (r Resolver) Project(ctx context.Context, organization, name string) (*string, error) {
	requestConfig := &abstractions.RequestConfiguration[organizations.ItemProjectsRequestBuilderGetQueryParameters]{
		QueryParameters: &organizations.ItemProjectsRequestBuilderGetQueryParameters{
			Filternames: &name,
		},
	}

	items, err := r.client.TFE.API.Organizations().ByOrganization_name(organization).Projects().Get(ctx, requestConfig)
	if err != nil {
		return nil, err
	}

	for _, item := range items.GetData() {
		att := item.GetAttributes()
		if att.GetName() != nil && *att.GetName() == name {
			return item.GetId(), nil
		}
	}

	return nil, fmt.Errorf("project named %q was not found in organization %q", name, organization)
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
