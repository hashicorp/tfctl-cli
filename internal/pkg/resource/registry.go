// Copyright IBM Corp. 2026

package resource

import (
	"sort"
	"strings"
)

var registry = []Resource{
	{
		Type:       "workspaces",
		Aliases:    []string{"ws", "workspace"},
		IDPrefix:   "ws-",
		PathGet:    "/workspaces/{id}",
		PathList:   "/organizations/{organization_name}/workspaces",
		PathCreate: "/organizations/{organization_name}/workspaces",
		Columns:    []string{"id", "name", "terraform-version", "updated-at"},
	},
	{
		Type:       "runs",
		Aliases:    []string{"run"},
		IDPrefix:   "run-",
		PathGet:    "/runs/{id}",
		PathList:   "",
		PathCreate: "",
		Columns:    []string{"id", "status", "created-at"},
	},
	{
		Type:       "projects",
		Aliases:    []string{"prj", "project"},
		IDPrefix:   "prj-",
		PathGet:    "/projects/{id}",
		PathList:   "/organizations/{organization_name}/projects",
		PathCreate: "/organizations/{organization_name}/projects",
		Columns:    []string{"id", "name"},
	},
	{
		Type:       "teams",
		Aliases:    []string{"team"},
		IDPrefix:   "team-",
		PathGet:    "/teams/{id}",
		PathList:   "/organizations/{organization_name}/teams",
		PathCreate: "/organizations/{organization_name}/teams",
		Columns:    []string{"id", "name"},
	},
	{
		Type:       "varsets",
		Aliases:    []string{"varset", "variable-sets", "variable-set"},
		IDPrefix:   "varset-",
		PathGet:    "/varsets/{id}",
		PathList:   "/organizations/{organization_name}/varsets",
		PathCreate: "/organizations/{organization_name}/varsets",
		Columns:    []string{"id", "name"},
	},
	{
		Type:       "organizations",
		Aliases:    []string{"org", "orgs", "organization"},
		IDPrefix:   "",
		PathGet:    "/organizations/{id}",
		PathList:   "/organizations",
		PathCreate: "",
		Columns:    []string{"id", "name", "email"},
	},
	{
		Type:       "plans",
		Aliases:    []string{"plan"},
		IDPrefix:   "plan-",
		PathGet:    "/plans/{id}",
		PathList:   "",
		PathCreate: "",
		Columns:    []string{"id", "status"},
	},
	{
		Type:       "applies",
		Aliases:    []string{"apply"},
		IDPrefix:   "apply-",
		PathGet:    "/applies/{id}",
		PathList:   "",
		PathCreate: "",
		Columns:    []string{"id", "status"},
	},
	{
		Type:       "state-versions",
		Aliases:    []string{"sv", "state-version"},
		IDPrefix:   "sv-",
		PathGet:    "/state-versions/{id}",
		PathList:   "",
		PathCreate: "",
		Columns:    []string{"id", "serial", "created-at"},
	},
	{
		Type:       "policy-sets",
		Aliases:    []string{"policy-set", "polset"},
		IDPrefix:   "polset-",
		PathGet:    "/policy-sets/{id}",
		PathList:   "/organizations/{organization_name}/policy-sets",
		PathCreate: "/organizations/{organization_name}/policy-sets",
		Columns:    []string{"id", "name"},
	},
	{
		Type:       "vars",
		Aliases:    []string{"var", "variable", "variables"},
		IDPrefix:   "var-",
		PathGet:    "/vars/{id}",
		PathList:   "",
		PathCreate: "",
		Columns:    []string{"id", "key", "value", "category"},
	},
	{
		Type:       "agent-pools",
		Aliases:    []string{"agent-pool"},
		IDPrefix:   "apool-",
		PathGet:    "/agent-pools/{id}",
		PathList:   "/organizations/{organization_name}/agent-pools",
		PathCreate: "/organizations/{organization_name}/agent-pools",
		Columns:    []string{"id", "name"},
	},
	{
		Type:       "configuration-versions",
		Aliases:    []string{"cv", "config-version", "configuration-version"},
		IDPrefix:   "cv-",
		PathGet:    "/configuration-versions/{id}",
		PathList:   "",
		PathCreate: "",
		Columns:    []string{"id", "status", "created-at"},
	},
}

// ByName matches the canonical Type or any Alias, case-insensitive.
// Returns nil if not found.
func ByName(name string) *Resource {
	lower := strings.ToLower(name)
	for i := range registry {
		if strings.ToLower(registry[i].Type) == lower {
			return &registry[i]
		}
		for _, alias := range registry[i].Aliases {
			if strings.ToLower(alias) == lower {
				return &registry[i]
			}
		}
	}
	return nil
}

// ByIDPrefix scans the registry for a resource whose IDPrefix matches the
// beginning of value. Returns nil if no match is found or if value is empty.
func ByIDPrefix(value string) *Resource {
	if value == "" {
		return nil
	}
	for i := range registry {
		if registry[i].IDPrefix == "" {
			continue
		}
		if strings.HasPrefix(value, registry[i].IDPrefix) {
			return &registry[i]
		}
	}
	return nil
}

// All returns all registered resources.
func All() []Resource {
	out := make([]Resource, len(registry))
	copy(out, registry)
	return out
}

// Names returns sorted canonical type names (for error messages).
func Names() []string {
	names := make([]string, len(registry))
	for i, r := range registry {
		names[i] = r.Type
	}
	sort.Strings(names)
	return names
}
