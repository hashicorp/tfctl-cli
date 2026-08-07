// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/redact"
)

// These tests drive the api command against a fake Terraform Enterprise so that
// the command path is covered, not only the redactor. The credentials are
// invented: a signed URL with a fake signature, and a token that says it is not
// real.
const (
	fakeStateDownloadURL = "https://archivist.terraform.io/v1/object/EXAMPLE?X-Amz-Signature=notarealsignature"
	fakeCreatedToken     = "EXAMPLEnotreal.atlasv1.notarealtokenvaluenotarealtokenvaluenotarealtoken00"
)

func TestRunAPI_MasksSensitiveValuesByDefault(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/state-versions/sv-EXAMPLE": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONAPIResponse(w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"id":   "sv-EXAMPLE",
					"type": "state-versions",
					"attributes": map[string]any{
						"serial":                    42,
						"status":                    "finalized",
						"hosted-state-download-url": fakeStateDownloadURL,
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	opts := newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/state-versions/sv-EXAMPLE")
	})
	opts.Output.SetRedactor(redact.New(redact.ModeStrict))

	require.NoError(t, RunAPI(context.Background(), opts))

	require.NotContains(t, io.Output.String(), fakeStateDownloadURL)
	require.NotContains(t, io.Output.String(), "notarealsignature")
	require.Contains(t, io.Output.String(), redact.Placeholder)
	require.Contains(t, io.Output.String(), "finalized")
	require.Contains(t, io.Error.String(), "hosted-state-download-url")
}

func TestRunAPI_NoRedactShowsSensitiveValues(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/organizations/example/authentication-tokens": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id":   "at-EXAMPLE",
					"type": "authentication-tokens",
					"attributes": map[string]any{
						"description": "ci runner",
						"token":       fakeCreatedToken,
					},
				},
			})
		},
	})
	defer server.Close()

	// A token is returned once. Masking it by default is right, and being unable
	// to retrieve it at all would make the command useless, so the escape hatch
	// has to work.
	io := iostreams.Test()
	opts := newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/organizations/example/authentication-tokens")
		opts.Method = http.MethodPost
	})
	opts.Output.SetRedactor(redact.New(redact.ModeOff))

	require.NoError(t, RunAPI(context.Background(), opts))
	require.Contains(t, io.Output.String(), fakeCreatedToken)
}

func TestRunAPI_MasksCreatedToken(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"POST /api/v2/organizations/example/authentication-tokens": func(w http.ResponseWriter, _ *http.Request) {
			writeJSONAPIResponse(w, http.StatusCreated, map[string]any{
				"data": map[string]any{
					"id":   "at-EXAMPLE",
					"type": "authentication-tokens",
					"attributes": map[string]any{
						"description": "ci runner",
						"token":       fakeCreatedToken,
					},
				},
			})
		},
	})
	defer server.Close()

	io := iostreams.Test()
	opts := newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/organizations/example/authentication-tokens")
		opts.Method = http.MethodPost
	})
	opts.Output.SetRedactor(redact.New(redact.ModeStrict))

	require.NoError(t, RunAPI(context.Background(), opts))
	require.NotContains(t, io.Output.String(), fakeCreatedToken)
	require.Contains(t, io.Output.String(), "ci runner")
}

func TestWriteDryRunRequest_MasksTheRequestBody(t *testing.T) {
	t.Parallel()

	// A dry run is what a careful person does before setting a sensitive
	// variable, so the value being previewed is the secret itself.
	server, _ := newAPITestServer(map[string]http.HandlerFunc{})
	defer server.Close()

	io := iostreams.Test()
	opts := newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-EXAMPLE/vars")
		opts.Method = http.MethodPost
		opts.DryRun = true
		opts.Attributes = map[string]string{
			"key":       "db_password",
			"value":     "notarealpassword-EXAMPLE",
			"sensitive": "true",
		}
	})
	opts.Output.SetRedactor(redact.New(redact.ModeStrict))

	require.NoError(t, RunAPI(context.Background(), opts))

	report := io.Error.String()
	require.Contains(t, report, "would send POST request")
	require.NotContains(t, report, "notarealpassword-EXAMPLE")
	require.Contains(t, report, redact.Placeholder)
	// The rest of the preview still has to be useful.
	require.Contains(t, report, "db_password")
	require.Contains(t, report, "/workspaces/ws-EXAMPLE/vars")
	require.Contains(t, report, "--no-redact")
}

func TestWriteDryRunRequest_MasksASensitiveHeader(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{})
	defer server.Close()

	io := iostreams.Test()
	opts := newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-EXAMPLE/vars")
		opts.Method = http.MethodPost
		opts.DryRun = true
		opts.Attributes = map[string]string{"key": "harmless"}
		opts.Headers = []string{
			"Authorization: Bearer notarealtokenvalue-EXAMPLE",
			"X-Api-Key: notarealapikey-EXAMPLE",
			"X-Request-Id: keep-me",
		}
	})
	opts.Output.SetRedactor(redact.New(redact.ModeStrict))

	require.NoError(t, RunAPI(context.Background(), opts))

	report := io.Error.String()
	require.NotContains(t, report, "notarealtokenvalue-EXAMPLE")
	require.NotContains(t, report, "notarealapikey-EXAMPLE")
	require.Contains(t, report, "keep-me")
}

func TestWriteDryRunRequest_NoRedactShowsTheBody(t *testing.T) {
	t.Parallel()

	server, _ := newAPITestServer(map[string]http.HandlerFunc{})
	defer server.Close()

	io := iostreams.Test()
	opts := newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/workspaces/ws-EXAMPLE/vars")
		opts.Method = http.MethodPost
		opts.DryRun = true
		opts.Attributes = map[string]string{"value": "notarealpassword-EXAMPLE", "key": "db_password"}
	})
	opts.Output.SetRedactor(redact.New(redact.ModeOff))

	require.NoError(t, RunAPI(context.Background(), opts))
	require.Contains(t, io.Error.String(), "notarealpassword-EXAMPLE")
}

func TestFormatDryRunBody_WithholdsABodyItCannotParse(t *testing.T) {
	t.Parallel()

	// An unparseable body cannot be masked, so it must not be printed. Every body
	// this command sends is JSON, so this is already a request that would fail.
	body := []byte(`{"data": notjson notarealpassword-EXAMPLE`)

	withheld := formatDryRunBody(body, redact.New(redact.ModeStrict))
	require.NotContains(t, string(withheld), "notarealpassword-EXAMPLE")
	require.Contains(t, string(withheld), "--no-redact")

	shown := formatDryRunBody(body, redact.New(redact.ModeOff))
	require.Contains(t, string(shown), "notarealpassword-EXAMPLE")
}

func TestRunAPI_MasksRawJSONBody(t *testing.T) {
	t.Parallel()

	// Plan JSON output is not a JSON:API envelope, so no displayer handles it. It
	// holds every value Terraform wrote.
	const planJSON = `{"format_version":"1.2","variables":{"db_password":{"value":"notarealpassword-EXAMPLE"},"region":{"value":"us-east-1"}}}`

	server, _ := newAPITestServer(map[string]http.HandlerFunc{
		"GET /api/v2/plans/plan-EXAMPLE/json-output": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(planJSON))
		},
	})
	defer server.Close()

	io := iostreams.Test()
	opts := newTestOpts(t, server.URL, io, func(opts *Opts) {
		opts.URL = mustResolveTestURL(t, opts.Client.BaseURL.String(), "/plans/plan-EXAMPLE/json-output")
	})
	opts.Output.SetRedactor(redact.New(redact.ModeStrict))

	require.NoError(t, RunAPI(context.Background(), opts))

	require.NotContains(t, io.Output.String(), "notarealpassword-EXAMPLE")
	require.Contains(t, io.Output.String(), "us-east-1")
}
