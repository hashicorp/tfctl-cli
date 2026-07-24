// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package resource provides a registry of known HCP Terraform API resource types.
package resource

// Destroyability indicates whether a resource can be destroyed.
type Destroyability int

const (
	// NotDestroyable indicates that the resource cannot be destroyed.
	NotDestroyable Destroyability = iota
	// Destroyable indicates that the resource can be destroyed normally.
	Destroyable
	// DestroyableRecoverable indicates that the resource can be destroyed but also can be recovered.
	DestroyableRecoverable
	// DestroyableButSensitive indicates that the resource can be destroyed but is considered irreversible/sensitive.
	DestroyableButSensitive
)

// Resource describes a known API resource type.
type Resource struct {
	Type           string         // JSON:API type: "workspaces"
	Aliases        []string       // shorthand: ["ws", "workspace"]
	IDPrefix       string         // "ws-" (empty if unknown)
	PathGet        string         // "/workspaces/{id}"
	PathList       string         // "/organizations/{organization_name}/workspaces" (empty if not top-level listable)
	PathCreate     string         // "/organizations/{organization_name}/workspaces" (empty if not supported)
	Resolvable     bool           // true if the API supports name-to-ID resolution for this type
	Columns        []string       // most important attributes for display (nil = auto-detect)
	ExcludeColumns []string       // attributes to exclude from display
	Destroyable    Destroyability // indicates if and how this resource can be destroyed
}
