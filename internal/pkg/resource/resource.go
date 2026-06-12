// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package resource provides a registry of known HCP Terraform API resource types.
package resource

// Resource describes a known API resource type.
type Resource struct {
	Type           string   // JSON:API type: "workspaces"
	Aliases        []string // shorthand: ["ws", "workspace"]
	IDPrefix       string   // "ws-" (empty if unknown)
	PathGet        string   // "/workspaces/{id}"
	PathList       string   // "/organizations/{organization_name}/workspaces" (empty if not top-level listable)
	PathCreate     string   // "/organizations/{organization_name}/workspaces" (empty if not supported)
	Resolvable     bool     // true if the API supports name-to-ID resolution for this type
	Columns        []string // most important attributes for display (nil = auto-detect)
	ExcludeColumns []string // attributes to exclude from display
}
