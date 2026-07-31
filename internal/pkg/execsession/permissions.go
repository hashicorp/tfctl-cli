// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package execsession

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/resource"
)

// DestroyableResourceTypes returns the suggested values for --allow-delete: every
// known destroyable resource class.
func DestroyableResourceTypes() []string {
	allResources := resource.All()

	out := make([]string, 0, len(allResources))
	for _, r := range allResources {
		if r.Destroyable != resource.NotDestroyable {
			out = append(out, r.Type)
		}
	}
	slices.Sort(out)
	return out
}

// AllowsDelete reports whether class is permitted by the granted set. Explicit
// class names always match. An empty/unknown class is always
// denied.
func AllowsDelete(granted []string, class string) bool {
	for _, g := range granted {
		if g == class && class != "" {
			return true // explicit match, including irreversible
		}
	}

	if class == "" {
		return false // unknown path -> deny
	}

	ponder, ok := resource.ByName(class)
	if !ok {
		// The class is unknown to this CLI, so it cannot be allowed.
		return false
	}

	for _, g := range granted {
		if g == ponder.Type {
			return true
		}
	}

	return false
}

// ClassFromPath derives the resource class being deleted from a resolved API
// path. The heuristic returns the collection segment immediately preceding the
// final id segment. It returns "" when it cannot be determined (fewer than two
// meaningful segments), which callers treat as deny-by-default.
//
//	/organizations/tfc-demo-au       -> "organizations"
//	/workspaces/ws-abc               -> "workspaces"
//	/workspaces/ws-abc/vars/var-xyz  -> "vars"
//	/workspaces/ws/relationships/x   -> "x"   (link removal; reversible)
//	/workspaces                      -> ""    (collection only)
func ClassFromPath(p string) string {
	segments := strings.FieldsFunc(p, func(r rune) bool { return r == '/' })

	// Drop a leading "api"/"vN" prefix (e.g. /api/v2/...) so the class
	// heuristic operates on the resource portion of the path.
	for len(segments) > 0 {
		head := segments[0]
		if head == "api" || (len(head) >= 2 && head[0] == 'v' && isAllDigits(head[1:])) {
			segments = segments[1:]
			continue
		}
		break
	}

	if len(segments) < 2 {
		return ""
	}

	// Relationship link removals (DELETE /<...>/relationships/<name>) have no
	// trailing id; the final segment names the linked collection being removed.
	if segments[len(segments)-2] == "relationships" {
		return segments[len(segments)-1]
	}

	// The class is the collection segment immediately preceding the final id.
	return segments[len(segments)-2]
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// NormalizeAllowDelete lowercases, trims, and CSV-splits the raw --allow-delete
// values into a normalized, deduplicated list of types. Unknown types (not
// are returned as warnings but are still kept in the output, since the API surface is large.
func NormalizeAllowDelete(in []string) (out []string, warnings []string) {
	seen := make(map[string]bool)
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			grant := strings.ToLower(strings.TrimSpace(part))
			if grant == "" {
				continue
			}
			if seen[grant] {
				continue
			}
			seen[grant] = true
			out = append(out, grant)

			found, ok := resource.ByName(grant)
			if !ok {
				warnings = append(warnings, fmt.Sprintf("unknown resource type %q in --allow-delete; it may still be honored", grant))
			} else if found.Destroyable == resource.NotDestroyable {
				warnings = append(warnings, fmt.Sprintf("resource type %q in --allow-delete is known to be not destroyable and will never match a delete path", found.Type))
			}
		}
	}
	return out, warnings
}
