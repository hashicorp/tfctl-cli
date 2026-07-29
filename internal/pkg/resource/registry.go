// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"slices"
	"sort"
	"strings"
)

// A list of resource types to assist certain commands with identifying, retrieving, and managing
// resources within the system. This list doesn't have to be complete to be effective, but can
// be useful for prediction, column display, and destroy permissions.

var registry = []Resource{
	{
		Type:        "agents",
		Aliases:     []string{"agent"},
		IDPrefix:    "agent-",
		PathGet:     "/agents/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:        "agent-pools",
		Aliases:     []string{"apool", "agent-pool"},
		IDPrefix:    "apool-",
		PathGet:     "/agent-pools/{id}",
		PathList:    "/organizations/{organization_name}/agent-pools",
		PathCreate:  "/organizations/{organization_name}/agent-pools",
		Columns:     []string{"name", "organization-scoped", "agent-count"},
		Destroyable: Destroyable,
	},
	{
		Type:        "applies",
		Aliases:     []string{"apply"},
		IDPrefix:    "apply-",
		PathGet:     "/applies/{id}",
		Columns:     []string{"status", "status-timestamps", "log-read-url"},
		Destroyable: NotDestroyable,
	},
	{
		Type:        "assessment-results",
		IDPrefix:    "asmtres-",
		PathGet:     "/assessment-results/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "authentication-tokens",
		IDPrefix:    "at-",
		PathGet:     "/authentication-tokens/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:        "cidr-range-lists",
		IDPrefix:    "crl-",
		PathGet:     "/cidr-range-lists/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:        "comments",
		IDPrefix:    "wsc-",
		PathGet:     "/comments/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "configuration-versions",
		Aliases:     []string{"cv", "config-version", "configuration-version"},
		IDPrefix:    "cv-",
		PathGet:     "/configuration-versions/{id}",
		Columns:     []string{"status", "speculative", "provisional"},
		Destroyable: NotDestroyable,
	},
	{
		Type:        "cost-estimates",
		Aliases:     []string{"cost-estimate", "ce"},
		IDPrefix:    "ce-",
		PathGet:     "/cost-estimates/{id}",
		Columns:     []string{"status", "delta-monthly-cost", "proposed-monthly-cost"},
		Destroyable: NotDestroyable,
	},
	{
		Type:        "explorer-saved-queries",
		IDPrefix:    "sq-",
		PathGet:     "/organizations/{organization_name}/explorer/views/{id}",
		PathCreate:  "/organizations/{organization_name}/explorer/views",
		Destroyable: Destroyable,
	},
	{
		Type:           "feature-sets",
		IDPrefix:       "fs-",
		Aliases:        []string{"feature-set"},
		PathList:       "/organizations/{organization_name}/feature-sets",
		Destroyable:    NotDestroyable,
		ExcludeColumns: []string{"description"},
	},
	{
		Type:        "github-app-installations",
		IDPrefix:    "ghain-",
		PathGet:     "/github-app/installation/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "gpg-keys",
		Aliases:     []string{"gpg-key"},
		Destroyable: Destroyable,
	},
	{
		Type:        "hyok-configurations",
		IDPrefix:    "hyok-",
		PathGet:     "/hyok-configurations/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:        "hyok-customer-key-versions",
		IDPrefix:    "keyv-",
		PathGet:     "/hyok-customer-key-versions/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "hyok-encrypted-data-keys",
		IDPrefix:    "dek-",
		PathGet:     "/hyok-encrypted-data-keys/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "notification-configurations",
		Aliases:     []string{"notification-configuration", "nc"},
		IDPrefix:    "nc-",
		PathGet:     "/notification-configurations/{id}",
		Columns:     []string{"name", "destination-type", "enabled", "triggers"},
		Destroyable: Destroyable,
	},
	{
		Type:        "oauth-clients",
		Aliases:     []string{"oauth-client", "oc"},
		IDPrefix:    "oc-",
		PathGet:     "/oauth-clients/{id}",
		PathCreate:  "/organizations/{organization_name}/oauth-clients",
		Destroyable: Destroyable,
	},
	{
		Type:        "oauth-tokens",
		Aliases:     []string{"oauth-token", "ot"},
		IDPrefix:    "ot-",
		PathGet:     "/oauth-tokens/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:           "organizations",
		Aliases:        []string{"org", "orgs", "organization"},
		PathGet:        "/organizations/{id}",
		PathList:       "/organizations",
		Columns:        []string{"name", "email", "external-id", "access-beta-tools", "stacks-enabled"},
		ExcludeColumns: []string{"id"},
		Destroyable:    DestroyableButSensitive,
	},
	{
		Type:        "organization-memberships",
		Aliases:     []string{"organization-membership", "org-membership"},
		IDPrefix:    "ou-",
		PathGet:     "/organization-memberships/{id}",
		PathList:    "/organizations/{organization_name}/organization-memberships",
		Columns:     []string{"email", "status", "role"},
		Destroyable: Destroyable,
	},
	{
		Type:        "plans",
		Aliases:     []string{"plan"},
		IDPrefix:    "plan-",
		PathGet:     "/plans/{id}",
		Columns:     []string{"status", "has-changes", "generated-configuration"},
		Destroyable: NotDestroyable,
	},
	{
		Type:        "plan-exports",
		Aliases:     []string{"plan-export"},
		IDPrefix:    "pe-",
		PathGet:     "/plan-exports/{id}",
		Columns:     []string{"status", "data-type", "url"},
		Destroyable: Destroyable,
	},
	{
		Type:        "policies",
		Aliases:     []string{"policy"},
		IDPrefix:    "pol-",
		PathGet:     "/policies/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:        "policy-checks",
		Aliases:     []string{"policy-check"},
		IDPrefix:    "polchk-",
		PathGet:     "/policy-checks/{id}",
		Columns:     []string{"status", "scope", "actions", "permissions"},
		Destroyable: NotDestroyable,
	},
	{
		Type:        "policy-evaluations",
		Aliases:     []string{"policy-evaluation"},
		Columns:     []string{"status", "result-count", "passed"},
		Destroyable: NotDestroyable,
	},
	{
		Type:        "policy-sets",
		Aliases:     []string{"policy-set", "polset"},
		IDPrefix:    "polset-",
		PathGet:     "/policy-sets/{id}",
		PathList:    "/organizations/{organization_name}/policy-sets",
		PathCreate:  "/organizations/{organization_name}/policy-sets",
		Columns:     []string{"name", "kind", "global", "overridable"},
		Destroyable: Destroyable,
	},
	{
		Type:        "projects",
		Aliases:     []string{"prj", "project"},
		IDPrefix:    "prj-",
		PathGet:     "/projects/{id}",
		PathList:    "/organizations/{organization_name}/projects",
		PathCreate:  "/organizations/{organization_name}/projects",
		Resolvable:  true,
		Columns:     []string{"name", "description", "organization-name"},
		Destroyable: DestroyableButSensitive,
	},
	{
		Type:           "provider-sets",
		Aliases:        []string{"provider-set", "provset"},
		IDPrefix:       "provset-",
		PathGet:        "/provider-sets/{id}",
		PathList:       "/organizations/{organization_name}/provider-sets",
		PathCreate:     "/organizations/{organization_name}/provider-sets",
		Columns:        []string{"name", "provider-source", "global", "priority", "updated-at"},
		ExcludeColumns: []string{"configuration-hcl"},
		Destroyable:    Destroyable,
	},
	{
		Type:        "queries",
		IDPrefix:    "query-",
		PathGet:     "/queries/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "registry-modules",
		Aliases:     []string{"registry-module"},
		IDPrefix:    "mod-",
		Destroyable: Destroyable,
	},
	{
		Type:        "registry-module-versions",
		Aliases:     []string{"registry-module-version"},
		IDPrefix:    "modver-",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "registry-providers",
		Aliases:     []string{"registry-provider"},
		IDPrefix:    "prov-",
		Destroyable: Destroyable,
	},
	{
		Type:        "registry-provider-versions",
		IDPrefix:    "provver-",
		Destroyable: Destroyable,
	},
	{
		Type:        "registry-provider-version-platforms",
		IDPrefix:    "provpltfrm-",
		Destroyable: Destroyable,
	},
	{
		Type:        "reserved-tag-keys",
		IDPrefix:    "rtk-",
		Destroyable: Destroyable,
	},
	{
		Type:        "runs",
		Aliases:     []string{"run"},
		IDPrefix:    "run-",
		PathGet:     "/runs/{id}",
		Columns:     []string{"message", "status", "is-destroy", "has-changes"},
		Destroyable: NotDestroyable,
	},
	{
		Type:        "run-tasks",
		Aliases:     []string{"run-task"},
		IDPrefix:    "task-",
		PathGet:     "/tasks/{id}",
		Columns:     []string{"name", "url", "category", "enabled"},
		Destroyable: Destroyable,
	},
	{
		Type:        "run-triggers",
		Aliases:     []string{"run-trigger"},
		IDPrefix:    "rt-",
		PathGet:     "/run-triggers/{id}",
		Columns:     []string{"name", "sourceable-name", "workspace-name"},
		Destroyable: Destroyable,
	},
	{
		Type:        "ssh-keys",
		IDPrefix:    "ssh-",
		PathGet:     "/ssh-keys/{id}",
		PathCreate:  "/organizations/{organization_name}/ssh-keys",
		Destroyable: Destroyable,
	},
	{
		Type:        "stacks",
		IDPrefix:    "st-",
		PathGet:     "/stacks/{id}",
		Destroyable: DestroyableRecoverable,
	},
	{
		Type:        "stack-configurations",
		IDPrefix:    "stc-",
		PathGet:     "/stack-configurations/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "stack-states",
		IDPrefix:    "sts-",
		PathGet:     "/stack-states/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "state-versions",
		Aliases:     []string{"sv", "state-version"},
		IDPrefix:    "sv-",
		PathGet:     "/state-versions/{id}",
		Columns:     []string{"serial", "status", "resource-count", "size"},
		Destroyable: NotDestroyable,
	},
	{
		Type:           "state-version-outputs",
		Aliases:        []string{"state-version-output", "svo"},
		IDPrefix:       "wsout-",
		PathGet:        "/state-version-outputs/{id}",
		Columns:        []string{"name", "sensitive", "type"},
		ExcludeColumns: []string{"detailed-type"},
		Destroyable:    NotDestroyable,
	},
	{
		Type:        "subscriptions",
		Aliases:     []string{"subscription"},
		IDPrefix:    "sub-",
		PathGet:     "/subscriptions/{id}",
		Columns:     []string{"status", "plan-name", "quantity"},
		Destroyable: NotDestroyable,
	},
	{
		Type:     "task-stages",
		Aliases:  []string{"task-stage"},
		IDPrefix: "ts-",
		Columns:  []string{"status", "stage", "task-result-count"},
	},
	{
		Type:        "teams",
		Aliases:     []string{"team"},
		IDPrefix:    "team-",
		PathGet:     "/teams/{id}",
		PathList:    "/organizations/{organization_name}/teams",
		PathCreate:  "/organizations/{organization_name}/teams",
		Resolvable:  true,
		Destroyable: Destroyable,
	},
	{
		Type:        "team-projects",
		IDPrefix:    "tprj-",
		PathGet:     "/team-projects/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:        "team-workspaces",
		IDPrefix:    "tws-",
		PathGet:     "/team-workspaces/{id}",
		Destroyable: Destroyable,
	},
	{
		Type:        "test-runs",
		IDPrefix:    "trun-",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "users",
		IDPrefix:    "usr-",
		PathGet:     "/users/{id}",
		Destroyable: NotDestroyable,
	},
	{
		Type:        "vars",
		Aliases:     []string{"var", "variable", "variables"},
		IDPrefix:    "var-",
		PathGet:     "/vars/{id}",
		Columns:     []string{"key", "value", "category", "hcl", "sensitive"},
		Destroyable: Destroyable,
	},
	{
		Type:        "varsets",
		Aliases:     []string{"varset", "variable-sets", "variable-set"},
		IDPrefix:    "varset-",
		PathGet:     "/varsets/{id}",
		PathList:    "/organizations/{organization_name}/varsets",
		PathCreate:  "/organizations/{organization_name}/varsets",
		Resolvable:  true,
		Columns:     []string{"name", "description", "global", "priority"},
		Destroyable: Destroyable,
	},
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
		Destroyable:    DestroyableRecoverable,
	},
}

func init() {
	slices.SortFunc(registry, func(a, b Resource) int {
		return strings.Compare(a.Type, b.Type)
	})
}

// ByNameOrAlias matches the canonical Type or any Alias, case-insensitive.
// Returns nil if not found.
func ByNameOrAlias(name string) *Resource {
	lower := strings.ToLower(name)
	if exact, ok := ByName(lower); ok {
		return &exact
	}

	for i := range registry {
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

// AllDestroyable returns all registered resources that are destroyable.
func AllDestroyable() []Resource {
	out := make([]Resource, 0, len(registry)-20) // Estimated number of NotDestroyable resources above
	for _, r := range registry {
		if r.Destroyable != NotDestroyable {
			out = append(out, r)
		}
	}
	return out
}

// ByName looks up a resource by type name and returns it along with a boolean indicating if it was found.
// Presumes that the registry is sorted by the Type field.
func ByName(typeName string) (Resource, bool) {
	index, ok := slices.BinarySearchFunc(registry, Resource{Type: typeName}, func(a, b Resource) int {
		return strings.Compare(a.Type, b.Type)
	})
	if ok {
		return registry[index], true
	}
	return Resource{}, false
}

// ColumnsForType returns the preferred display columns for the given type, or nil.
func ColumnsForType(typeName string) []string {
	r, ok := ByName(typeName)
	if !ok {
		return nil
	}
	return r.Columns
}

// ExcludeColumnsForType returns columns to exclude for the given type, or nil.
func ExcludeColumnsForType(typeName string) []string {
	r, ok := ByName(typeName)
	if !ok {
		return nil
	}
	return r.ExcludeColumns
}

// IDPrefixForType returns the ID prefix for the given type name, or "" if unknown.
func IDPrefixForType(typeName string) string {
	r, ok := ByName(typeName)
	if !ok {
		return ""
	}
	return r.IDPrefix
}

// IsResolvableType returns true if the given type name (e.g. "workspaces")
// supports name-to-ID resolution via the API.
func IsResolvableType(typeName string) bool {
	r, ok := ByName(typeName)
	if !ok {
		return false
	}
	return r.Resolvable
}
