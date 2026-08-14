// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/redact"
)

// The corpus below uses invented credentials only. Every value in mustNotAppear
// is either a published documentation example or a literal that says it is not
// real, so the corpus is safe to read, to share, and to paste into a bug report.
// Do not replace any of them with a value copied from a real response.
const (
	fakeAWSKeyID       = "AKIAIOSFODNN7EXAMPLE" // AWS publishes this one in its own docs.
	fakeTerraformToken = "EXAMPLEnotreal.atlasv1.notarealtokenvaluenotarealtokenvaluenotarealtoken00"
	fakeVaultToken     = "hvs.notarealtokenvalueEXAMPLE000000"
	fakeGitHubToken    = "ghp_notarealtokenvalueEXAMPLE0000000"
	fakeJWT            = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJleGFtcGxlIn0.notarealsignatureEXAMPLE"
	fakePassword       = "notarealpassword-EXAMPLE"
	fakeClientSecret   = "notarealclientsecret-EXAMPLE"
	fakeSignedURL      = "https://archivist.terraform.io/v1/object/EXAMPLE?X-Amz-Signature=notarealsignature"
	fakeUploadURL      = "https://archivist.terraform.io/v1/upload/EXAMPLE?X-Amz-Signature=notarealsignature"
	// The newlines are escaped rather than literal: this constant is embedded
	// into a JSON document below, and JSON strings cannot hold a raw newline.
	fakePrivateKey = `-----BEGIN RSA PRIVATE KEY-----\nTk9UQVJFQUxLRVlFWEFNUExF\n-----END RSA PRIVATE KEY-----`
)

// redactCase is one synthetic response and what must and must not survive it.
type redactCase struct {
	name string

	// envelope is a JSON:API response body. Exactly one of envelope or rawBody
	// is set.
	envelope string

	// rawBody is a response body that no displayer handles, such as plan JSON
	// output.
	rawBody string

	// mustNotAppear are the invented credentials. None of them may appear in any
	// output format.
	mustNotAppear []string

	// mustAppear are values a user needs to see. Masking them would make the
	// command useless, so over-masking fails the test as loudly as leaking.
	mustAppear []string

	// jqProbe is a filter that reaches straight for a credential. It must return
	// the placeholder, not the value.
	jqProbe string
}

func redactCorpus() []redactCase {
	return []redactCase{
		{
			name: "state version download URLs grant access to all of state",
			envelope: `{"data":{"id":"sv-EXAMPLE","type":"state-versions","attributes":{
				"serial": 42,
				"status": "finalized",
				"terraform-version": "1.9.8",
				"hosted-state-download-url": "` + fakeSignedURL + `",
				"hosted-json-state-download-url": "` + fakeSignedURL + `"
			}}}`,
			mustNotAppear: []string{fakeSignedURL, "notarealsignature"},
			mustAppear:    []string{"finalized", "1.9.8", "42"},
			jqProbe:       `.data.attributes["hosted-state-download-url"]`,
		},
		{
			name: "a created token is returned once in full",
			envelope: `{"data":{"id":"at-EXAMPLE","type":"authentication-tokens","attributes":{
				"description": "ci runner",
				"created-at": "2026-08-06T12:00:00Z",
				"token": "` + fakeTerraformToken + `"
			}}}`,
			mustNotAppear: []string{fakeTerraformToken, "notarealtokenvalue"},
			mustAppear:    []string{"ci runner"},
			jqProbe:       `.data.attributes.token`,
		},
		{
			name: "configuration version upload URL",
			envelope: `{"data":{"id":"cv-EXAMPLE","type":"configuration-versions","attributes":{
				"status": "pending",
				"speculative": false,
				"upload-url": "` + fakeUploadURL + `"
			}}}`,
			mustNotAppear: []string{fakeUploadURL},
			mustAppear:    []string{"pending"},
			jqProbe:       `.data.attributes["upload-url"]`,
		},
		{
			name: "variable the API declares sensitive but still returns",
			envelope: `{"data":[
				{"id":"var-1","type":"vars","attributes":{"key":"region","value":"us-east-1","sensitive":false,"category":"terraform"}},
				{"id":"var-2","type":"vars","attributes":{"key":"tls_cert","value":"` + fakePassword + `","sensitive":true,"category":"terraform"}}
			]}`,
			mustNotAppear: []string{fakePassword},
			mustAppear:    []string{"us-east-1", "region", "tls_cert"},
			jqProbe:       `.data[1].attributes.value`,
		},
		{
			name: "variable holding a secret that nobody marked sensitive",
			envelope: `{"data":[
				{"id":"var-3","type":"vars","attributes":{"key":"db_password","value":"` + fakePassword + `","sensitive":false,"category":"terraform"}},
				{"id":"var-4","type":"vars","attributes":{"key":"aws_access_key","value":"` + fakeAWSKeyID + `","sensitive":false,"category":"env"}},
				{"id":"var-5","type":"vars","attributes":{"key":"vault_login","value":"` + fakeVaultToken + `","sensitive":false,"category":"env"}},
				{"id":"var-6","type":"vars","attributes":{"key":"gh_pat","value":"` + fakeGitHubToken + `","sensitive":false,"category":"env"}},
				{"id":"var-7","type":"vars","attributes":{"key":"id_token","value":"` + fakeJWT + `","sensitive":false,"category":"env"}},
				{"id":"var-8","type":"vars","attributes":{"key":"deploy_key","value":"` + fakePrivateKey + `","sensitive":false,"category":"env"}}
			]}`,
			mustNotAppear: []string{
				fakePassword, fakeAWSKeyID, fakeVaultToken,
				fakeGitHubToken, fakeJWT, "Tk9UQVJFQUxLRVlFWEFNUExF",
			},
			mustAppear: []string{"db_password", "deploy_key"},
			jqProbe:    `.data[1].attributes.value`,
		},
		{
			name: "oauth client secret",
			envelope: `{"data":{"id":"oc-EXAMPLE","type":"oauth-clients","attributes":{
				"service-provider": "github",
				"http-url": "https://github.com",
				"secret": "` + fakeClientSecret + `",
				"client-secret": "` + fakeClientSecret + `"
			}}}`,
			mustNotAppear: []string{fakeClientSecret},
			mustAppear:    []string{"github"},
			jqProbe:       `.data.attributes.secret`,
		},
		{
			name: "plan and apply log read URLs",
			envelope: `{"data":{"id":"plan-EXAMPLE","type":"plans","attributes":{
				"status": "finished",
				"has-changes": true,
				"log-read-url": "` + fakeSignedURL + `"
			}}}`,
			mustNotAppear: []string{fakeSignedURL},
			mustAppear:    []string{"finished"},
			jqProbe:       `.data.attributes["log-read-url"]`,
		},
		{
			name: "a workspace must survive masking intact",
			envelope: `{"data":{"id":"ws-EXAMPLE","type":"workspaces","attributes":{
				"name": "example-workspace",
				"description": "nothing secret here",
				"execution-mode": "agent",
				"terraform-version": "1.9.8",
				"vcs-repo": {"identifier":"example/repo","oauth-token-id":"ot-EXAMPLE","branch":"main"},
				"created-at": "2026-08-06T12:00:00Z"
			}}}`,
			mustNotAppear: nil,
			// oauth-token-id names a credential, it is not one. Masking it would
			// break every workflow that needs the VCS connection.
			mustAppear: []string{"example-workspace", "ot-EXAMPLE", "example/repo", "agent", "main"},
		},
		{
			name: "plan JSON output is not a JSON:API envelope",
			rawBody: `{"format_version":"1.2","terraform_version":"1.9.8","variables":{
				"region":{"value":"us-east-1"},
				"db_password":{"value":"` + fakePassword + `"}
			},"planned_values":{"outputs":{"api_token":{"sensitive":true,"value":"` + fakeTerraformToken + `"}}}}`,
			mustNotAppear: []string{fakePassword, fakeTerraformToken},
			mustAppear:    []string{"us-east-1", "1.9.8"},
		},
	}
}

// renderedFormats returns every way a payload can reach stdout. A credential
// must not survive any of them. The default format is included because it is
// what a user sees when they pass no flags at all, which is how the state
// version URLs were being exposed.
func renderedFormats() map[string]format.Format {
	return map[string]format.Format{
		"default":  format.Unset,
		"json":     format.JSON,
		"agent":    format.Agent,
		"markdown": format.Markdown,
		"pretty":   format.Pretty,
	}
}

// TestRedactCorpus asserts the invariant that matters: for every synthetic
// response, in every output format, no invented credential reaches stdout, and
// everything a user legitimately needs still does.
//
// Run it with -v to see each rendering, which is the quickest way to judge
// whether masking is too aggressive.
func TestRedactCorpus(t *testing.T) {
	t.Parallel()

	for _, tc := range redactCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.rawBody != "" {
				runRawCase(t, tc)
				return
			}

			for formatName, f := range renderedFormats() {
				t.Run(formatName, func(t *testing.T) {
					r := require.New(t)

					io := iostreams.Test()
					out := format.New(io)
					out.SetRedactor(redact.New(redact.ModeStrict))
					if f != format.Unset {
						out.SetFormat(f)
					}

					disp, err := format.NewJSONAPIDisplayer([]byte(tc.envelope), hclog.Default())
					r.NoError(err)
					r.NoError(out.Display(disp))

					stdout := io.Output.String()
					t.Logf("%s output:\n%s", formatName, stdout)

					for _, secret := range tc.mustNotAppear {
						r.NotContains(stdout, secret, "a credential reached %s output", formatName)
					}

					// Table and pretty output truncate and wrap, so the
					// over-masking check runs against the machine formats.
					if f == format.JSON || f == format.Agent {
						for _, needed := range tc.mustAppear {
							r.Contains(stdout, needed, "masking removed a value the user needs")
						}
					}
				})
			}

			if tc.jqProbe != "" {
				t.Run("jq probe", func(t *testing.T) {
					r := require.New(t)

					io := iostreams.Test()
					out := format.New(io)
					out.SetRedactor(redact.New(redact.ModeStrict))
					out.SetFormat(format.JSON)
					out.SetJQFilter(tc.jqProbe)

					disp, err := format.NewJSONAPIDisplayer([]byte(tc.envelope), hclog.Default())
					r.NoError(err)
					r.NoError(out.Display(disp))

					stdout := strings.TrimSpace(io.Output.String())
					t.Logf("jq %s -> %s", tc.jqProbe, stdout)

					for _, secret := range tc.mustNotAppear {
						r.NotContains(stdout, secret, "a jq filter reached around the mask")
					}
					r.Contains(stdout, redact.Placeholder)
				})
			}
		})
	}
}

func runRawCase(t *testing.T, tc redactCase) {
	t.Helper()
	r := require.New(t)

	io := iostreams.Test()
	out := format.New(io)
	out.SetRedactor(redact.New(redact.ModeStrict))

	r.NoError(out.CopyRaw(strings.NewReader(tc.rawBody), "application/json"))

	stdout := io.Output.String()
	t.Logf("raw body output:\n%s", stdout)

	for _, secret := range tc.mustNotAppear {
		r.NotContains(stdout, secret, "a credential reached a raw body")
	}
	for _, needed := range tc.mustAppear {
		r.Contains(stdout, needed, "masking removed a value the user needs")
	}
}

// TestRedactCorpus_LeaksWithoutTheRedactor is the control. It proves the corpus
// actually carries the credentials it claims to, so a passing TestRedactCorpus
// cannot be the result of a corpus that was empty or misspelled.
func TestRedactCorpus_LeaksWithoutTheRedactor(t *testing.T) {
	t.Parallel()

	for _, tc := range redactCorpus() {
		if len(tc.mustNotAppear) == 0 {
			continue
		}

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			io := iostreams.Test()
			out := format.New(io)
			out.SetFormat(format.JSON)

			if tc.rawBody != "" {
				r.NoError(out.CopyRaw(strings.NewReader(tc.rawBody), "application/json"))
			} else {
				disp, err := format.NewJSONAPIDisplayer([]byte(tc.envelope), hclog.Default())
				r.NoError(err)
				r.NoError(out.Display(disp))
			}

			stdout := io.Output.String()
			for _, secret := range tc.mustNotAppear {
				r.Contains(stdout, secret,
					"the corpus does not actually contain %q, so the masking test proves nothing", secret)
			}
		})
	}
}
