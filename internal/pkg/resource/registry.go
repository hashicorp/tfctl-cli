// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

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
		Resolvable: true,
	},
	{
		Type:     "runs",
		Aliases:  []string{"run"},
		IDPrefix: "run-",
		PathGet:  "/runs/{id}",
	},
	{
		Type:       "projects",
		Aliases:    []string{"prj", "project"},
		IDPrefix:   "prj-",
		PathGet:    "/projects/{id}",
		PathList:   "/organizations/{organization_name}/projects",
		PathCreate: "/organizations/{organization_name}/projects",
		Resolvable: true,
	},
	{
		Type:       "teams",
		Aliases:    []string{"team"},
		IDPrefix:   "team-",
		PathGet:    "/teams/{id}",
		PathList:   "/organizations/{organization_name}/teams",
		PathCreate: "/organizations/{organization_name}/teams",
		Resolvable: true,
	},
	{
		Type:       "varsets",
		Aliases:    []string{"varset", "variable-sets", "variable-set"},
		IDPrefix:   "varset-",
		PathGet:    "/varsets/{id}",
		PathList:   "/organizations/{organization_name}/varsets",
		PathCreate: "/organizations/{organization_name}/varsets",
		Resolvable: true,
	},
	{
		Type:     "organizations",
		Aliases:  []string{"org", "orgs", "organization"},
		PathGet:  "/organizations/{id}",
		PathList: "/organizations",
	},
	{
		Type:     "plans",
		Aliases:  []string{"plan"},
		IDPrefix: "plan-",
		PathGet:  "/plans/{id}",
	},
	{
		Type:     "applies",
		Aliases:  []string{"apply"},
		IDPrefix: "apply-",
		PathGet:  "/applies/{id}",
	},
	{
		Type:     "state-versions",
		Aliases:  []string{"sv", "state-version"},
		IDPrefix: "sv-",
		PathGet:  "/state-versions/{id}",
	},
	{
		Type:       "policy-sets",
		Aliases:    []string{"policy-set", "polset"},
		IDPrefix:   "polset-",
		PathGet:    "/policy-sets/{id}",
		PathList:   "/organizations/{organization_name}/policy-sets",
		PathCreate: "/organizations/{organization_name}/policy-sets",
	},
	{
		Type:     "vars",
		Aliases:  []string{"var", "variable", "variables"},
		IDPrefix: "var-",
		PathGet:  "/vars/{id}",
	},
	{
		Type:       "agent-pools",
		Aliases:    []string{"apool", "agent-pool"},
		IDPrefix:   "apool-",
		PathGet:    "/agent-pools/{id}",
		PathList:   "/organizations/{organization_name}/agent-pools",
		PathCreate: "/organizations/{organization_name}/agent-pools",
	},
	{
		Type:     "configuration-versions",
		Aliases:  []string{"cv", "config-version", "configuration-version"},
		IDPrefix: "cv-",
		PathGet:  "/configuration-versions/{id}",
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

// CompletionNames returns all type names and aliases (for shell autocompletion).
func CompletionNames() []string {
	var names []string
	for _, r := range registry {
		names = append(names, r.Type)
		names = append(names, r.Aliases...)
	}
	sort.Strings(names)
	return names
}

// CreatableNames returns names and aliases of resource types that support creation.
func CreatableNames() []string {
	var names []string
	for _, r := range registry {
		if r.PathCreate == "" {
			continue
		}
		names = append(names, r.Type)
		names = append(names, r.Aliases...)
	}
	sort.Strings(names)
	return names
}

// IsResolvableType returns true if the given type name (e.g. "workspaces")
// supports name-to-ID resolution via the API.
func IsResolvableType(typeName string) bool {
	for i := range registry {
		if registry[i].Type == typeName && registry[i].Resolvable {
			return true
		}
	}
	return false
}

// IDPrefixForType returns the ID prefix for the given type name, or "" if unknown.
func IDPrefixForType(typeName string) string {
	for i := range registry {
		if registry[i].Type == typeName {
			return registry[i].IDPrefix
		}
	}
	return ""
}
