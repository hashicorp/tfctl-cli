// Copyright IBM Corp. 2026

// Package cmdutil provides shared helpers for command implementations.
package cmdutil

import (
	"fmt"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
)

// ResolveOrganization returns org from: explicit flag > profile > terraform cloud config.
func ResolveOrganization(ctx *cmd.Context, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if ctx.Profile.Organization != "" {
		return ctx.Profile.Organization
	}
	cfg, err := terraformcfg.FindCloudConfig(".")
	if err == nil && cfg != nil && cfg.Organization != "" {
		return cfg.Organization
	}
	return ""
}

// ResolvePath substitutes {organization_name} in a path template with org.
// Returns an error if org is required but empty, with helpful message about
// setting one via `profile set organization` or `--organization`.
func ResolvePath(pathTemplate, org string) (string, error) {
	if !strings.Contains(pathTemplate, "{organization_name}") {
		return pathTemplate, nil
	}
	if org == "" {
		return "", fmt.Errorf(
			"organization is required but not set\n\n" +
				"Set one with:\n" +
				"  tfctl profile set organization <name>\n" +
				"  --organization <name> / -o <name>",
		)
	}
	return strings.ReplaceAll(pathTemplate, "{organization_name}", org), nil
}
