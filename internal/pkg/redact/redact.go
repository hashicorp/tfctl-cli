// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package redact masks sensitive values in decoded API payloads before any
// output format renders them.
//
// This is an output filter, not an access-control boundary. The API decides
// what a token is permitted to read. Redaction limits what a permitted response
// leaves behind in a terminal, a shell history, a CI job log, or a coding-agent
// transcript. A response attribute that is a credential, or a signed URL that
// grants access to state, must not reach stdout by accident.
package redact

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Placeholder is the value written in place of a masked value. It follows the
// style that Profile.String uses for the stored token. Angle brackets are
// avoided on purpose: encoding/json escapes them, so an angle-bracket
// placeholder reaches JSON output as "<redacted>".
const Placeholder = "(redacted)"

// EnvRedact is the environment variable that controls the redaction mode.
const EnvRedact = "TFCTL_REDACT"

// Mode selects how aggressively values are masked.
type Mode int

const (
	// ModeStrict masks known secret fields, values the API declares sensitive,
	// and values whose name or shape indicates a credential. This is the
	// default.
	ModeStrict Mode = iota

	// ModeKnown masks only known secret fields and values the API declares
	// sensitive. Use it when a name or shape heuristic hides a value that is
	// needed.
	ModeKnown

	// ModeOff performs no masking.
	ModeOff
)

// String returns the canonical name of the mode.
func (m Mode) String() string {
	switch m {
	case ModeKnown:
		return "known"
	case ModeOff:
		return "off"
	default:
		return "strict"
	}
}

// ParseMode converts a configured value into a Mode.
func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "strict", "on", "true", "1":
		return ModeStrict, nil
	case "known":
		return ModeKnown, nil
	case "off", "false", "0", "disabled", "none":
		return ModeOff, nil
	}

	return ModeStrict, fmt.Errorf("invalid redact value %q. Must be one of \"strict\", \"known\", or \"off\"", value)
}

// ResolveMode determines the redaction mode. The --no-redact flag takes
// precedence over the environment variable, which takes precedence over the
// profile setting.
//
// Resolution order:
//  1. noRedact flag → ModeOff
//  2. TFCTL_REDACT
//  3. Profile redact
//  4. Otherwise → ModeStrict
func ResolveMode(noRedact bool, profileRedact string) (Mode, error) {
	if noRedact {
		return ModeOff, nil
	}

	if envValue := os.Getenv(EnvRedact); envValue != "" {
		mode, err := ParseMode(envValue)
		if err != nil {
			return ModeStrict, fmt.Errorf("%s: %w", EnvRedact, err)
		}
		return mode, nil
	}

	if profileRedact != "" {
		mode, err := ParseMode(profileRedact)
		if err != nil {
			return ModeStrict, fmt.Errorf("profile redact: %w", err)
		}
		return mode, nil
	}

	return ModeStrict, nil
}

// knownSecretFields are attribute names that hold a credential in the HCP
// Terraform and Terraform Enterprise API. The match is on the final segment of
// the attribute path and is case-insensitive.
var knownSecretFields = map[string]struct{}{
	// Returned once when a user, team, organization, or agent token is created.
	"token": {},
	// OAuth client secret and SSH key material.
	"secret":              {},
	"private-ssh-key":     {},
	"encryption-password": {},
	// Request headers, which a dry run reports back to the user.
	"authorization":       {},
	"proxy-authorization": {},
}

// capabilityURLFields are attribute-name fragments that indicate a signed URL.
// The URL is itself the credential: anyone holding it can read the object
// without a token. State downloads and plan or apply logs contain every value
// that Terraform wrote, whether or not the variable was marked sensitive.
var capabilityURLFields = []string{
	"download-url",
	"upload-url",
	"log-read-url",
}

// declaredSensitiveFields are the attribute names masked when the containing
// object declares itself sensitive with "sensitive": true. Variables and state
// version outputs use this shape.
var declaredSensitiveFields = map[string]struct{}{
	"value": {},
}

// sensitiveNameNeedles indicate a credential in a name. API attribute names
// are kebab-case and Terraform variable and output names are snake_case, so
// both forms are listed.
var sensitiveNameNeedles = []string{
	"secret",
	"token",
	"password",
	"passwd",
	"passphrase",
	"credential",
	"private-key",
	"private_key",
	"privatekey",
	"ssh-key",
	"ssh_key",
	"access-key",
	"access_key",
	"secret-key",
	"secret_key",
	"api-key",
	"api_key",
	"apikey",
	"signing-key",
	"signing_key",
}

// structuralNameSuffixes mark an attribute that describes a credential rather
// than holding one, such as "oauth-token-id" or "ssh-key-name". The name
// heuristic skips them.
var structuralNameSuffixes = []string{
	"-id",
	"_id",
	"-ids",
	"_ids",
	"-name",
	"_name",
	"-count",
	"-at",
}

// valueDetectors match values whose shape identifies a credential, which
// catches a secret held in an attribute that nobody marked sensitive.
var valueDetectors = []struct {
	reason string
	match  func(string) bool
}{
	{
		// Signed URLs for archivist, S3, or GCS. Presigned query parameters are
		// the credential.
		reason: "signed-url",
		match: func(s string) bool {
			if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
				return false
			}
			for _, marker := range []string{"X-Amz-Signature=", "X-Goog-Signature=", "&Signature=", "?Signature=", "archivist.terraform.io"} {
				if strings.Contains(s, marker) {
					return true
				}
			}
			return false
		},
	},
	{reason: "private-key", match: rePrivateKey.MatchString},
	{reason: "jwt", match: reJWT.MatchString},
	{reason: "terraform-token", match: reTerraformToken.MatchString},
	{reason: "vault-token", match: reVaultToken.MatchString},
	{reason: "github-token", match: reGitHubToken.MatchString},
	{reason: "aws-access-key-id", match: reAWSAccessKeyID.MatchString},
}

var (
	rePrivateKey     = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	reJWT            = regexp.MustCompile(`^eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}$`)
	reTerraformToken = regexp.MustCompile(`^[A-Za-z0-9]{14}\.(atlasv1|hcp)\.[A-Za-z0-9_-]{40,}$`)
	reVaultToken     = regexp.MustCompile(`^hv[sbr]\.[A-Za-z0-9_-]{20,}$`)
	reGitHubToken    = regexp.MustCompile(`^gh[pousr]_[A-Za-z0-9]{20,}$`)
	reAWSAccessKeyID = regexp.MustCompile(`^(AKIA|ASIA)[0-9A-Z]{16}$`)
)

// Redactor masks sensitive values in a decoded JSON tree. A Redactor records
// which fields it masked so that a command can tell the user what was hidden.
// It is not safe for concurrent use.
type Redactor struct {
	mode Mode

	// masked records the reason for each masked field name. Field names are
	// used instead of full paths so that masking the same field in a
	// collection, or in two views of one response, reports once.
	masked map[string]string
}

// New returns a Redactor for the given mode.
func New(mode Mode) *Redactor {
	return &Redactor{mode: mode, masked: map[string]string{}}
}

// Enabled reports whether the Redactor masks anything. A nil Redactor is
// disabled.
func (r *Redactor) Enabled() bool {
	return r != nil && r.mode != ModeOff
}

// Mode returns the configured mode.
func (r *Redactor) Mode() Mode {
	if r == nil {
		return ModeOff
	}
	return r.mode
}

// Count returns the number of distinct fields that were masked.
func (r *Redactor) Count() int {
	if r == nil {
		return 0
	}
	return len(r.masked)
}

// Fields returns the sorted names of the masked fields.
func (r *Redactor) Fields() []string {
	if r == nil {
		return nil
	}

	fields := make([]string, 0, len(r.masked))
	for field := range r.masked {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// Reasons returns the masked field names mapped to the rule that masked them.
// It is intended for debug logging.
func (r *Redactor) Reasons() map[string]string {
	if r == nil {
		return nil
	}

	reasons := make(map[string]string, len(r.masked))
	for field, reason := range r.masked {
		reasons[field] = reason
	}
	return reasons
}

// Apply returns the decoded JSON value with sensitive values masked. The input
// is never modified, so applying a Redactor to two views of one response is
// safe.
//
// Copying is on write. A value with nothing to mask is returned as it was
// received, without allocating, because most responses hold no credential at
// all and paying for a full copy of every response to mask nothing is not
// acceptable on a large body. Only the containers on the path to a masked value
// are rebuilt.
func (r *Redactor) Apply(value any) any {
	if !r.Enabled() {
		return value
	}

	masked, _ := r.walk("", value)
	return masked
}

// MaskHeader masks a header value when the header name or the value itself
// indicates a credential. It returns the value unchanged and false when the
// header carries nothing sensitive.
//
// Authorization is the obvious case, but a user can pass any header with
// --header, including a vendor API key header, so the same name and shape rules
// apply as for a response attribute.
func (r *Redactor) MaskHeader(name, value string) (string, bool) {
	if !r.Enabled() || value == "" {
		return value, false
	}

	if reason, ok := r.matchKey(name, value, false, ""); ok {
		return r.mask(strings.ToLower(name), reason), true
	}

	if reason, ok := r.matchValue(value); ok {
		return r.mask(strings.ToLower(name), reason), true
	}

	return value, false
}

// ApplyRow masks a flattened display row. Row keys can be dot-separated paths,
// and the final segment is used for name matching.
func (r *Redactor) ApplyRow(row map[string]any) map[string]any {
	if !r.Enabled() {
		return row
	}

	masked, ok := r.Apply(row).(map[string]any)
	if !ok {
		return row
	}
	return masked
}

// walk masks what must not be shown and reports whether it changed anything.
//
// The returned value is the input itself when nothing was masked, so a subtree
// with no credential in it costs no allocation. When something is masked, only
// the containers between the root and that value are rebuilt; every untouched
// subtree is shared with the input. The input is never written to.
//
// name is the key under which this value sits. It serves two purposes: it names
// the field in the report, and it tells an object that does not label its own
// value what that value is called. A variable object carries its own name in a
// "key" attribute, but Terraform plan JSON nests the name as the map key, as in
// variables.db_password.value, and both forms have to reach the "value"
// attribute below them.
//
// Every unchanged path returns value rather than typed. That is not a stylistic
// choice: value is already an interface, while returning typed re-boxes it, and
// boxing a string or a slice header allocates. Returning typed costs one
// allocation per leaf, which is the whole cost this function exists to avoid.
// TestApplyReturnsTheInputWhenNothingIsMasked pins it at zero.
func (r *Redactor) walk(name string, value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		declared := declaresSensitive(typed)

		hint := nameHint(typed)
		if hint == "" {
			hint = name
		}

		// out stays nil until the first change, at which point the whole map is
		// shallow-copied. Everything iterated before that point was unchanged,
		// so the copy is correct, and the changed entry is written immediately
		// after.
		var out map[string]any

		for key, child := range typed {
			var (
				masked  any
				changed bool
			)

			if reason, ok := r.matchKey(key, child, declared, hint); ok {
				masked, changed = r.mask(key, reason), true
			} else {
				masked, changed = r.walk(key, child)
			}

			if !changed {
				continue
			}

			if out == nil {
				out = make(map[string]any, len(typed))
				for k, v := range typed {
					out[k] = v
				}
			}
			out[key] = masked
		}

		if out == nil {
			return value, false
		}
		return out, true
	case []any:
		// Elements inherit the name of the attribute holding the list, which is
		// what a reader needs to see reported. The index is not a field name.
		var out []any

		for i, item := range typed {
			masked, changed := r.walk(name, item)
			if !changed {
				continue
			}

			if out == nil {
				out = make([]any, len(typed))
				copy(out, typed)
			}
			out[i] = masked
		}

		if out == nil {
			return value, false
		}
		return out, true
	case string:
		if reason, ok := r.matchValue(typed); ok {
			return r.mask(name, reason), true
		}
		return value, false
	default:
		return value, false
	}
}

// matchKey reports whether an attribute must be masked because of its name.
// nameHint carries the name that a self-describing object gives itself, which
// is the variable key or output name for a "value" attribute.
func (r *Redactor) matchKey(key string, value any, declaredSensitive bool, nameHint string) (string, bool) {
	// A value the server did not send cannot leak. Leave null and empty values
	// alone so that output still shows the server actually withheld them.
	if value == nil {
		return "", false
	}
	if str, ok := value.(string); ok && str == "" {
		return "", false
	}

	name := strings.ToLower(lastSegment(key))
	_, holdsDeclaredValue := declaredSensitiveFields[name]

	if declaredSensitive && holdsDeclaredValue {
		return "declared-sensitive", true
	}

	if _, ok := knownSecretFields[name]; ok {
		return "known-secret-field", true
	}

	for _, fragment := range capabilityURLFields {
		if strings.Contains(name, fragment) {
			return "capability-url", true
		}
	}

	if r.mode != ModeStrict {
		return "", false
	}

	// Only string values are masked by the name heuristic. A boolean or number
	// is not a credential, and masking "sensitive": true would hide the very
	// marker that drives the declared-sensitive rule.
	if _, ok := value.(string); !ok {
		return "", false
	}

	if looksSensitiveName(name) {
		return "sensitive-field-name", true
	}

	// A variable or output holds its own name next to its value, or is nested
	// under it. The attribute is always called "value", so the name that
	// indicates a credential is the one the enclosing structure gives it. This
	// catches a variable that holds a secret but that nobody marked sensitive.
	if holdsDeclaredValue && nameHint != "" && looksSensitiveName(strings.ToLower(nameHint)) {
		return "sensitive-object-name", true
	}

	return "", false
}

// matchValue reports whether a value must be masked because of its shape.
func (r *Redactor) matchValue(value string) (string, bool) {
	if r.mode != ModeStrict || value == "" {
		return "", false
	}

	for _, detector := range valueDetectors {
		if detector.match(value) {
			return detector.reason, true
		}
	}

	return "", false
}

func (r *Redactor) mask(field, reason string) string {
	if field == "" {
		field = "(value)"
	}
	if _, seen := r.masked[field]; !seen {
		r.masked[field] = reason
	}
	return Placeholder
}

func looksSensitiveName(name string) bool {
	for _, suffix := range structuralNameSuffixes {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}

	for _, needle := range sensitiveNameNeedles {
		if strings.Contains(name, needle) {
			return true
		}
	}

	return false
}

// declaresSensitive reports whether an object marks its own value sensitive.
func declaresSensitive(object map[string]any) bool {
	sensitive, ok := object["sensitive"].(bool)
	return ok && sensitive
}

// nameHint returns the name that an object reports for itself. Variables use
// "key" and state version outputs use "name".
func nameHint(object map[string]any) string {
	for _, field := range []string{"key", "name"} {
		if value, ok := object[field].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func lastSegment(path string) string {
	if index := strings.LastIndex(path, "."); index >= 0 {
		return path[index+1:]
	}
	return path
}
