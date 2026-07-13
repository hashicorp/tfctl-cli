// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

// A JSON:API envelope with a one-to-one relationship. The displayer pulls the
// relationship ID up into the flattened rows for table/pretty display, but it
// must not mutate the raw envelope that feeds --json / --jq output.
const relEnvelope = `{
  "data": [
    {
      "id": "ws-abc123",
      "type": "workspaces",
      "attributes": {
        "name": "my-workspace",
        "auto-apply": false
      },
      "relationships": {
        "organization": {
          "data": {"id": "org-xyz789", "type": "organizations"}
        }
      }
    }
  ]
}`

// TestJSONAPI_JSONOutput_NotPollutedByRelationshipID asserts that JSON output
// reflects the server payload exactly: the relationship ID must NOT be injected
// into data[].attributes.
func TestJSONAPI_JSONOutput_NotPollutedByRelationshipID(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	disp, err := format.NewJSONAPIDisplayer([]byte(relEnvelope), hclog.Default())
	r.NoError(err)

	io := iostreams.Test()
	out := format.New(io)
	out.SetFormat(format.JSON)
	r.NoError(out.Display(disp))

	var parsed struct {
		Data []struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"data"`
	}
	r.NoError(json.Unmarshal(io.Output.Bytes(), &parsed))
	r.Len(parsed.Data, 1)

	attrs := parsed.Data[0].Attributes
	r.Contains(attrs, "name", "real attributes should survive")
	r.NotContains(attrs, "organization",
		"relationship ID must not be injected into attributes in JSON output; got %v", attrs)
}

// TestJSONAPI_PayloadStableAcrossFormats asserts the raw payload is not mutated
// as a side effect of rendering a table (which pulls relationship IDs up).
func TestJSONAPI_PayloadStableAcrossFormats(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	disp, err := format.NewJSONAPIDisplayer([]byte(relEnvelope), hclog.Default())
	r.NoError(err)

	// Render as a table first (this is what triggers the relationship pull-up).
	tableIO := iostreams.Test()
	tableOut := format.New(tableIO)
	tableOut.SetFormat(format.Table)
	r.NoError(tableOut.Display(disp))

	// Now render JSON from the same displayer and confirm attributes are clean.
	jsonIO := iostreams.Test()
	jsonOut := format.New(jsonIO)
	jsonOut.SetFormat(format.JSON)
	r.NoError(jsonOut.Display(disp))

	var parsed struct {
		Data []struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"data"`
	}
	r.NoError(json.Unmarshal(jsonIO.Output.Bytes(), &parsed))
	r.Len(parsed.Data, 1)
	r.NotContains(parsed.Data[0].Attributes, "organization",
		"raw payload was mutated by table rendering; attributes = %v", parsed.Data[0].Attributes)
}
