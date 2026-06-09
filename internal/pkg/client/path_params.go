// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"fmt"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/resource"
)

// ResolvePathParams replaces all {param} placeholders in a URL path with values
// from the params map. Any unresolved params are returned as an error.
func ResolvePathParams(path string, params map[string]string) (string, error) {
	result := path
	for k, v := range params {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}

	// Check for unresolved params.
	if start := strings.IndexByte(result, '{'); start >= 0 {
		if end := strings.IndexByte(result[start:], '}'); end >= 0 {
			param := result[start+1 : start+end]
			return "", fmt.Errorf("unresolved path param {%s}; use -p '%s=VALUE'", param, param)
		}
	}

	return result, nil
}

// ParsePathParams returns a map of param names to their preceding path segments.
// For example, "/workspaces/{workspace_id}/runs" returns {"workspace_id": "workspaces"}.
func ParsePathParams(path string) map[string]string {
	result := make(map[string]string)
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(path[start:], '}')
		if end < 0 {
			break
		}
		token := path[start+1 : start+end]

		// Extract the segment before /{token}.
		segment := ""
		if start > 1 {
			sub := path[:start]
			if sub[len(sub)-1] == '/' {
				sub = sub[:len(sub)-1]
			}
			if i := strings.LastIndexByte(sub, '/'); i >= 0 {
				segment = sub[i+1:]
			} else {
				segment = sub
			}
		}
		result[token] = segment
		path = path[start+end+1:]
	}
	return result
}

// IsResolvableSegment returns true if the segment is a resource type that
// supports name-to-ID resolution via the API.
func IsResolvableSegment(segment string) bool {
	return resource.IsResolvableType(segment)
}

// LooksLikeID returns true if the value already appears to be an
// ID for the given resource segment, based on known prefixes.
func LooksLikeID(segment, value string) bool {
	prefix := resource.IDPrefixForType(segment)
	if prefix == "" {
		return false
	}
	return strings.HasPrefix(value, prefix)
}
