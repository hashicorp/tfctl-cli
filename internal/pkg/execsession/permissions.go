// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package execsession

import (
	"fmt"
	"sort"
	"strings"
)

// IrreversibleClasses are resource classes whose deletes cannot be undone, so
// they are NEVER covered by wildcards and must be named explicitly in
// --allow-delete.
var IrreversibleClasses = map[string]bool{
	"organizations": true,
	"projects":      true,
}

// KnownClasses is a best-effort set of resource classes that tfctl can delete.
// It is used only to warn (not hard-fail) when --allow-delete names something
// outside this set, because the API surface is large and evolving.
var KnownClasses = map[string]bool{
	"organizations":               true,
	"projects":                    true,
	"workspaces":                  true,
	"runs":                        true,
	"vars":                        true,
	"varsets":                     true,
	"teams":                       true,
	"team-workspaces":             true,
	"notification-configurations": true,
	"configuration-versions":      true,
	"state-versions":              true,
	"policy-checks":               true,
	"policies":                    true,
	"policy-sets":                 true,
	"remote-state-consumers":      true,
	"oauth-clients":               true,
	"oauth-tokens":                true,
	"ssh-keys":                    true,
	"agent-pools":                 true,
	"registry-modules":            true,
	"registry-providers":          true,
}

// Sentinels accepted in --allow-delete that mean "any reversible class". "all"
// is treated identically to "reversible" on purpose so there is no footgun
// token that silently includes orgs/projects.
const (
	// SentinelReversible permits deletes of any reversible resource class.
	SentinelReversible = "reversible"

	// SentinelAll is an alias for SentinelReversible. It does NOT cover
	// irreversible classes.
	SentinelAll = "all"
)

// AllowDeleteCompletions returns the suggested values for --allow-delete: every
// known resource class plus the reversible/all sentinels, sorted and
// deduplicated. The irreversible classes are intentionally included so a human
// can tab-complete them when naming them explicitly (wildcards never cover
// them, but explicit grants are allowed).
func AllowDeleteCompletions() []string {
	out := make([]string, 0, len(KnownClasses)+2)
	out = append(out, SentinelReversible, SentinelAll)
	for class := range KnownClasses {
		out = append(out, class)
	}
	sort.Strings(out)
	return out
}

// AllowsDelete reports whether class is permitted by the granted set. Explicit
// class names always match (including irreversible classes). The reversible/all
// sentinels match any non-irreversible class. An empty/unknown class is always
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
	if IrreversibleClasses[class] {
		return false // wildcards never cover irreversible classes
	}

	for _, g := range granted {
		if g == SentinelReversible || g == SentinelAll {
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
// values into a normalized, deduplicated list of classes. Unknown classes (not
// in KnownClasses and not a sentinel) are returned as warnings but are still
// kept in the output, since the API surface is large.
func NormalizeAllowDelete(in []string) (out []string, warnings []string) {
	seen := make(map[string]bool)
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			class := strings.ToLower(strings.TrimSpace(part))
			if class == "" {
				continue
			}
			if seen[class] {
				continue
			}
			seen[class] = true
			out = append(out, class)

			if class == SentinelReversible || class == SentinelAll {
				continue
			}
			if !KnownClasses[class] {
				warnings = append(warnings, fmt.Sprintf("unknown resource class %q in --allow-delete; it will be honored literally but may never match a delete path", class))
			}
		}
	}
	return out, warnings
}
