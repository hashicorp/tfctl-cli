package format

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// MaxTableColumns is the maximum number of columns shown in horizontal table output.
const MaxTableColumns = 6

// ErrNotJSONAPI is the error returned by NewJSONAPIDisplayer when the raw data
// does not conform to the expected JSON:API shape.
var ErrNotJSONAPI = errors.New("not a JSON:API envelope")

// typeColumns is a mapping of API resource types to their preferred columns for horizontal table rendering.
var typeColumns = map[string][]string{
	"agent-pools":                 {"name", "organization-scoped", "agent-count"},
	"applies":                     {"status", "status-timestamps", "log-read-url"},
	"configuration-versions":      {"status", "speculative", "provisional"},
	"cost-estimates":              {"status", "delta-monthly-cost", "proposed-monthly-cost"},
	"notification-configurations": {"name", "destination-type", "enabled", "triggers"},
	"organization-memberships":    {"email", "status", "role"},
	"organizations":               {"name", "email", "external-id", "access-beta-tools", "stacks-enabled"},
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
	"workspaces":    {"actions"},
	"organizations": {"id"},
}

// JSONAPIDisplayer prepares responses within a JSON:API data envelope to be formatted.
type JSONAPIDisplayer struct {
	payload      any
	resourceType string
	collection   bool
}

// Check interface at compile time.
var _ Displayer = JSONAPIDisplayer{}

// DefaultFormat implements the Displayer interface.
func (d JSONAPIDisplayer) DefaultFormat() Format {
	if d.collection {
		return Table
	}
	return Pretty
}

// Payload implements the Displayer interface.
func (d JSONAPIDisplayer) Payload() any {
	return d.payload
}

// FieldTemplates implements the Displayer interface.
func (d JSONAPIDisplayer) FieldTemplates() []Field {
	rows := d.payload.([]map[string]any)

	var cols = make([]string, 0, 16)
	if d.DefaultFormat() == Table {
		cols = collectColumns(rows, typeColumns[d.resourceType], excludeColumns[d.resourceType])
	} else if len(rows) > 0 {
		cols = orderedFields(rows[0], typeColumns[d.resourceType])
	}

	if slices.Contains(excludeColumns[d.resourceType], "id") && len(cols) > 0 && cols[0] == "id" {
		cols = cols[1:]
	}

	result := make([]Field, len(cols))

	for i, att := range cols {
		name := att
		if att == "external-id" {
			name = "id"
		}
		result[i] = NewField(kebabToCapital(name), fmt.Sprintf(`{{ index . "%s" }}`, att))
	}

	return result
}

func kebabToCapital(input string) string {
	spaced := strings.ReplaceAll(input, "-", " ")

	caser := cases.Title(language.English)
	return caser.String(spaced)
}

// NewJSONAPIDisplayer creates a new displayer based on the contents of a JSON:API response body.
func NewJSONAPIDisplayer(raw []byte) (*JSONAPIDisplayer, error) {
	resourceType := ""
	collection := true
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrNotJSONAPI
	}

	data, ok := payload["data"]
	if !ok {
		return nil, ErrNotJSONAPI
	}

	var rows []map[string]any
	switch typed := data.(type) {
	case []any:
		for _, item := range typed {
			row, ok := resourceAsMap(item)
			if !ok {
				return nil, ErrNotJSONAPI
			}
			rows = append(rows, row)
		}
	case map[string]any:
		collection = false
		row, ok := resourceAsMap(typed)
		if !ok {
			return nil, ErrNotJSONAPI
		}
		rows = append(rows, row)
	default:
		return nil, ErrNotJSONAPI
	}

	if len(rows) > 0 {
		resourceType = stringValue(rows[0]["type"])
	}

	return &JSONAPIDisplayer{
		payload:      rows,
		resourceType: resourceType,
		collection:   collection,
	}, nil
}

func stringValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return v
	case bool, float64, int, int64:
		return fmt.Sprint(v)
	case json.Number:
		return v.String()
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(encoded)
	}
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
		row["type"] = kind
	}

	attrs, ok := obj["attributes"]
	if !ok {
		return row, len(row) > 0
	}
	attrMap, ok := attrs.(map[string]any)
	if !ok {
		return nil, false
	}
	for key, value := range attrMap {
		row[key] = value
	}
	return row, true
}

func orderedFields(row map[string]any, preferred []string) []string {
	seen := make(map[string]struct{}, len(row))
	ordered := make([]string, 0, len(row))
	if _, ok := row["id"]; ok {
		ordered = append(ordered, "id")
		seen["id"] = struct{}{}
	}
	for _, key := range preferred {
		if _, ok := row[key]; ok {
			ordered = append(ordered, key)
			seen[key] = struct{}{}
		}
	}

	remaining := make([]string, 0, len(row)-len(ordered))
	nested := make([]string, 0, len(row))
	typeTrailer := false
	for key := range row {
		if _, ok := seen[key]; ok {
			continue
		}
		if key == "type" {
			typeTrailer = true
			continue
		}
		if isNestedValue(row[key]) {
			nested = append(nested, key)
			continue
		}
		remaining = append(remaining, key)
	}
	sort.Strings(remaining)
	sort.Strings(nested)
	ordered = append(ordered, remaining...)
	ordered = append(ordered, nested...)
	if typeTrailer {
		ordered = append(ordered, "type")
	}
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
		if exclude == nil || !slices.Contains(exclude, key) {
			remaining = append(remaining, key)
		}
	}

	sort.Strings(remaining)
	if len(remaining) < MaxTableColumns-len(columns) {
		return append(columns, remaining...)
	}
	return append(columns, remaining[:MaxTableColumns-len(columns)]...)
}
