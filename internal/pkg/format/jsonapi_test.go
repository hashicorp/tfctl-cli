// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestJSONAPI_TemplateCollectionColumns(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	const envelope = `{
  "data": [
    {
      "id": "template-123",
      "type": "tdp-templates",
      "attributes": {
        "name": "web-app",
        "summary": "Web application",
        "description": "A detailed template description",
        "module-source": "private",
        "module-id": "mod-123",
        "created-at": "2026-01-01T00:00:00Z",
        "updated-at": "2026-01-02T00:00:00Z"
      }
    }
  ]
}`

	disp, err := format.NewJSONAPIDisplayer([]byte(envelope), hclog.NewNullLogger())
	r.NoError(err)
	r.Equal(format.Table, disp.DefaultFormat())

	fields := disp.FieldTemplates()
	headers := make([]string, len(fields))
	for i, field := range fields {
		headers[i] = field.Name
	}
	r.Equal([]string{"ID", "Name", "Summary", "Module Source", "Module ID", "Updated At"}, headers)

	io := iostreams.Test()
	out := format.New(io)
	r.NoError(out.Display(disp))
	r.Contains(io.Output.String(), "web-app")
	r.NotContains(io.Output.String(), "A detailed template description")
	r.NotContains(io.Output.String(), "2026-01-01")
}

func TestJSONAPI_DeveloperPortalCollectionColumns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		envelope string
		headers  []string
		values   []string
		hidden   []string
	}{
		{
			name: "tdp-addon-definitions",
			envelope: `{
  "data": [
    {
      "id": "definition-123",
      "type": "tdp-addon-definitions",
      "attributes": {
        "name": "database",
        "summary": "Shared DB",
        "description": "A detailed definition description",
        "module-source": "private",
        "module-id": "mod-456",
        "readme-template": "Definition readme",
        "created-at": "2026-01-01T00:00:00Z",
        "updated-at": "2026-01-02T00:00:00Z"
      }
    }
  ]
}`,
			headers: []string{"ID", "Name", "Summary", "Module Source", "Module ID", "Updated At"},
			values:  []string{"definition-123", "database", "Shared DB", "private", "mod-456", "2026-01-02"},
			hidden:  []string{"A detailed definition description", "Definition readme", "2026-01-01"},
		},
		{
			name: "tdp-applications",
			envelope: `{
  "data": [
    {
      "id": "app-123",
      "type": "tdp-applications",
      "attributes": {
        "name": "web-app",
        "template-name": "web-template",
        "application-template": {
          "name": "snapshot-name",
          "description": "A detailed template snapshot"
        },
        "readme": "Application readme",
        "tfc-workspace-id": "ws-123",
        "created-at": "2026-01-01T00:00:00Z",
        "updated-at": "2026-01-02T00:00:00Z"
      },
      "relationships": {
        "project": {
          "data": {"id": "prj-123", "type": "projects"}
        }
      }
    }
  ]
}`,
			headers: []string{"ID", "Name", "Template Name", "Tfc Workspace ID", "Project", "Updated At"},
			values:  []string{"app-123", "web-app", "web-template", "ws-123", "prj-123", "2026-01-02"},
			hidden:  []string{"snapshot-name", "A detailed template snapshot", "Application readme", "2026-01-01"},
		},
		{
			name: "tdp-addons",
			envelope: `{
  "data": [
    {
      "id": "addon-123",
      "type": "tdp-addons",
      "attributes": {
        "name": "app-database",
        "definition": {"name": "postgres"},
        "application": {"id": "app-123"},
        "summary": "Shared database",
        "description": "A detailed add-on description",
        "module-source": "private",
        "module-id": "mod-456",
        "terraform-workspace-id": "ws-456",
        "created-at": "2026-01-01T00:00:00Z",
        "updated-at": "2026-01-02T00:00:00Z"
      }
    }
  ]
}`,
			headers: []string{"ID", "Name", "Definition.Name", "Terraform Workspace ID", "Application.ID", "Updated At"},
			values:  []string{"addon-123", "app-database", "postgres", "ws-456", "app-123", "2026-01-02"},
			hidden:  []string{"Shared database", "A detailed add-on description", "private", "mod-456", "2026-01-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			disp, err := format.NewJSONAPIDisplayer([]byte(tt.envelope), hclog.NewNullLogger())
			r.NoError(err)
			r.Equal(format.Table, disp.DefaultFormat())

			fields := disp.FieldTemplates()
			headers := make([]string, len(fields))
			for i, field := range fields {
				headers[i] = field.Name
			}
			r.Equal(tt.headers, headers)

			io := iostreams.Test()
			out := format.New(io)
			r.NoError(out.Display(disp))
			for _, value := range tt.values {
				r.Contains(io.Output.String(), value)
			}
			for _, value := range tt.hidden {
				r.NotContains(io.Output.String(), value)
			}
		})
	}
}

func TestJSONAPI_DeveloperPortalCollectionMissingOptionalFields(t *testing.T) {
	t.Parallel()

	for _, resourceType := range []string{"tdp-templates", "tdp-applications", "tdp-addon-definitions", "tdp-addons"} {
		t.Run(resourceType, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			envelope := fmt.Sprintf(`{
  "data": [
    {
      "id": "resource-123",
      "type": %q,
      "attributes": {
        "name": "example",
        "created-at": "2026-01-01T00:00:00Z"
      }
    }
  ]
}`, resourceType)
			disp, err := format.NewJSONAPIDisplayer([]byte(envelope), hclog.NewNullLogger())
			r.NoError(err)
			r.Equal(format.Table, disp.DefaultFormat())

			fields := disp.FieldTemplates()
			headers := make([]string, len(fields))
			for i, field := range fields {
				headers[i] = field.Name
			}
			r.Equal([]string{"ID", "Name", "Created At"}, headers)

			io := iostreams.Test()
			out := format.New(io)
			r.NoError(out.Display(disp))
			r.Contains(io.Output.String(), "resource-123")
			r.Contains(io.Output.String(), "example")
			r.Contains(io.Output.String(), "2026-01-01")
		})
	}
}
