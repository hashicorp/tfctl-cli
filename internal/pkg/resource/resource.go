// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package resource provides a registry of known HCP Terraform API resource types.
package resource

type ResourceDestroyability int

const (
	NotDestroyable          ResourceDestroyability = iota // cannot be destroyed
	Destroyable                                           // can be destroyed normally
	DestroyableRecoverable                                // can be destroyed but also can be recovered
	DestroyableButSensitive                               // can be destroyed but is considered irreversible/sensitive
)

// Resource describes a known API resource type.
type Resource struct {
	Type           string                 // JSON:API type: "workspaces"
	Aliases        []string               // shorthand: ["ws", "workspace"]
	IDPrefix       string                 // "ws-" (empty if unknown)
	PathGet        string                 // "/workspaces/{id}"
	PathList       string                 // "/organizations/{organization_name}/workspaces" (empty if not top-level listable)
	PathCreate     string                 // "/organizations/{organization_name}/workspaces" (empty if not supported)
	Resolvable     bool                   // true if the API supports name-to-ID resolution for this type
	Columns        []string               // most important attributes for display (nil = auto-detect)
	ExcludeColumns []string               // attributes to exclude from display
	Destroyable    ResourceDestroyability // indicates if and how this resource can be destroyed
}
