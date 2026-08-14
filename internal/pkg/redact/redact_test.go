// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package redact

import (
	"encoding/json"
	"reflect"
	"testing"
)

// applyJSON runs a Redactor over a JSON document and returns the result as
// JSON, which keeps the test cases readable.
func applyJSON(t *testing.T, mode Mode, document string) (string, *Redactor) {
	t.Helper()

	var decoded any
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("test document is not valid JSON: %v", err)
	}

	r := New(mode)
	masked, err := json.Marshal(r.Apply(decoded))
	if err != nil {
		t.Fatalf("could not marshal masked document: %v", err)
	}

	return string(masked), r
}

func TestApply(t *testing.T) {
	tests := []struct {
		name     string
		mode     Mode
		document string
		want     string
	}{
		{
			name:     "state version download URLs are capability URLs",
			mode:     ModeStrict,
			document: `{"data":{"type":"state-versions","attributes":{"serial":7,"hosted-state-download-url":"https://archivist.terraform.io/v1/object/abc","hosted-json-state-download-url":"https://archivist.terraform.io/v1/object/def"}}}`,
			want:     `{"data":{"attributes":{"hosted-json-state-download-url":"(redacted)","hosted-state-download-url":"(redacted)","serial":7},"type":"state-versions"}}`,
		},
		{
			name:     "capability URLs are masked in the known mode",
			mode:     ModeKnown,
			document: `{"data":{"attributes":{"hosted-state-download-url":"https://archivist.terraform.io/v1/object/abc"}}}`,
			want:     `{"data":{"attributes":{"hosted-state-download-url":"(redacted)"}}}`,
		},
		{
			name:     "configuration version upload URL",
			mode:     ModeKnown,
			document: `{"data":{"attributes":{"upload-url":"https://archivist.terraform.io/v1/object/ghi","status":"pending"}}}`,
			want:     `{"data":{"attributes":{"status":"pending","upload-url":"(redacted)"}}}`,
		},
		{
			name:     "created token is a known secret field",
			mode:     ModeKnown,
			document: `{"data":{"type":"authentication-tokens","attributes":{"description":"ci","token":"abcdefghijklmn.atlasv1.zzz"}}}`,
			want:     `{"data":{"attributes":{"description":"ci","token":"(redacted)"},"type":"authentication-tokens"}}`,
		},
		{
			name:     "declared sensitive value",
			mode:     ModeKnown,
			document: `{"data":{"attributes":{"key":"harmless","value":"visible-secret","sensitive":true}}}`,
			want:     `{"data":{"attributes":{"key":"harmless","sensitive":true,"value":"(redacted)"}}}`,
		},
		{
			name:     "value the server already withheld stays null",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"key":"db_password","value":null,"sensitive":true}}}`,
			want:     `{"data":{"attributes":{"key":"db_password","sensitive":true,"value":null}}}`,
		},
		{
			name:     "variable that nobody marked sensitive is masked by its own name",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"key":"db_password","value":"hunter2","sensitive":false,"category":"terraform"}}}`,
			want:     `{"data":{"attributes":{"category":"terraform","key":"db_password","sensitive":false,"value":"(redacted)"}}}`,
		},
		{
			name:     "the known mode does not apply the name heuristic",
			mode:     ModeKnown,
			document: `{"data":{"attributes":{"key":"db_password","value":"hunter2","sensitive":false}}}`,
			want:     `{"data":{"attributes":{"key":"db_password","sensitive":false,"value":"hunter2"}}}`,
		},
		{
			name:     "attribute name that indicates a credential",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"client-secret":"shhhh","service-provider":"github"}}}`,
			want:     `{"data":{"attributes":{"client-secret":"(redacted)","service-provider":"github"}}}`,
		},
		{
			name:     "identifier that only describes a credential is kept",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"vcs-repo":{"oauth-token-id":"ot-abc123","identifier":"my-org/my-repo"}}}}`,
			want:     `{"data":{"attributes":{"vcs-repo":{"identifier":"my-org/my-repo","oauth-token-id":"ot-abc123"}}}}`,
		},
		{
			name:     "sensitive marker itself is never masked",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"sensitive":true,"name":"vpc_id","value":"vpc-123"}}}`,
			want:     `{"data":{"attributes":{"name":"vpc_id","sensitive":true,"value":"(redacted)"}}}`,
		},
		{
			name:     "private key shape in an unremarkable attribute",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"description":"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----"}}}`,
			want:     `{"data":{"attributes":{"description":"(redacted)"}}}`,
		},
		{
			name:     "presigned URL shape in an unremarkable attribute",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"notification-url":"https://example.s3.amazonaws.com/x?X-Amz-Signature=deadbeef"}}}`,
			want:     `{"data":{"attributes":{"notification-url":"(redacted)"}}}`,
		},
		{
			name:     "terraform token shape",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"note":"abcdefghijklmn.atlasv1.0123456789012345678901234567890123456789012345"}}}`,
			want:     `{"data":{"attributes":{"note":"(redacted)"}}}`,
		},
		{
			name:     "ordinary URL is kept",
			mode:     ModeStrict,
			document: `{"data":{"attributes":{"vcs-commit-url":"https://github.com/my-org/my-repo/commit/abc"}}}`,
			want:     `{"data":{"attributes":{"vcs-commit-url":"https://github.com/my-org/my-repo/commit/abc"}}}`,
		},
		{
			name:     "values inside a collection",
			mode:     ModeStrict,
			document: `{"data":[{"attributes":{"key":"a","value":"1","sensitive":false}},{"attributes":{"key":"api_key","value":"2","sensitive":false}}]}`,
			want:     `{"data":[{"attributes":{"key":"a","sensitive":false,"value":"1"}},{"attributes":{"key":"api_key","sensitive":false,"value":"(redacted)"}}]}`,
		},
		{
			name:     "the off mode changes nothing",
			mode:     ModeOff,
			document: `{"data":{"attributes":{"token":"abcdefghijklmn.atlasv1.zzz"}}}`,
			want:     `{"data":{"attributes":{"token":"abcdefghijklmn.atlasv1.zzz"}}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := applyJSON(t, tc.mode, tc.document)
			if got != tc.want {
				t.Errorf("Apply() mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestApplyDoesNotModifyTheInput(t *testing.T) {
	document := `{"data":{"attributes":{"hosted-state-download-url":"https://archivist.terraform.io/v1/object/abc"}}}`

	var decoded any
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("test document is not valid JSON: %v", err)
	}

	r := New(ModeStrict)
	_ = r.Apply(decoded)

	after, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("could not marshal the original document: %v", err)
	}

	if string(after) != document {
		t.Errorf("Apply() modified its input\n got: %s\nwant: %s", after, document)
	}
}

// mapPointer identifies the backing store of a map or slice so a test can tell
// a shared reference from a copy.
func mapPointer(t *testing.T, value any) uintptr {
	t.Helper()

	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map && rv.Kind() != reflect.Slice {
		t.Fatalf("value is a %s, not a map or slice", rv.Kind())
	}
	return rv.Pointer()
}

func mustDecode(t *testing.T, document string) any {
	t.Helper()

	var decoded any
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("test document is not valid JSON: %v", err)
	}
	return decoded
}

func TestApplyReturnsTheInputWhenNothingIsMasked(t *testing.T) {
	// Copying every response in order to mask nothing is the common case, and on
	// a large plan JSON document it is also the expensive case. Nothing to mask
	// has to mean nothing to copy.
	decoded := mustDecode(t, `{"data":{"id":"ws-1","type":"workspaces","attributes":{
		"name":"example","execution-mode":"agent",
		"vcs-repo":{"identifier":"my-org/my-repo","oauth-token-id":"ot-abc"}
	}}}`)

	r := New(ModeStrict)
	masked := r.Apply(decoded)

	if r.Count() != 0 {
		t.Fatalf("Count() = %d, want 0; this document is supposed to be clean", r.Count())
	}
	if mapPointer(t, masked) != mapPointer(t, decoded) {
		t.Error("Apply() copied a document that had nothing to mask")
	}

	// Measure the walk alone. Constructing a Redactor allocates a small fixed
	// amount, which would hide the number that matters. Repeating the walk over a
	// clean document records nothing, so reusing one Redactor is safe here.
	allocs := testing.AllocsPerRun(5, func() {
		_ = r.Apply(decoded)
	})
	if allocs != 0 {
		t.Errorf("Apply() over a clean document made %.0f allocations, want 0", allocs)
	}
}

func TestApplySharesUntouchedSubtrees(t *testing.T) {
	// When something is masked, only the containers between the root and the
	// masked value are rebuilt. A sibling subtree must be shared, not copied.
	decoded := mustDecode(t, `{"data":{"attributes":{
		"token":"abcdefghijklmn.atlasv1.zzz",
		"vcs-repo":{"identifier":"my-org/my-repo","oauth-token-id":"ot-abc"}
	}}}`)

	r := New(ModeStrict)
	masked := r.Apply(decoded)

	if r.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", r.Count())
	}

	attributesOf := func(root any) map[string]any {
		return root.(map[string]any)["data"].(map[string]any)["attributes"].(map[string]any)
	}

	original := attributesOf(decoded)
	rebuilt := attributesOf(masked)

	if mapPointer(t, rebuilt) == mapPointer(t, original) {
		t.Error("the container holding the masked value was not rebuilt, so the input was written to")
	}
	if mapPointer(t, rebuilt["vcs-repo"]) != mapPointer(t, original["vcs-repo"]) {
		t.Error("an untouched sibling subtree was copied instead of shared")
	}
	if original["token"] == Placeholder {
		t.Error("Apply() masked its input in place")
	}
}

func TestApplyReportsTheAttributeNameForAListElement(t *testing.T) {
	// A masked element inside a list is reported under the attribute that holds
	// the list. An index is not a field name and tells the reader nothing.
	decoded := mustDecode(t, `{"data":{"attributes":{"deploy-keys":["harmless","hvs.notarealtokenvalueEXAMPLE00000"]}}}`)

	r := New(ModeStrict)
	_ = r.Apply(decoded)

	want := []string{"deploy-keys"}
	if !reflect.DeepEqual(r.Fields(), want) {
		t.Errorf("Fields() = %v, want %v", r.Fields(), want)
	}
}

func TestApplyIsRepeatableAcrossViews(t *testing.T) {
	// A JSON:API response is rendered from two views of one payload: the
	// envelope for JSON output and flattened rows for table output. Masking
	// both must report one field, not two.
	envelope := `{"data":{"attributes":{"hosted-state-download-url":"https://archivist.terraform.io/v1/object/abc"}}}`

	var decoded any
	if err := json.Unmarshal([]byte(envelope), &decoded); err != nil {
		t.Fatalf("test document is not valid JSON: %v", err)
	}

	r := New(ModeStrict)
	_ = r.Apply(decoded)
	_ = r.ApplyRow(map[string]any{"hosted-state-download-url": "https://archivist.terraform.io/v1/object/abc"})

	if r.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (fields must report once across views)", r.Count())
	}

	want := []string{"hosted-state-download-url"}
	if !reflect.DeepEqual(r.Fields(), want) {
		t.Errorf("Fields() = %v, want %v", r.Fields(), want)
	}
}

func TestApplyRowUsesTheFinalPathSegment(t *testing.T) {
	// Flattened display rows use dot-separated keys.
	r := New(ModeStrict)
	row := r.ApplyRow(map[string]any{
		"vcs-repo.oauth-token-id": "ot-abc123",
		"vcs-repo.webhook-secret": "shhhh",
	})

	if row["vcs-repo.oauth-token-id"] != "ot-abc123" {
		t.Errorf("oauth-token-id was masked, want it kept: %v", row["vcs-repo.oauth-token-id"])
	}
	if row["vcs-repo.webhook-secret"] != Placeholder {
		t.Errorf("webhook-secret = %v, want %s", row["vcs-repo.webhook-secret"], Placeholder)
	}
}

func TestReasons(t *testing.T) {
	_, r := applyJSON(t, ModeStrict, `{"data":{"attributes":{"key":"db_password","value":"hunter2","sensitive":false,"hosted-state-download-url":"https://archivist.terraform.io/v1/object/abc"}}}`)

	want := map[string]string{
		"value":                     "sensitive-object-name",
		"hosted-state-download-url": "capability-url",
	}

	if !reflect.DeepEqual(r.Reasons(), want) {
		t.Errorf("Reasons() = %v, want %v", r.Reasons(), want)
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		input   string
		want    Mode
		wantErr bool
	}{
		{input: "", want: ModeStrict},
		{input: "strict", want: ModeStrict},
		{input: "on", want: ModeStrict},
		{input: "true", want: ModeStrict},
		{input: "known", want: ModeKnown},
		{input: "off", want: ModeOff},
		{input: "false", want: ModeOff},
		{input: "disabled", want: ModeOff},
		{input: " OFF ", want: ModeOff},
		{input: "banana", want: ModeStrict, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseMode(tc.input)
			if tc.wantErr != (err != nil) {
				t.Fatalf("ParseMode(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("ParseMode(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name     string
		noRedact bool
		env      string
		profile  string
		want     Mode
		wantErr  bool
	}{
		{name: "default is strict", want: ModeStrict},
		{name: "flag wins over everything", noRedact: true, env: "strict", profile: "strict", want: ModeOff},
		{name: "environment wins over the profile", env: "known", profile: "off", want: ModeKnown},
		{name: "profile applies when the environment is unset", profile: "off", want: ModeOff},
		{name: "invalid environment value is an error", env: "banana", want: ModeStrict, wantErr: true},
		{name: "invalid profile value is an error", profile: "banana", want: ModeStrict, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvRedact, tc.env)

			got, err := ResolveMode(tc.noRedact, tc.profile)
			if tc.wantErr != (err != nil) {
				t.Fatalf("ResolveMode() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("ResolveMode() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNilRedactorIsDisabled(t *testing.T) {
	var r *Redactor

	if r.Enabled() {
		t.Error("Enabled() = true, want false for a nil Redactor")
	}
	if got := r.Apply("anything"); got != "anything" {
		t.Errorf("Apply() = %v, want the value unchanged", got)
	}
	if r.Count() != 0 {
		t.Errorf("Count() = %d, want 0", r.Count())
	}
}
