// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/go-hclog"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/hashicorp/tfctl-cli/internal/pkg/resource"
)

// MaxTableColumns is the maximum number of columns shown in horizontal table output.
const MaxTableColumns = 6

// ErrNotJSONAPI is the error returned by NewJSONAPIDisplayer when the raw data
// does not conform to the expected JSON:API shape.
var ErrNotJSONAPI = errors.New("not a JSON:API envelope")

// acronyms are capitalized in field names, e.g. "VCS Repo.Repository HTTP URL" instead of "Vcs Repo.Repository Http Url".
var acronyms = map[string]string{
	"api":   "API",
	"cidr":  "CIDR",
	"http":  "HTTP",
	"https": "HTTPS",
	"id":    "ID",
	"ids":   "IDs",
	"ip":    "IP",
	"kpis":  "KPIs",
	"oauth": "OAuth",
	"rum":   "RUM",
	"saml":  "SAML",
	"scim":  "SCIM",
	"ssh":   "SSH",
	"sso":   "SSO",
	"ttl":   "TTL",
	"url":   "URL",
	"vcs":   "VCS",
}

const sentinelResourceType = "__JSONAPI_RESOURCE_TYPE__"

// JSONAPIDisplayer prepares responses within a JSON:API data envelope to be formatted.
type JSONAPIDisplayer struct {
	payload      any
	rawPayload   any
	resourceType string
	collection   bool
	logger       hclog.Logger
}

// Check interface at compile time.
var _ Displayer = JSONAPIDisplayer{}

// Any attribute keys that contain characters other than letters, numbers, hyphens, underscores,
// and periods are skipped for display. Usually indicates user content in embedded
// objects attributes.
var reAttributeNameDisallow = regexp.MustCompile(`[^-_.a-zA-Z0-9]`)

// DefaultFormat implements the Displayer interface.
func (d JSONAPIDisplayer) DefaultFormat() Format {
	if d.collection {
		return Table
	}
	return Pretty
}

// Payload implements the Displayer interface. It returns the full JSON:API
// envelope for use by the JSON output format.
func (d JSONAPIDisplayer) Payload() any {
	return d.rawPayload
}

// TemplatedPayload implements the TemplatedPayload interface. It returns the
// flattened attribute rows for use by table and pretty output formats.
func (d JSONAPIDisplayer) TemplatedPayload() any {
	return d.payload
}

// FieldTemplates implements the Displayer interface.
func (d JSONAPIDisplayer) FieldTemplates() []Field {
	var cols []string
	if d.DefaultFormat() == Table {
		rows := d.payload.([]map[string]any)
		cols = collectColumns(rows, resource.ColumnsForType(d.resourceType), resource.ExcludeColumnsForType(d.resourceType))
	} else {
		rows := d.payload.(map[string]any)
		cols = orderedFields(rows, resource.ColumnsForType(d.resourceType), resource.ExcludeColumnsForType(d.resourceType))
	}

	result := make([]Field, 0, len(cols))

	for _, att := range cols {
		name := att
		// Nasty special case for organizations
		if att == "external-id" {
			name = "id"
		}

		// Skip attributes that contain keys that don't look like API attributes. Certain output values
		// may be user data, such as the object value of a state version output.
		if name == sentinelResourceType {
			if d.DefaultFormat() != Table {
				result = append(result, NewField("Resource", fmt.Sprintf(`{{ index . "%s" }}`, sentinelResourceType)))
			}
			continue
		} else if reAttributeNameDisallow.MatchString(name) {
			d.logger.Debug("Skipping attribute for display due to invalid characters", "attribute", att)
			continue
		}

		result = append(result, NewField(kebabToLabel(name), fmt.Sprintf(`{{ index . "%s" }}`, att)))
	}

	return result
}

func kebabToLabel(input string) string {
	caser := cases.Title(language.AmericanEnglish)
	parts := strings.Split(input, ".")
	for i, part := range parts {
		spacedParts := strings.Split(part, "-")
		for j, spacedPart := range spacedParts {
			if acronym, ok := acronyms[spacedPart]; ok {
				spacedParts[j] = acronym
			} else {
				spacedParts[j] = caser.String(spacedPart)
			}
		}

		parts[i] = strings.Join(spacedParts, " ")
	}
	return strings.Join(parts, ".")
}

// NewJSONAPIDisplayer creates a new displayer based on the contents of a JSON:API response body.
func NewJSONAPIDisplayer(raw []byte, logger hclog.Logger) (*JSONAPIDisplayer, error) {
	resourceType := ""
	collection := true
	var rawPayload map[string]any
	if err := json.Unmarshal(raw, &rawPayload); err != nil {
		return nil, ErrNotJSONAPI
	}

	data, ok := rawPayload["data"]
	if !ok {
		return nil, ErrNotJSONAPI
	}

	var payload any
	switch typed := data.(type) {
	case []any:
		payload = make([]map[string]any, len(typed))
		for i, item := range typed {
			row, ok := resourceAsMap(item)
			if !ok {
				return nil, ErrNotJSONAPI
			}
			if resourceType == "" && row[sentinelResourceType] != nil {
				if rt, ok := row[sentinelResourceType].(string); ok {
					resourceType = rt
				}
			}
			payload.([]map[string]any)[i] = row
		}
	case map[string]any:
		collection = false
		row, ok := resourceAsMap(typed)
		if !ok {
			return nil, ErrNotJSONAPI
		}
		payload = row
		if row[sentinelResourceType] != nil {
			if rt, ok := row[sentinelResourceType].(string); ok {
				resourceType = rt
			}
		}
	default:
		return nil, ErrNotJSONAPI
	}

	return &JSONAPIDisplayer{
		payload:      payload,
		rawPayload:   rawPayload,
		resourceType: resourceType,
		collection:   collection,
		logger:       logger,
	}, nil
}

func resourceAsMap(item any) (map[string]any, bool) {
	obj, ok := item.(map[string]any)
	if !ok {
		return nil, false
	}

	row := map[string]any{}
	if id, ok := obj["id"]; ok {
		row["id"] = id
	}
	if kind, ok := obj["type"]; ok {
		row[sentinelResourceType] = kind
	}

	attrs, ok := obj["attributes"]
	if !ok {
		return row, len(row) > 0
	}
	attrMap, ok := attrs.(map[string]any)
	if !ok {
		return nil, false
	}

	// Copy attributes into the row. We must not mutate attrMap directly: it is
	// shared with the raw payload that backs --json / --jq output, and pulling
	// relationship IDs into it would pollute that output with keys the server
	// never returned under "attributes".
	for key, value := range attrMap {
		row[key] = value
	}

	// Look for one-to-one relationships and pull the ID up to the top level of the row for display.
	rels, ok := obj["relationships"]
	if ok {
		for rel, relData := range rels.(map[string]any) {
			relObj, ok := relData.(map[string]any)
			if !ok {
				continue
			}
			data, ok := relObj["data"]
			if !ok {
				continue
			}
			if oneToOne, isOneToOne := data.(map[string]any); isOneToOne {
				row[rel] = oneToOne["id"]
			}
		}
	}

	flattenRow(row)
	return row, true
}

// flattenRow expands map[string]any and []any values in the row into dot-separated keys,
// recursively flattening nested collections.
func flattenRow(row map[string]any) {
	for key, value := range row {
		switch v := value.(type) {
		case map[string]any:
			delete(row, key)
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				row[key+"."+k] = v[k]
			}
		case []any:
			delete(row, key)
			for i, item := range v {
				row[fmt.Sprintf("%s.%d", key, i)] = item
			}
		}
	}
	// Check if any new values still need flattening
	for _, value := range row {
		if isNestedValue(value) {
			flattenRow(row)
			return
		}
	}
}

func orderedFields(row map[string]any, preferred []string, exclude []string) []string {
	seen := make(map[string]struct{}, len(row))
	ordered := make([]string, 0, len(row))
	if _, ok := row["id"]; ok {
		ordered = append(ordered, "id")
		seen["id"] = struct{}{}
	}
	if _, ok := row[sentinelResourceType]; ok {
		ordered = append(ordered, sentinelResourceType)
		seen[sentinelResourceType] = struct{}{}
	}

	for _, key := range preferred {
		if _, ok := row[key]; ok {
			ordered = append(ordered, key)
			seen[key] = struct{}{}
		}
	}

	remaining := make([]string, 0, len(row)-len(ordered))
	nested := make([]string, 0, len(row))
	for key := range row {
		if _, ok := seen[key]; ok {
			continue
		}

		shouldExclude := slices.ContainsFunc(exclude, func(s string) bool {
			return s == key || strings.HasPrefix(key, s+".")
		})

		if isNestedValue(row[key]) || isNestedKey(key) {
			if !shouldExclude {
				nested = append(nested, key)
			}
			continue
		}

		if !shouldExclude {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	sort.Strings(nested)
	ordered = append(ordered, remaining...)
	ordered = append(ordered, nested...)
	return ordered
}

func isNestedValue(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func isNestedKey(key string) bool {
	return strings.Contains(key, ".")
}

func collectColumns(rows []map[string]any, preferred []string, exclude []string) []string {
	seen := map[string]struct{}{}
	for _, row := range rows {
		for key := range row {
			seen[key] = struct{}{}
		}
	}

	columns := make([]string, 0, len(seen))
	if _, ok := seen["id"]; ok {
		columns = append(columns, "id")
		delete(seen, "id")
		if len(columns) >= MaxTableColumns {
			return columns
		}
	}

	for _, key := range preferred {
		if _, ok := seen[key]; ok {
			columns = append(columns, key)
			delete(seen, key)
			if len(columns) >= MaxTableColumns {
				return columns
			}
		}
	}

	remaining := make([]string, 0, len(seen))
	for key := range seen {
		shouldExclude := slices.ContainsFunc(exclude, func(s string) bool {
			if s == key || strings.HasPrefix(key, s+".") {
				return true
			}
			return false
		})

		if !shouldExclude {
			remaining = append(remaining, key)
		}
	}

	sort.Strings(remaining)
	if len(remaining) < MaxTableColumns-len(columns) {
		return append(columns, remaining...)
	}
	return append(columns, remaining[:MaxTableColumns-len(columns)]...)
}
