// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package render

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// MaxTableColumns is the maximum number of columns shown in horizontal table output.
const MaxTableColumns = 6

type rowValue struct {
	text    string
	styled  string
	visible int
}

var (
	ansiReset    = "\x1b[0m"
	ansiGreen    = "\x1b[32m"
	ansiRed      = "\x1b[31m"
	ansiMagenta  = "\x1b[35m"
	ansiBlueBold = "\x1b[1;94m"
)

// JSONAPITable renders JSON:API resource data as a human-readable table when possible.
func JSONAPITable(raw []byte) (string, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false, err
	}

	data, ok := payload["data"]
	if !ok {
		return "", false, nil
	}

	var rows []map[string]any
	switch typed := data.(type) {
	case []any:
		for _, item := range typed {
			row, ok := flattenResource(item)
			if !ok {
				return "", false, nil
			}
			rows = append(rows, row)
		}
	case map[string]any:
		row, ok := flattenResource(typed)
		if !ok {
			return "", false, nil
		}
		return renderVerticalTable(row, typeColumns[stringValue(row["type"])]), true, nil
	default:
		return "", false, nil
	}

	if len(rows) == 0 {
		return "", false, nil
	}

	resourceType := stringValue(rows[0]["type"])
	return renderHorizontalTable(rows, typeColumns[resourceType], excludeColumns[resourceType]), true, nil
}

func renderHorizontalTable(rows []map[string]any, preferred []string, exclude []string) string {
	columns := collectColumns(rows, preferred, exclude)
	headerValues := make([]rowValue, len(columns))
	widths := make([]int, len(columns))
	for i, col := range columns {
		headerValues[i] = formatLabel(col)
		widths[i] = headerValues[i].visible
	}

	renderedRows := make([][]rowValue, len(rows))
	for i, row := range rows {
		renderedRows[i] = make([]rowValue, len(columns))
		for j, col := range columns {
			value, ok := row[col]
			if !ok {
				renderedRows[i][j] = rowValue{}
				continue
			}
			value = summarizeNestedValue(value)
			rendered := formatScalar(value)
			renderedRows[i][j] = rendered
			if rendered.visible > widths[j] {
				widths[j] = rendered.visible
			}
		}
	}

	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, renderAlignedRow(headerValues, widths))
	for _, row := range renderedRows {
		lines = append(lines, renderAlignedRow(row, widths))
	}
	return strings.Join(lines, "\n")
}

func renderVerticalTable(row map[string]any, preferred []string) string {
	keys := orderedFields(row, preferred)
	width := 0
	for _, key := range keys {
		if len(key) > width {
			width = len(key)
		}
	}

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		value := row[key]
		label := formatLabel(key)
		if nested, ok := value.(map[string]any); ok {
			lines = append(lines, label.styled+strings.Repeat(" ", width-label.visible)+"  \\")
			lines = append(lines, renderNestedMap(nested)...)
			continue
		}
		if items, ok := value.([]any); ok {
			if len(items) == 0 {
				formatted := formatScalar(items)
				lines = append(lines, label.styled+strings.Repeat(" ", width-label.visible)+"  "+formatted.styled)
				continue
			}
			if isSimpleArray(items) {
				lines = append(lines, label.styled+strings.Repeat(" ", width-label.visible)+"  \\")
				lines = append(lines, renderSimpleArray(items)...)
				continue
			}
		}
		formatted := formatScalar(summarizeNestedValue(value))
		lines = append(lines, label.styled+strings.Repeat(" ", width-label.visible)+"  "+formatted.styled)
	}
	return strings.Join(lines, "\n")
}

func renderNestedMap(value map[string]any) []string {
	keys := sortedKeys(value)
	width := 0
	for _, key := range keys {
		if len(key) > width {
			width = len(key)
		}
	}

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		label := formatLabel(key)
		formatted := formatScalar(summarizeNestedValue(value[key]))
		lines = append(lines, "  "+label.styled+strings.Repeat(" ", width-label.visible)+"  "+formatted.styled)
	}
	return lines
}

func renderSimpleArray(value []any) []string {
	lines := make([]string, 0, len(value))
	for _, item := range value {
		formatted := formatScalar(item)
		lines = append(lines, " - "+formatted.styled)
	}
	return lines
}

func flattenResource(item any) (map[string]any, bool) {
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

func renderAlignedRow(values []rowValue, widths []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		padding := widths[i] - value.visible
		parts[i] = value.styled + strings.Repeat(" ", padding)
	}
	return strings.TrimRight(strings.Join(parts, "  "), " ")
}

func formatScalar(value any) rowValue {
	text := stringValue(value)
	styled := text
	switch v := value.(type) {
	case nil:
		styled = ansiRed + text + ansiReset
	case bool:
		if v {
			styled = ansiGreen + text + ansiReset
		} else {
			styled = ansiRed + text + ansiReset
		}
	case float64:
		if v > 0 {
			styled = ansiGreen + text + ansiReset
		} else {
			styled = ansiRed + text + ansiReset
		}
	case int:
		if v > 0 {
			styled = ansiGreen + text + ansiReset
		} else {
			styled = ansiRed + text + ansiReset
		}
	case int64:
		if v > 0 {
			styled = ansiGreen + text + ansiReset
		} else {
			styled = ansiRed + text + ansiReset
		}
	case json.Number:
		if numericValue(v.String()) > 0 {
			styled = ansiGreen + text + ansiReset
		} else {
			styled = ansiRed + text + ansiReset
		}
	case []any:
		if len(v) == 0 {
			styled = ansiRed + text + ansiReset
		}
	case string:
		if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
			styled = ansiMagenta + text + ansiReset
		}
	}
	return rowValue{text: text, styled: styled, visible: len(text)}
}

func formatLabel(text string) rowValue {
	return rowValue{text: text, styled: ansiBlueBold + text + ansiReset, visible: len(text)}
}

func numericValue(raw string) float64 {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return value
}

func summarizeNestedValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return "{...}"
	case []any:
		if len(typed) == 0 {
			return typed
		}
		return "[...]"
	default:
		return typed
	}
}

func isSimpleArray(value []any) bool {
	for _, item := range value {
		if isNestedValue(item) {
			return false
		}
	}
	return true
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

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		leftNested := isNestedValue(value[keys[i]])
		rightNested := isNestedValue(value[keys[j]])
		if leftNested != rightNested {
			return !leftNested
		}
		return keys[i] < keys[j]
	})
	return keys
}

func isNestedValue(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}
