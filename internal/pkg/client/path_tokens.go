package client

import (
	"context"
	"fmt"
	"strings"
)

// PathTokenResolutionOpts configures path token resolution.
type PathTokenResolutionOpts struct {
	// PathTokens are explicit token=value mappings from the -p flag.
	PathTokens map[string]string
	// Organization is the resolved organization context (from flag, profile, or terraform config).
	Organization string
	// Workspace is the resolved workspace name (from terraform config), used for auto-resolution.
	Workspace string
}

// ResolvePathTokens resolves all {token} placeholders in a URL path to their API values.
// Tokens that map to a known resource type are resolved using the Resolver (name→ID).
// The organization token is substituted directly (TFC API accepts org names in paths).
// Returns the resolved path string with all tokens replaced.
func ResolvePathTokens(ctx context.Context, path string, opts PathTokenResolutionOpts, resolver *Resolver) (string, error) {
	var errs []string
	result := path

	for {
		start := strings.IndexByte(result, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(result[start:], '}')
		if end < 0 {
			break
		}
		end += start

		tokenName := result[start+1 : end]
		value, err := resolveTokenValue(ctx, tokenName, opts, resolver)
		if err != nil {
			errs = append(errs, err.Error())
			result = result[:start] + result[end+1:]
			continue
		}

		result = result[:start] + value + result[end+1:]
	}

	if len(errs) > 0 {
		return "", fmt.Errorf("failed to resolve path tokens:\n  %s", strings.Join(errs, "\n  "))
	}

	return result, nil
}

// resolveTokenValue determines the resolved value for a single path token.
// It first checks for an explicit -p value, then falls back to auto-resolution from config.
func resolveTokenValue(ctx context.Context, tokenName string, opts PathTokenResolutionOpts, resolver *Resolver) (string, error) {
	name, explicit := opts.PathTokens[tokenName]

	switch tokenName {
	case "organization", "organization_name":
		if explicit {
			return name, nil
		}
		if opts.Organization == "" {
			return "", fmt.Errorf("{%s}: no organization configured; use -p '%s=NAME' or set a default organization in your profile", tokenName, tokenName)
		}
		return opts.Organization, nil

	case "workspace", "workspace_id", "workspace_name":
		if !explicit {
			name = opts.Workspace
		}
		if name == "" {
			return "", fmt.Errorf("{%s}: no workspace configured; use -p '%s=NAME' or run in a directory with terraform cloud configuration", tokenName, tokenName)
		}
		if opts.Organization == "" {
			return "", fmt.Errorf("{%s}: organization is required to resolve workspace %q", tokenName, name)
		}
		id, err := resolver.Workspace(ctx, opts.Organization, name)
		if err != nil {
			return "", fmt.Errorf("{%s}: %w", tokenName, err)
		}
		return *id, nil

	case "team", "team_id":
		if !explicit {
			return "", fmt.Errorf("{%s}: use -p '%s=NAME' to specify a team", tokenName, tokenName)
		}
		if opts.Organization == "" {
			return "", fmt.Errorf("{%s}: organization is required to resolve team %q", tokenName, name)
		}
		id, err := resolver.Team(ctx, opts.Organization, name)
		if err != nil {
			return "", fmt.Errorf("{%s}: %w", tokenName, err)
		}
		return *id, nil

	case "project", "project_id":
		if !explicit {
			return "", fmt.Errorf("{%s}: use -p '%s=NAME' to specify a project", tokenName, tokenName)
		}
		if opts.Organization == "" {
			return "", fmt.Errorf("{%s}: organization is required to resolve project %q", tokenName, name)
		}
		id, err := resolver.Project(ctx, opts.Organization, name)
		if err != nil {
			return "", fmt.Errorf("{%s}: %w", tokenName, err)
		}
		return *id, nil

	case "varset", "varset_id":
		if !explicit {
			return "", fmt.Errorf("{%s}: use -p '%s=NAME' to specify a variable set", tokenName, tokenName)
		}
		if opts.Organization == "" {
			return "", fmt.Errorf("{%s}: organization is required to resolve variable set %q", tokenName, name)
		}
		id, err := resolver.VariableSet(ctx, opts.Organization, name)
		if err != nil {
			return "", fmt.Errorf("{%s}: %w", tokenName, err)
		}
		return *id, nil

	default:
		if explicit {
			return name, nil
		}
		return "", fmt.Errorf("{%s}: unrecognized path token; use -p '%s=VALUE' to provide a value", tokenName, tokenName)
	}
}
