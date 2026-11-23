// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package configupload

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	tfe "github.com/hashicorp/go-tfe/v2"
	"github.com/hashicorp/go-tfe/v2/api/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdtest"
)

func TestUpload(t *testing.T) {
	t.Parallel()
	const capability = "http://upload.invalid/archive?token=CAPABILITY_SECRET"
	const create = "POST /api/v2/workspaces/ws-test/configuration-versions"
	const read = "GET /api/v2/configuration-versions/cv-test"
	for _, tt := range []struct {
		name        string
		status      string
		wantStatus  string
		wantError   string
		options     Options
		missingDir  bool
		realUpload  bool
		createError bool
		readError   bool
		timeout     bool
		cancel      bool
	}{
		{
			name:       "wait for a speculative config version",
			status:     "uploaded",
			wantStatus: "uploaded",
			options:    Options{Speculative: true},
		},
		{
			name:       "no wait with auto-queue enabled",
			wantStatus: "pending",
			options:    Options{NoWait: true, AutoQueueRun: true},
		},
		{name: "pack failure", missingDir: true, wantError: "pack config"},
		{name: "create failure", createError: true, wantError: "HTTP 403"},
		{name: "SDK rejects HTTP", realUpload: true, wantStatus: "pending", wantError: "invalid HTTPS upload URL"},
		{
			name:       "terminal status",
			status:     "errored",
			wantStatus: "errored",
			wantError:  "unusable status",
			options:    Options{Provisional: true},
		},
		{name: "read failure", readError: true, wantStatus: "pending", wantError: "HTTP 403"},
		{name: "timeout", timeout: true, status: "fetching", wantError: "deadline exceeded"},
		{name: "cancellation", cancel: true, wantStatus: "pending", wantError: "context canceled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var creates, reads atomic.Int32
			server := cmdtest.NewServer(t, cmdtest.RouteMap{
				create: func(w http.ResponseWriter, r *http.Request) {
					creates.Add(1)
					var body struct {
						Data struct {
							Type       string
							Attributes map[string]bool
						}
					}
					if assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
						assert.Equal(t, "configuration-versions", body.Data.Type)
						assert.Equal(t, map[string]bool{
							"auto-queue-runs": tt.options.AutoQueueRun,
							"speculative":     tt.options.Speculative,
							"provisional":     tt.options.Provisional,
						}, body.Data.Attributes)
					}
					if tt.createError {
						writeDenied(w, capability)
						return
					}
					writeVersion(w, "pending", capability)
				},
				read: func(w http.ResponseWriter, _ *http.Request) {
					reads.Add(1)
					if tt.readError {
						writeDenied(w, capability)
						return
					}
					writeVersion(w, tt.status, capability)
				},
			})
			api, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "dummy"})
			require.NoError(t, err)
			uploader := NewUploader(api)
			dir := t.TempDir()
			if tt.missingDir {
				dir = filepath.Join(dir, "missing")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var packed *tfe.PackedConfig
			if !tt.realUpload {
				uploader.uploadConfig = func(_ context.Context, version models.ConfigurationVersionsable, config *tfe.PackedConfig) error {
					require.Equal(t, int32(1), creates.Load())
					require.Equal(t, int32(0), reads.Load())
					require.Equal(t, "cv-test", *version.GetId())
					packed = config
					if tt.cancel {
						cancel()
						return ctx.Err()
					}
					return nil
				}
			}
			var updates []string
			opts := tt.options
			opts.Timeout = time.Second
			opts.OnStatus = func(_ string, status string) {
				updates = append(updates, status)
			}
			if tt.timeout {
				opts.Timeout = 20 * time.Millisecond
			}
			result, err := uploader.Upload(ctx, "ws-test", dir, opts)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				assert.NotContains(t, err.Error(), "CAPABILITY_SECRET")
			} else {
				require.NoError(t, err)
			}
			if tt.missingDir {
				assert.Zero(t, creates.Load())
				assert.Nil(t, packed)
				return
			}
			if tt.createError {
				assert.Equal(t, int32(1), creates.Load())
				assert.Zero(t, reads.Load())
				assert.Nil(t, packed)
				assert.Empty(t, result.ID)
				return
			}
			require.Equal(t, "cv-test", result.ID)
			if err != nil {
				assert.ErrorContains(t, err, result.ID)
			}
			assert.Equal(t, "ws-test", result.WorkspaceID)
			assert.Equal(t, opts.AutoQueueRun, result.AutoQueueRun)
			assert.Equal(t, opts.Speculative, result.Speculative)
			assert.Equal(t, opts.Provisional, result.Provisional)
			if tt.wantStatus != "" {
				assert.Equal(t, tt.wantStatus, result.Status)
			}
			if opts.NoWait || tt.realUpload || tt.cancel {
				assert.Zero(t, reads.Load())
			}
			if tt.wantStatus == "uploaded" {
				assert.Equal(t, []string{"uploaded"}, updates)
			}
			if tt.timeout {
				assert.ErrorIs(t, err, context.DeadlineExceeded)
			}
			if tt.cancel {
				assert.ErrorIs(t, err, context.Canceled)
			}
			if packed != nil {
				assert.ErrorContains(t, api.UploadConfig(context.Background(), models.NewConfigurationVersions(), packed), "closed")
			}
		})
	}
}

func writeDenied(w http.ResponseWriter, detail string) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusForbidden)
	cmdtest.WriteJSONAPI(w, map[string]any{"errors": []map[string]string{{"title": "Denied", "detail": detail}}})
}

func writeVersion(w http.ResponseWriter, status, capability string) {
	cmdtest.WriteJSONAPI(w, map[string]any{
		"data": map[string]any{
			"id":   "cv-test",
			"type": "configuration-versions",
			"attributes": map[string]string{
				"status":     status,
				"upload-url": capability,
			},
		},
	})
}
