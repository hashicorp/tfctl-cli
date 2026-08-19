// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/hashicorp/tfctl-cli/internal/pkg/openapi"
)

// linkage describes a JSON:API relationship's linkage as declared by the
// schema: the resource type its members point to, and whether it is to-many.
type linkage struct {
	Type   string
	ToMany bool
}

// relationshipLinkages returns the relationships declared on the POST request
// body for the operation whose templated path matches the concrete requestPath,
// keyed by relationship name.
//
// A relationship is included only when the schema pins its linkage type to a
// single value; links-only relationships (no data) and ambiguous ones (a type
// enum with more than one member, e.g. locked-by → users|teams|runs) are
// omitted so the caller falls back to requiring an explicit name:type=id.
//
// ok is false when no schema was available, no POST operation matched the path,
// or the operation declares no settable relationships.
func relationshipLinkages(oas openapi.Schema, requestPath string) (result map[string]linkage, ok bool) {
	if oas == nil {
		return nil, false
	}

	tmpl := matchTemplatePath(oas, requestPath)
	if tmpl == "" {
		return nil, false
	}

	pathItem, err := oas.PathByPath(tmpl)
	if err != nil || pathItem.Post == nil {
		return nil, false
	}
	rels := requestBodyRelationships(pathItem.Post)
	if rels == nil {
		return nil, false
	}

	out := make(map[string]linkage, len(rels.Properties))
	for name, ref := range rels.Properties {
		if ref.Value == nil {
			continue
		}
		data := ref.Value.Properties["data"]
		if data == nil || data.Value == nil {
			continue // links-only relationship: not settable via a linkage
		}

		target := data.Value
		toMany := false
		if data.Value.Items != nil {
			toMany = true
			target = data.Value.Items.Value
		}

		enum := schemaTypeEnum(target)
		if len(enum) != 1 {
			continue // zero → not a linkage; >1 → ambiguous, require explicit type
		}
		typ, isStr := enum[0].(string)
		if !isStr || typ == "" {
			continue
		}
		out[name] = linkage{Type: typ, ToMany: toMany}
	}

	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// requestBodyRelationships returns the schema of the JSON:API "relationships"
// object in an operation's request body, or nil if it has none.
func requestBodyRelationships(op *openapi3.Operation) *openapi3.Schema {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return nil
	}
	media := op.RequestBody.Value.Content["application/vnd.api+json"]
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		return nil
	}
	data := media.Schema.Value.Properties["data"]
	if data == nil || data.Value == nil {
		return nil
	}
	rels := data.Value.Properties["relationships"]
	if rels == nil || rels.Value == nil {
		return nil
	}
	return rels.Value
}

// schemaTypeEnum returns the enum values constraining a JSON:API identifier's
// "type" field, following allOf/oneOf composition to reach the identifier
// schema. To-one linkages wrap the identifier in an allOf; to-many ones expose
// it directly under the array items.
func schemaTypeEnum(s *openapi3.Schema) []any {
	if s == nil {
		return nil
	}
	if p, ok := s.Properties["type"]; ok && p.Value != nil && len(p.Value.Enum) > 0 {
		return p.Value.Enum
	}
	for _, sub := range s.AllOf {
		if sub.Value != nil {
			if e := schemaTypeEnum(sub.Value); e != nil {
				return e
			}
		}
	}
	for _, sub := range s.OneOf {
		if sub.Value != nil {
			if e := schemaTypeEnum(sub.Value); e != nil {
				return e
			}
		}
	}
	return nil
}

// matchTemplatePath returns the templated spec path whose shape matches the
// given concrete path (e.g. /organizations/acme/workspaces matches
// /organizations/{organization_name}/workspaces), or "" if none matches. A spec
// segment matches when it equals the concrete segment or is a {placeholder}.
func matchTemplatePath(oas openapi.Schema, concrete string) string {
	want := splitPathSegments(concrete)
	for _, key := range oas.Paths().Keys() {
		have := splitPathSegments(key)
		if len(have) != len(want) {
			continue
		}
		matched := true
		for i, seg := range have {
			if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
				continue
			}
			if seg != want[i] {
				matched = false
				break
			}
		}
		if matched {
			return key
		}
	}
	return ""
}

func splitPathSegments(p string) []string {
	return strings.FieldsFunc(strings.Trim(p, "/"), func(r rune) bool { return r == '/' })
}

// buildRelationships constructs the JSON:API "relationships" object from the -r
// flag values. Each flag key is a relationship name, optionally suffixed with an
// explicit ":type" override (name:type=id); without the override the linkage
// type and cardinality are resolved from the schema. Ids for a to-many
// relationship are comma-separated.
func buildRelationships(rels map[string]string, linkages map[string]linkage, haveSchema bool) (map[string]any, error) {
	if len(rels) == 0 {
		return nil, nil
	}

	out := make(map[string]any, len(rels))
	for rawKey, rawVal := range rels {
		name, explicitType, _ := strings.Cut(rawKey, ":")
		if name == "" {
			return nil, fmt.Errorf("relationship name is empty in %q", rawKey)
		}

		lk, known := linkages[name]

		typ := explicitType
		if typ == "" {
			if !known {
				return nil, unknownRelationshipError(name, linkages, haveSchema)
			}
			typ = lk.Type
		}

		ids := splitIDs(rawVal)
		if len(ids) == 0 {
			return nil, fmt.Errorf("relationship %q has no id", name)
		}

		// Cardinality comes from the schema when the relationship is known;
		// otherwise infer it from the number of ids supplied.
		var toMany bool
		switch {
		case known:
			toMany = lk.ToMany
		default:
			toMany = len(ids) > 1
		}

		if !toMany {
			if len(ids) > 1 {
				return nil, fmt.Errorf("relationship %q is to-one but got %d ids: %s", name, len(ids), strings.Join(ids, ", "))
			}
			out[name] = map[string]any{"data": identifier(typ, ids[0])}
			continue
		}

		data := make([]any, 0, len(ids))
		for _, id := range ids {
			data = append(data, identifier(typ, id))
		}
		out[name] = map[string]any{"data": data}
	}
	return out, nil
}

func identifier(typ, id string) map[string]any {
	return map[string]any{"type": typ, "id": id}
}

// splitIDs splits a comma-separated id list, trimming whitespace and dropping
// empty entries.
func splitIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// unknownRelationshipError explains that a relationship name could not be
// resolved, listing the valid names when a schema was consulted or pointing at
// the explicit override when it was not.
func unknownRelationshipError(name string, linkages map[string]linkage, haveSchema bool) error {
	override := fmt.Sprintf("-r %s:<type>=<id>", name)
	if !haveSchema || len(linkages) == 0 {
		return fmt.Errorf("could not infer the resource type for relationship %q; specify it explicitly with %s", name, override)
	}

	names := make([]string, 0, len(linkages))
	for n := range linkages {
		names = append(names, n)
	}
	sort.Strings(names)
	return fmt.Errorf("unknown relationship %q for this endpoint; valid relationships: %s (or override with %s)",
		name, strings.Join(names, ", "), override)
}
