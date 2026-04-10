// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package render

// typeColumns is a mapping of API resource types to their preferred columns for horizontal table rendering.
var typeColumns = map[string][]string{
	"agent-pools":                 {"name", "organization-scoped", "agent-count"},
	"applies":                     {"status", "status-timestamps", "log-read-url"},
	"configuration-versions":      {"status", "speculative", "provisional"},
	"cost-estimates":              {"status", "delta-monthly-cost", "proposed-monthly-cost"},
	"notification-configurations": {"name", "destination-type", "enabled", "triggers"},
	"organization-memberships":    {"email", "status", "role"},
	"organizations":               {"name", "email", "external-id"},
	"plan-exports":                {"status", "data-type", "url"},
	"plans":                       {"status", "has-changes", "generated-configuration"},
	"policy-checks":               {"status", "scope", "actions", "permissions"},
	"policy-evaluations":          {"status", "result-count", "passed"},
	"policy-sets":                 {"name", "kind", "global", "overridable"},
	"projects":                    {"name", "description", "organization-name"},
	"run-tasks":                   {"name", "url", "category", "enabled"},
	"run-triggers":                {"name", "sourceable-name", "workspace-name"},
	"runs":                        {"message", "status", "is-destroy", "has-changes"},
	"state-version-outputs":       {"name", "sensitive", "type"},
	"state-versions":              {"serial", "status", "resource-count", "size"},
	"subscriptions":               {"status", "plan-name", "quantity"},
	"task-stages":                 {"status", "stage", "task-result-count"},
	"varsets":                     {"name", "description", "global", "priority"},
	"vars":                        {"key", "value", "category", "hcl", "sensitive"},
	"workspaces":                  {"name", "description", "execution-mode", "locked", "resource-count"},
}

var excludeColumns = map[string][]string{
	"workspaces": {"actions"},
}
