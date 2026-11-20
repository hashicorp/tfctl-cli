// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package configversion

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdtest"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/configupload"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/redact"
)

func TestConfigVersionHelp(t *testing.T) {
	t.Parallel()
	inv := &cmd.Invocation{IO: iostreams.Test()}
	group := NewCmdConfigVersion(inv)
	assert.Equal(t, "config-version", group.Name)
	assert.Contains(t, group.LongHelp, "Terraform config")
	assert.Contains(t, group.LongHelp, "without queuing a run by default")
	assert.NotContains(t, group.LongHelp, "configuration")
}

func TestUploadCommand(t *testing.T) {
	t.Parallel()
	for _, dryRun := range []bool{false, true} {
		name := "upload failure without redaction"
		if dryRun {
			name = "dry run"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var creates atomic.Int32
			server := cmdtest.NewServer(t, cmdtest.RouteMap{
				"GET /api/v2/organizations/flag-org/workspaces/test": func(w http.ResponseWriter, _ *http.Request) {
					cmdtest.WriteJSONAPI(w, map[string]any{"data": map[string]string{"id": "ws-test", "type": "workspaces"}})
				},
				"POST /api/v2/workspaces/ws-test/configuration-versions": func(w http.ResponseWriter, _ *http.Request) {
					creates.Add(1)
					cmdtest.WriteJSONAPI(w, map[string]any{"data": map[string]any{
						"id": "cv-test", "type": "configuration-versions", "attributes": map[string]string{
							"status": "pending", "upload-url": "http://upload.invalid/archive?token=CAPABILITY_SECRET",
						},
					}})
				},
			})
			io := iostreams.Test()
			inv := cmdtest.NewInvocation(t, io, server)
			inv.Profile.DefaultOrganization = "profile-org"
			parent := &cmd.Command{Name: "test"}
			cmd.ConfigureRootCommand(inv, parent)
			c := newCmdUpload(inv)
			parent.AddChild(c)
			dir := t.TempDir()
			args := []string{"test", "--organization", "flag-org", "--config-dir", dir, "--no-redact", "--quiet"}
			if dryRun {
				require.Zero(t, c.Run(append(args, "--dry-run"), inv))
				assert.Zero(t, creates.Load())
				assert.Contains(t, io.Error.String(), dir)
				assert.Contains(t, io.Error.String(), "ws-test")
			} else {
				require.Equal(t, 1, c.Run(args, inv))
				assert.Equal(t, int32(1), creates.Load())
				assert.Contains(t, io.Error.String(), "config version cv-test was created")
				assert.Contains(t, io.Error.String(), "invalid HTTPS upload URL")
				assert.NotContains(t, io.Error.String(), "CAPABILITY_SECRET")
			}
			assert.Empty(t, io.Output.String())
		})
	}
}

func TestRunUpload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := cmdtest.NewServer(t, cmdtest.RouteMap{
		"GET /api/v2/organizations/my-org/workspaces/my-workspace": func(w http.ResponseWriter, _ *http.Request) {
			cmdtest.WriteJSONAPI(w, map[string]any{"data": map[string]string{"id": "ws-resolved", "type": "workspaces"}})
		},
	})
	api, err := client.New(ctx, server.URL, "dummy", nil)
	require.NoError(t, err)
	io := iostreams.Test()
	opts := uploadOpts{
		io: io, output: format.New(io), resolver: client.NewResolver(api, false, true),
		workspace: "my-workspace", organization: "my-org", configDir: t.TempDir(), dryRun: true,
		upload: func(context.Context, string, string, configupload.Options) (configupload.Result, error) {
			t.Fatal("dry-run must not invoke the uploader")
			return configupload.Result{}, nil
		},
	}
	require.NoError(t, runUpload(ctx, opts))
	assert.Contains(t, io.Error.String(), "workspace ws-resolved")
	assert.Contains(t, io.Error.String(), "auto-queue-runs=false")
	assert.Empty(t, io.Output.String())
}

func TestUploadOutput(t *testing.T) {
	t.Parallel()
	for name, kind := range map[string]format.Format{
		"json": format.JSON, "markdown": format.Markdown, "pretty": format.Pretty, "table": format.Table, "agent": format.Agent,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			io := iostreams.Test()
			io.SetQuiet(true)
			output := format.New(io)
			output.SetFormat(kind)
			output.SetRedactor(redact.New(redact.ModeOff))
			opts := uploadOpts{io: io, output: output, workspace: "ws-test", configDir: ".",
				upload: func(_ context.Context, workspace, dir string, opts configupload.Options) (configupload.Result, error) {
					assert.Equal(t, ".", dir)
					assert.False(t, opts.AutoQueueRun)
					opts.OnStatus("cv-test", "uploaded")
					return configupload.Result{ID: "cv-test", WorkspaceID: workspace, Status: "uploaded"}, nil
				},
			}
			require.NoError(t, runUpload(context.Background(), opts))
			assert.Contains(t, io.Output.String(), "cv-test")
			assert.Contains(t, io.Output.String(), "uploaded")
			assert.NotContains(t, io.Output.String(), "upload-url")
			assert.Empty(t, io.Error.String())
		})
	}
	io := iostreams.Test()
	output := format.New(io)
	output.SetFormat(format.JSON)
	output.SetJQFilter(".id")
	require.NoError(t, output.Display(uploadDisplayer{configupload.Result{ID: "cv-test"}}))
	assert.Equal(t, "cv-test\n", io.Output.String())
}
