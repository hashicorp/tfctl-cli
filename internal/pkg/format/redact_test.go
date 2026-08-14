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

// stateVersionEnvelope is a state version response. The download URLs are signed
// capability URLs: anyone who holds one can read the whole state, which contains
// every value Terraform wrote.
const stateVersionEnvelope = `{
	"data": {
		"id": "sv-abc123",
		"type": "state-versions",
		"attributes": {
			"serial": 7,
			"status": "finalized",
			"hosted-state-download-url": "https://archivist.terraform.io/v1/object/secret-token-abc",
			"hosted-json-state-download-url": "https://archivist.terraform.io/v1/object/secret-token-def"
		}
	}
}`

func newRedactingOutputter(t *testing.T, mode redact.Mode) (*format.Outputter, *iostreams.Testing) {
	t.Helper()

	io := iostreams.Test()
	out := format.New(io)
	out.SetRedactor(redact.New(mode))
	return out, io
}

func TestDisplay_RedactsJSONOutput(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	out, io := newRedactingOutputter(t, redact.ModeStrict)
	out.SetFormat(format.JSON)

	disp, err := format.NewJSONAPIDisplayer([]byte(stateVersionEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	r.NotContains(io.Output.String(), "secret-token-abc")
	r.NotContains(io.Output.String(), "secret-token-def")
	r.Contains(io.Output.String(), redact.Placeholder)
	// The escaping matters: an angle-bracket placeholder would be written as
	// <redacted> by encoding/json.
	r.NotContains(io.Output.String(), `<`)
	// Attributes that are not sensitive still render.
	r.Contains(io.Output.String(), `"status": "finalized"`)
}

func TestDisplay_RedactsBeforeTheJQFilter(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// A jq filter that selects the sensitive attribute directly must not be able
	// to reach around the mask.
	out, io := newRedactingOutputter(t, redact.ModeStrict)
	out.SetFormat(format.JSON)
	out.SetJQFilter(`.data.attributes["hosted-state-download-url"]`)

	disp, err := format.NewJSONAPIDisplayer([]byte(stateVersionEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	r.NotContains(io.Output.String(), "secret-token-abc")
	r.Equal(redact.Placeholder, strings.TrimSpace(io.Output.String()))
}

func TestDisplay_RedactsPrettyOutput(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Pretty is the default format for a single resource, and it prints every
	// attribute that is not excluded, so it leaks the same values as --json.
	out, io := newRedactingOutputter(t, redact.ModeStrict)

	disp, err := format.NewJSONAPIDisplayer([]byte(stateVersionEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	r.NotContains(io.Output.String(), "secret-token-abc")
	r.NotContains(io.Output.String(), "secret-token-def")
	r.Contains(io.Output.String(), redact.Placeholder)
}

func TestDisplay_RedactsTableOutput(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	const varsEnvelope = `{
		"data": [
			{"id":"var-1","type":"vars","attributes":{"key":"region","value":"us-east-1","sensitive":false,"category":"terraform"}},
			{"id":"var-2","type":"vars","attributes":{"key":"db_password","value":"hunter2","sensitive":false,"category":"terraform"}}
		]
	}`

	out, io := newRedactingOutputter(t, redact.ModeStrict)

	disp, err := format.NewJSONAPIDisplayer([]byte(varsEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	r.NotContains(io.Output.String(), "hunter2")
	r.Contains(io.Output.String(), "us-east-1")
	r.Contains(io.Output.String(), redact.Placeholder)
}

func TestDisplay_ReportsMaskedFieldsOnce(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	out, io := newRedactingOutputter(t, redact.ModeStrict)
	out.SetFormat(format.JSON)

	disp, err := format.NewJSONAPIDisplayer([]byte(stateVersionEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	report := io.Error.String()
	r.Contains(report, "masked 2 sensitive fields")
	r.Contains(report, "hosted-state-download-url")
	r.Contains(report, "--no-redact")

	// Rendering the same payload again must not repeat the report.
	io.Error.Reset()
	r.NoError(out.Display(disp))
	r.Empty(io.Error.String())
}

func TestDisplay_NoRedactorLeavesThePayloadAlone(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	out := format.New(io)
	out.SetFormat(format.JSON)

	disp, err := format.NewJSONAPIDisplayer([]byte(stateVersionEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	r.Contains(io.Output.String(), "secret-token-abc")
	r.Empty(io.Error.String())
}

func TestDisplay_OffModeLeavesThePayloadAlone(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	out, io := newRedactingOutputter(t, redact.ModeOff)
	out.SetFormat(format.JSON)

	disp, err := format.NewJSONAPIDisplayer([]byte(stateVersionEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	r.Contains(io.Output.String(), "secret-token-abc")
	r.Empty(io.Error.String())
}

func TestCopyRaw(t *testing.T) {
	t.Parallel()

	// Plan JSON output is not a JSON:API envelope, so no displayer handles it.
	// It holds the values Terraform wrote, whether or not the configuration
	// marked them sensitive.
	const planJSON = `{"format_version":"1.2","variables":{"db_password":{"value":"hunter2"}}}`

	t.Run("masks a JSON body", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		out, io := newRedactingOutputter(t, redact.ModeStrict)
		r.NoError(out.CopyRaw(strings.NewReader(planJSON), "application/json; charset=utf-8"))

		r.NotContains(io.Output.String(), "hunter2")
		r.Contains(io.Output.String(), redact.Placeholder)
		r.Contains(io.Error.String(), "masked 1 sensitive field")
	})

	t.Run("passes an unmasked JSON body through byte for byte", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		const harmless = `{"format_version":"1.2","variables":{"region":{"value":"us-east-1"}}}`

		out, io := newRedactingOutputter(t, redact.ModeStrict)
		r.NoError(out.CopyRaw(strings.NewReader(harmless), "application/json"))

		r.Equal(harmless, io.Output.String())
		r.Empty(io.Error.String())
	})

	t.Run("passes a non-JSON body through", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		const logOutput = "Terraform will perform the following actions"

		out, io := newRedactingOutputter(t, redact.ModeStrict)
		r.NoError(out.CopyRaw(strings.NewReader(logOutput), "text/plain"))

		r.Equal(logOutput, io.Output.String())
	})

	t.Run("passes a malformed JSON body through", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		const malformed = `{"not":`

		out, io := newRedactingOutputter(t, redact.ModeStrict)
		r.NoError(out.CopyRaw(strings.NewReader(malformed), "application/json"))

		r.Equal(malformed, io.Output.String())
	})

	t.Run("streams when redaction is off", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		out, io := newRedactingOutputter(t, redact.ModeOff)
		r.NoError(out.CopyRaw(strings.NewReader(planJSON), "application/json"))

		r.Equal(planJSON, io.Output.String())
	})
}

func TestReportRedactions_QuietSuppressesTheReport(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	out, io := newRedactingOutputter(t, redact.ModeStrict)
	out.SetFormat(format.JSON)
	io.SetQuiet(true)

	disp, err := format.NewJSONAPIDisplayer([]byte(stateVersionEnvelope), hclog.Default())
	r.NoError(err)
	r.NoError(out.Display(disp))

	r.NotContains(io.Output.String(), "secret-token-abc")
	r.Empty(io.Error.String())
}
