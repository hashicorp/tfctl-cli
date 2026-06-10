// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"sort"
	"strings"
)

var registry = []Resource{
	{
		Type:           "workspaces",
		Aliases:        []string{"ws", "workspace"},
		IDPrefix:       "ws-",
		PathGet:        "/workspaces/{id}",
		PathList:       "/organizations/{organization_name}/workspaces",
		PathCreate:     "/organizations/{organization_name}/workspaces",
		Resolvable:     true,
		Columns:        []string{"name", "description", "project", "execution-mode", "locked", "resource-count"},
		ExcludeColumns: []string{"actions"},
	},
	{
		Type:     "runs",
		Aliases:  []string{"run"},
		IDPrefix: "run-",
		PathGet:  "/runs/{id}",
		Columns:  []string{"message", "status", "is-destroy", "has-changes"},
	},
	{
		Type:       "projects",
		Aliases:    []string{"prj", "project"},
		IDPrefix:   "prj-",
		PathGet:    "/projects/{id}",
		PathList:   "/organizations/{organization_name}/projects",
		PathCreate: "/organizations/{organization_name}/projects",
		Resolvable: true,
		Columns:    []string{"name", "description", "organization-name"},
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
		Columns:    []string{"name", "description", "global", "priority"},
	},
	{
		Type:           "organizations",
		Aliases:        []string{"org", "orgs", "organization"},
		PathGet:        "/organizations/{id}",
		PathList:       "/organizations",
		Columns:        []string{"name", "email", "external-id", "access-beta-tools", "stacks-enabled"},
		ExcludeColumns: []string{"id"},
	},
	{
		Type:     "plans",
		Aliases:  []string{"plan"},
		IDPrefix: "plan-",
		PathGet:  "/plans/{id}",
		Columns:  []string{"status", "has-changes", "generated-configuration"},
	},
	{
		Type:     "applies",
		Aliases:  []string{"apply"},
		IDPrefix: "apply-",
		PathGet:  "/applies/{id}",
		Columns:  []string{"status", "status-timestamps", "log-read-url"},
	},
	{
		Type:     "state-versions",
		Aliases:  []string{"sv", "state-version"},
		IDPrefix: "sv-",
		PathGet:  "/state-versions/{id}",
		Columns:  []string{"serial", "status", "resource-count", "size"},
	},
	{
		Type:       "policy-sets",
		Aliases:    []string{"policy-set", "polset"},
		IDPrefix:   "polset-",
		PathGet:    "/policy-sets/{id}",
		PathList:   "/organizations/{organization_name}/policy-sets",
		PathCreate: "/organizations/{organization_name}/policy-sets",
		Columns:    []string{"name", "kind", "global", "overridable"},
	},
	{
		Type:     "vars",
		Aliases:  []string{"var", "variable", "variables"},
		IDPrefix: "var-",
		PathGet:  "/vars/{id}",
		Columns:  []string{"key", "value", "category", "hcl", "sensitive"},
	},
	{
		Type:       "agent-pools",
		Aliases:    []string{"agent-pool"},
		IDPrefix:   "apool-",
		PathGet:    "/agent-pools/{id}",
		PathList:   "/organizations/{organization_name}/agent-pools",
		PathCreate: "/organizations/{organization_name}/agent-pools",
		Columns:    []string{"name", "organization-scoped", "agent-count"},
	},
	{
		Type:     "configuration-versions",
		Aliases:  []string{"cv", "config-version", "configuration-version"},
		IDPrefix: "cv-",
		PathGet:  "/configuration-versions/{id}",
		Columns:  []string{"status", "speculative", "provisional"},
	},
	{
		Type:     "cost-estimates",
		Aliases:  []string{"cost-estimate", "ce"},
		IDPrefix: "ce-",
		PathGet:  "/cost-estimates/{id}",
		Columns:  []string{"status", "delta-monthly-cost", "proposed-monthly-cost"},
	},
	{
		Type:     "notification-configurations",
		Aliases:  []string{"notification-configuration", "nc"},
		IDPrefix: "nc-",
		PathGet:  "/notification-configurations/{id}",
		Columns:  []string{"name", "destination-type", "enabled", "triggers"},
	},
	{
		Type:     "organization-memberships",
		Aliases:  []string{"organization-membership", "org-membership"},
		IDPrefix: "ou-",
		PathGet:  "/organization-memberships/{id}",
		PathList: "/organizations/{organization_name}/organization-memberships",
		Columns:  []string{"email", "status", "role"},
	},
	{
		Type:     "plan-exports",
		Aliases:  []string{"plan-export"},
		IDPrefix: "pe-",
		PathGet:  "/plan-exports/{id}",
		Columns:  []string{"status", "data-type", "url"},
	},
	{
		Type:     "policy-checks",
		Aliases:  []string{"policy-check"},
		IDPrefix: "polchk-",
		PathGet:  "/policy-checks/{id}",
		Columns:  []string{"status", "scope", "actions", "permissions"},
	},
	{
		Type:    "policy-evaluations",
		Aliases: []string{"policy-evaluation"},
		Columns: []string{"status", "result-count", "passed"},
	},
	{
		Type:     "run-tasks",
		Aliases:  []string{"run-task"},
		IDPrefix: "task-",
		PathGet:  "/tasks/{id}",
		Columns:  []string{"name", "url", "category", "enabled"},
	},
	{
		Type:     "run-triggers",
		Aliases:  []string{"run-trigger"},
		IDPrefix: "rt-",
		PathGet:  "/run-triggers/{id}",
		Columns:  []string{"name", "sourceable-name", "workspace-name"},
	},
	{
		Type:           "state-version-outputs",
		Aliases:        []string{"state-version-output", "svo"},
		IDPrefix:       "wsout-",
		PathGet:        "/state-version-outputs/{id}",
		Columns:        []string{"name", "sensitive", "type"},
		ExcludeColumns: []string{"detailed-type"},
	},
	{
		Type:     "subscriptions",
		Aliases:  []string{"subscription"},
		IDPrefix: "sub-",
		PathGet:  "/subscriptions/{id}",
		Columns:  []string{"status", "plan-name", "quantity"},
	},
	{
		Type:     "task-stages",
		Aliases:  []string{"task-stage"},
		IDPrefix: "ts-",
		Columns:  []string{"status", "stage", "task-result-count"},
	},
	{
		Type:     "policies",
		Aliases:  []string{"policy"},
		IDPrefix: "pol-",
		PathGet:  "/policies/{id}",
	},
	{
		Type:     "feature-sets",
		Aliases:  []string{"feature-set"},
		PathList: "/organizations/{organization_name}/feature-sets",
	},
	{
		Type:     "oauth-clients",
		Aliases:  []string{"oauth-client", "oc"},
		IDPrefix: "oc-",
		PathGet:  "/oauth-clients/{id}",
	},
	{
		Type:     "oauth-tokens",
		Aliases:  []string{"oauth-token", "ot"},
		IDPrefix: "ot-",
		PathGet:  "/oauth-tokens/{id}",
	},
	{
		Type:     "registry-providers",
		Aliases:  []string{"registry-provider"},
		IDPrefix: "prov-",
	},
	{
		Type:     "gpg-keys",
		Aliases:  []string{"gpg-key"},
		IDPrefix: "gpg-",
	},
	{
		Type:     "agents",
		Aliases:  []string{"agent"},
		IDPrefix: "agent-",
		PathGet:  "/agents/{id}",
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

// ColumnsForType returns the preferred display columns for the given type, or nil.
func ColumnsForType(typeName string) []string {
	for i := range registry {
		if registry[i].Type == typeName {
			return registry[i].Columns
		}
	}
	return nil
}

// ExcludeColumnsForType returns columns to exclude for the given type, or nil.
func ExcludeColumnsForType(typeName string) []string {
	for i := range registry {
		if registry[i].Type == typeName {
			return registry[i].ExcludeColumns
		}
	}
	return nil
}
