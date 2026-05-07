// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"errors"
	"fmt"
	"strings"

	abstractions "github.com/microsoft/kiota-abstractions-go"

	"github.com/hashicorp/go-tfe/api/workspaces"
)

// resolveRunID resolves the given identifier to a run ID.
func resolveRunID(opts *StatusOpts, id string) (string, error) {
	switch {
	case strings.HasPrefix(id, "run-"):
		return id, nil

	case strings.HasPrefix(id, "ws-"):
		return getLatestRunForWorkspace(opts, id)

	default:
		if opts.Organization == "" {
			return "", errors.New("--organization is required when specifying a workspace name")
		}
		ws, err := opts.Client.TFE.API.Organizations().ByOrganization_name(opts.Organization).Workspaces().ByWorkspace_name(id).Get(opts.ShutdownCtx, nil)
		if err != nil {
			return "", fmt.Errorf("resolving workspace %q: %w", id, err)
		}
		wsID := ws.GetData().GetId()
		if wsID == nil {
			return "", fmt.Errorf("workspace %q has no ID", id)
		}
		return getLatestRunForWorkspace(opts, *wsID)
	}
}

// getLatestRunForWorkspace fetches the most recent run for the given workspace ID.
func getLatestRunForWorkspace(opts *StatusOpts, wsID string) (string, error) {
	pageSize := int32(1)
	config := &abstractions.RequestConfiguration[workspaces.ItemRunsRequestBuilderGetQueryParameters]{
		QueryParameters: &workspaces.ItemRunsRequestBuilderGetQueryParameters{
			Pagesize: &pageSize,
		},
	}

	runs, err := opts.Client.TFE.API.Workspaces().ByWorkspace_id(wsID).Runs().Get(opts.ShutdownCtx, config)
	if err != nil {
		return "", fmt.Errorf("fetching runs for workspace %s: %w", wsID, err)
	}

	data := runs.GetData()
	if len(data) == 0 {
		return "", fmt.Errorf("no runs found for workspace %s", wsID)
	}

	runID := data[0].GetId()
	if runID == nil {
		return "", errors.New("latest run has no ID")
	}
	return *runID, nil
}
