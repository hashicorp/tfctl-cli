// Copyright IBM Corp. 2026

package resource

// Resource describes a known API resource type.
type Resource struct {
	Type       string   // JSON:API type: "workspaces"
	Aliases    []string // shorthand: ["ws", "workspace"]
	IDPrefix   string   // "ws-" (empty if unknown)
	PathGet    string   // "/workspaces/{id}"
	PathList   string   // "/organizations/{organization_name}/workspaces" (empty if not top-level listable)
	PathCreate string   // "/organizations/{organization_name}/workspaces" (empty if not supported)
	Columns    []string // preferred display columns for table output
}
