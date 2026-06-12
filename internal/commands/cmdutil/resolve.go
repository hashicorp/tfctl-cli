// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package cmdutil provides shared helpers for command implementations.
package cmdutil

import (
	"fmt"
	"strings"

	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
)

// ResolveOrganization returns org from: explicit flag > profile > terraform cloud config.
func ResolveOrganization(profileOrganization string, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if profileOrganization != "" {
		return profileOrganization
	}
	cfg, err := terraformcfg.FindCloudConfig(".")
	if err == nil && cfg != nil && cfg.Organization != "" {
		return cfg.Organization
	}
	return ""
}

// ResolvePath substitutes {organization_name} in a path template with org.
// Returns an error if org is required but empty, with helpful message about
// setting one via `profile set default_organization` or `--organization`.
func ResolvePath(pathTemplate, org string) (string, error) {
	if !strings.Contains(pathTemplate, "{organization_name}") {
		return pathTemplate, nil
	}
	if org == "" {
		return "", fmt.Errorf(
			"organization is required but not set\n\n" +
				"Set one with:\n" +
				"  tfctl profile set default_organization <name>\n" +
				"Or use --organization <name> / -o <name>",
		)
	}
	return strings.ReplaceAll(pathTemplate, "{organization_name}", org), nil
}
