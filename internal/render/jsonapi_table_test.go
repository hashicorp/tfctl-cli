package render

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func hasBlueBoldANSI(s string) bool {
	return strings.Contains(s, "\x1b[1;") && strings.Contains(s, "94m")
}

func TestJSONAPITable(t *testing.T) {
	t.Parallel()

	t.Run("renders horizontal table with aligned preferred columns and colored values", func(t *testing.T) {
		body := []byte(`{
			"data": [
				{"id":"ws-1","type":"workspaces","attributes":{"name":"alpha","description":"one","locked":true,"resource-count":3}},
				{"id":"ws-2","type":"workspaces","attributes":{"name":"beta","locked":false,"resource-count":0}}
			]
		}`)

		table, ok, err := JSONAPITable(body)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected table output")
		}

		lines := strings.Split(table, "\n")
		if len(lines) != 3 {
			t.Fatalf("got %d lines, want 3: %q", len(lines), table)
		}

		plainLines := make([]string, len(lines))
		for i, line := range lines {
			plainLines[i] = stripANSI(line)
		}

		for i, want := range []string{
			"id    name   description  locked  resource-count  type",
			"ws-1  alpha  one          true    3               workspaces",
			"ws-2  beta                false   0               workspaces",
		} {
			if strings.TrimRight(plainLines[i], " ") != want {
				t.Fatalf("line %d = %q, want %q", i+1, plainLines[i], want)
			}
		}

		headerStarts := fieldStarts(plainLines[0])
		rowStarts := fieldStarts(plainLines[1])
		if !reflect.DeepEqual(headerStarts, rowStarts) {
			t.Fatalf("header columns %v do not align with row columns %v", headerStarts, rowStarts)
		}

		if !strings.Contains(lines[1], "\x1b[") || !strings.Contains(lines[2], "\x1b[") {
			t.Fatalf("expected colored values in rows: %q", table)
		}
		if !hasBlueBoldANSI(lines[0]) {
			t.Fatalf("expected styled header row: %q", lines[0])
		}
	})

	t.Run("renders single resource vertically with aligned values and no header formatting", func(t *testing.T) {
		body := []byte(`{
			"data": {
				"id":"run-1",
				"type":"runs",
				"attributes":{
					"status":"planned_and_finished",
					"message":"deploy app",
					"has-changes":true,
					"metadata":{"source":"cli"}
				}
			}
		}`)

		table, ok, err := JSONAPITable(body)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected table output")
		}

		lines := strings.Split(table, "\n")
		plainLines := make([]string, len(lines))
		for i, line := range lines {
			plainLines[i] = stripANSI(line)
		}

		for i, want := range []string{
			"id           run-1",
			"message      deploy app",
			"status       planned_and_finished",
			"has-changes  true",
			"metadata     \\",
			"  source  cli",
			"type         runs",
		} {
			if i >= len(plainLines) {
				t.Fatalf("missing line %d in %q", i+1, table)
			}
			if strings.TrimRight(plainLines[i], " ") != want {
				t.Fatalf("line %d = %q, want %q", i+1, plainLines[i], want)
			}
		}

		if !hasBlueBoldANSI(lines[0]) {
			t.Fatalf("expected styled vertical label: %q", lines[0])
		}
		if !strings.Contains(lines[3], "\x1b[") {
			t.Fatalf("expected colored boolean value: %q", lines[3])
		}
		if !hasBlueBoldANSI(lines[4]) {
			t.Fatalf("expected styled nested label row: %q", lines[4])
		}

		valueStarts := []int{}
		for _, line := range []string{plainLines[0], plainLines[1], plainLines[2], plainLines[3], plainLines[6]} {
			valueStarts = append(valueStarts, valueStart(line))
		}
		for i := 1; i < len(valueStarts); i++ {
			if valueStarts[i] != valueStarts[0] {
				t.Fatalf("expected aligned vertical values, got starts %v", valueStarts)
			}
		}
	})

	t.Run("renders unknown single resource in stable fallback order", func(t *testing.T) {
		body := []byte(`{
			"data": {
				"id":"thing-1",
				"type":"widgets",
				"attributes":{"beta":"two","alpha":"one"}
			}
		}`)

		table, ok, err := JSONAPITable(body)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected table output")
		}

		got := strings.Split(stripANSI(table), "\n")
		for i, want := range []string{"id     thing-1", "alpha  one", "beta   two", "type   widgets"} {
			if got[i] != want {
				t.Fatalf("line %d = %q, want %q", i+1, got[i], want)
			}
		}
	})

	t.Run("renders single resource with id and type only", func(t *testing.T) {
		body := []byte(`{"data":{"id":"org-1","type":"organizations"}}`)

		table, ok, err := JSONAPITable(body)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected table output")
		}

		got := strings.Split(stripANSI(table), "\n")
		want := []string{"id    org-1", "type  organizations"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("renders top level scalar arrays vertically and summarizes nested arrays", func(t *testing.T) {
		body := []byte(`{
			"data": {
				"id":"ws-1",
				"type":"workspaces",
				"attributes":{
					"empty-values":[],
					"tag-names":["foo","bar","baz"],
					"structured-values":[{"name":"foo"}],
					"nested-scalars":[["foo"]]
				}
			}
		}`)

		table, ok, err := JSONAPITable(body)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected table output")
		}

		lines := strings.Split(table, "\n")
		got := strings.Split(stripANSI(table), "\n")
		want := []string{
			"id                 ws-1",
			"empty-values       []",
			"nested-scalars     [...]",
			"structured-values  [...]",
			"tag-names          \\",
			" - foo",
			" - bar",
			" - baz",
			"type               workspaces",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		if !strings.Contains(lines[1], "\x1b[31m[]\x1b[0m") {
			t.Fatalf("expected empty array to be red: %q", table)
		}
	})

	t.Run("renders nested json values one level deep and summarizes deeper nesting", func(t *testing.T) {
		body := []byte(`{
			"data": {
				"id":"user-1",
				"type":"users",
				"attributes":{
					"password":null,
					"permissions":{
						"can-change-email":false,
						"can-change-password":false,
						"can-change-username":false,
						"can-manage-hcp-account":false,
						"account-permissions":{"billing":true}
					},
					"two-factor":{
						"enabled":true,
						"verified":true
					}
				}
			}
		}`)

		table, ok, err := JSONAPITable(body)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected table output")
		}

		got := strings.Split(stripANSI(table), "\n")
		want := []string{
			"id           user-1",
			"password     null",
			"permissions  \\",
			"  can-change-email        false",
			"  can-change-password     false",
			"  can-change-username     false",
			"  can-manage-hcp-account  false",
			"  account-permissions     {...}",
			"two-factor   \\",
			"  enabled   true",
			"  verified  true",
			"type         users",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		if !strings.Contains(table, "\x1b[31mnull\x1b[0m") {
			t.Fatalf("expected null value to be red: %q", table)
		}

		nestedStarts := []int{}
		for _, line := range []string{got[3], got[4], got[5], got[6], got[7]} {
			nestedStarts = append(nestedStarts, valueStart(line))
		}
		for i := 1; i < len(nestedStarts); i++ {
			if nestedStarts[i] != nestedStarts[0] {
				t.Fatalf("expected aligned nested values, got starts %v", nestedStarts)
			}
		}
	})

	t.Run("returns false for empty payload or non jsonapi shape", func(t *testing.T) {
		for _, body := range [][]byte{[]byte(`{}`), []byte(`{"errors":[{"title":"bad"}]}`)} {
			table, ok, err := JSONAPITable(body)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				t.Fatalf("expected ok=false, got table %q", table)
			}
		}
	})

	t.Run("uses preferred columns for additional resource types", func(t *testing.T) {
		cases := map[string]struct {
			body []byte
			want []string
		}{
			"projects": {
				body: []byte(`{"data":[{"id":"prj-1","type":"projects","attributes":{"name":"core","description":"shared","organization-name":"acme","irrelevant":"x"}}]}`),
				want: []string{"id", "name", "description", "organization-name"},
			},
			"organizations": {
				body: []byte(`{"data":[{"id":"org-1","type":"organizations","attributes":{"name":"acme","email":"ops@example.com","external-id":"ext-1","irrelevant":"x"}}]}`),
				want: []string{"id", "name", "email", "external-id"},
			},
			"vars": {
				body: []byte(`{"data":[{"id":"var-1","type":"vars","attributes":{"key":"AWS_REGION","value":"us-east-1","category":"env","hcl":false,"irrelevant":"x"}}]}`),
				want: []string{"id", "key", "value", "category", "hcl"},
			},
		}

		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				table, ok, err := JSONAPITable(tc.body)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Fatal("expected table output")
				}

				headers := strings.Fields(stripANSI(strings.Split(table, "\n")[0]))
				if !reflect.DeepEqual(headers[:len(tc.want)], tc.want) {
					t.Fatalf("headers = %#v, want prefix %#v", headers, tc.want)
				}
			})
		}
	})
}

func fieldStarts(line string) []int {
	starts := []int{}
	inField := false
	for i, r := range line {
		if r != ' ' && !inField {
			starts = append(starts, i)
			inField = true
			continue
		}
		if r == ' ' {
			inField = false
		}
	}
	return starts
}

func valueStart(line string) int {
	seenGap := false
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			seenGap = true
			continue
		}
		if seenGap {
			return i
		}
	}
	return -1
}
