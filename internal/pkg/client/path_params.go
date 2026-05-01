package client

import (
	"fmt"
	"strings"
)

// ResolvePathParams replaces all {token} placeholders in a URL path with values
// from the tokens map. Any unresolved tokens are returned as an error.
func ResolvePathParams(path string, tokens map[string]string) (string, error) {
	result := path
	for k, v := range tokens {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}

	// Check for unresolved tokens.
	if start := strings.IndexByte(result, '{'); start >= 0 {
		if end := strings.IndexByte(result[start:], '}'); end >= 0 {
			token := result[start+1 : start+end]
			return "", fmt.Errorf("unresolved path token {%s}; use -p '%s=VALUE'", token, token)
		}
	}

	return result, nil
}

// ParsePathParams returns a map of token names to their preceding path segments.
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
	switch segment {
	case "workspaces", "teams", "projects", "varsets":
		return true
	}
	return false
}

// LooksLikeExternalID returns true if the value already appears to be an
// external ID for the given resource segment, based on known prefixes.
func LooksLikeExternalID(segment, value string) bool {
	switch segment {
	case "workspaces":
		return strings.HasPrefix(value, "ws-")
	case "teams":
		return strings.HasPrefix(value, "team-")
	case "projects":
		return strings.HasPrefix(value, "prj-")
	case "varsets":
		return strings.HasPrefix(value, "varset-")
	}
	return false
}
