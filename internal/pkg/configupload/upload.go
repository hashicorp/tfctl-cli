// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package configupload uploads local Terraform config to a workspace.
package configupload

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	tfe "github.com/hashicorp/go-tfe/v2"
	"github.com/hashicorp/go-tfe/v2/api/models"
)

const (
	// DefaultWaitTimeout bounds polling after the archive upload.
	DefaultWaitTimeout = 2 * time.Minute
	pollInterval       = 2 * time.Second
)

var configVersionIDPattern = regexp.MustCompile(`^cv-[a-zA-Z0-9-]+$`)

// Options controls how Upload creates a config version and waits for it to be ready.
type Options struct {
	// Speculative restricts the config version to speculative runs.
	Speculative bool
	// Provisional prevents creation from setting the workspace's current config.
	Provisional bool
	// AutoQueueRun lets the server queue a run after upload.
	AutoQueueRun bool

	// NoWait skips polling after the archive upload succeeds.
	NoWait bool
	// Timeout bounds polling. A nonpositive value uses [DefaultWaitTimeout].
	Timeout time.Duration
	// OnStatus, if set, receives the first status from polling and each change.
	OnStatus func(id, status string)
}

// Result identifies the config version and workspace without exposing the upload URL.
// Status is the most recent status returned by the API, or "pending" if the
// create response did not contain a status. The three flags reflect the request.
type Result struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspace_id"`
	Status       string `json:"status"`
	Speculative  bool   `json:"speculative"`
	Provisional  bool   `json:"provisional"`
	AutoQueueRun bool   `json:"auto_queue_run"`
}

// Uploader creates config versions from local directories. Use [NewUploader]
// to construct one.
type Uploader struct {
	api          *tfe.Client
	uploadConfig func(context.Context, models.ConfigurationVersionsable, *tfe.PackedConfig) error
}

// NewUploader returns an uploader that uses api for config version requests.
func NewUploader(api *tfe.Client) *Uploader {
	return &Uploader{api: api, uploadConfig: api.UploadConfig}
}

// Upload packages configDir before creating a config version in workspaceID.
// It uploads the archive and waits for the uploaded status unless opts.NoWait is set.
// If the server returns a valid config version ID, Upload includes that ID in
// the result even if the upload or wait fails. It does not delete the version.
func (u *Uploader) Upload(ctx context.Context, workspaceID, configDir string, opts Options) (Result, error) {
	packed, err := tfe.PackConfig(ctx, configDir)
	if err != nil {
		return Result{}, err
	}
	// Release the archive before polling. Polling does not need its bytes.
	result, err := u.createAndUpload(ctx, workspaceID, packed, opts)
	if closeErr := packed.Close(); closeErr != nil {
		if result.ID != "" {
			closeErr = fmt.Errorf("close packed config for %s: %w", result.ID, closeErr)
		}
		err = errors.Join(err, closeErr)
	}
	if err != nil || opts.NoWait {
		return result, err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultWaitTimeout
	}
	waitCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	if err := u.waitForUpload(waitCtx, &result, opts.OnStatus); err != nil {
		return result, fmt.Errorf("config version %s was uploaded, but did not become usable: %w", result.ID, err)
	}
	return result, nil
}

func (u *Uploader) createAndUpload(ctx context.Context, workspaceID string, packed *tfe.PackedConfig, opts Options) (Result, error) {
	attrs := models.NewConfigurationVersions_attributes()
	attrs.SetAutoQueueRuns(&opts.AutoQueueRun)
	attrs.SetSpeculative(&opts.Speculative)
	attrs.SetProvisional(&opts.Provisional)
	data := models.NewConfigurationVersions()
	kind := models.CONFIGURATIONVERSIONS_CONFIGURATIONVERSIONS_TYPE
	data.SetTypeEscaped(&kind)
	data.SetAttributes(attrs)
	body := models.NewConfigurationVersionsEnvelope()
	body.SetData(data)

	response, err := u.api.API.Workspaces().ByWorkspace_id(workspaceID).ConfigurationVersions().Post(ctx, body, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create config version: %w", sanitizeAPIError(ctx, err))
	}
	if response == nil || response.GetData() == nil {
		return Result{}, errors.New("create config version: response has no valid config version ID")
	}
	version := response.GetData()
	id := version.GetId()
	if id == nil || !configVersionIDPattern.MatchString(*id) {
		return Result{}, errors.New("create config version: response has no valid config version ID")
	}
	result := Result{
		ID:           *id,
		WorkspaceID:  workspaceID,
		Status:       "pending",
		Speculative:  opts.Speculative,
		Provisional:  opts.Provisional,
		AutoQueueRun: opts.AutoQueueRun,
	}
	if attrs := version.GetAttributes(); attrs != nil && attrs.GetStatus() != nil {
		result.Status = attrs.GetStatus().String()
	}
	if err := u.uploadConfig(ctx, version, packed); err != nil {
		return result, fmt.Errorf("config version %s was created, but its upload failed: %w", result.ID, err)
	}
	return result, nil
}

func (u *Uploader) waitForUpload(ctx context.Context, result *Result, onStatus func(string, string)) error {
	version := u.api.API.ConfigurationVersions().ByConfiguration_version_id(result.ID)
	last := ""
	for {
		response, err := version.Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("read status: %w", sanitizeAPIError(ctx, err))
		}
		if response == nil || response.GetData() == nil || response.GetData().GetAttributes() == nil || response.GetData().GetAttributes().GetStatus() == nil {
			return errors.New("response has no recognized status")
		}
		status := response.GetData().GetAttributes().GetStatus()
		result.Status = status.String()
		if onStatus != nil && result.Status != last {
			onStatus(result.ID, result.Status)
			last = result.Status
		}
		switch *status {
		case models.UPLOADED_CONFIGURATIONVERSIONS_ATTRIBUTES_STATUS:
			return nil
		case models.PENDING_CONFIGURATIONVERSIONS_ATTRIBUTES_STATUS, models.FETCHING_CONFIGURATIONVERSIONS_ATTRIBUTES_STATUS:
		default:
			return fmt.Errorf("reached unusable status %q", result.Status)
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// sanitizeAPIError drops API error details because they can contain the upload URL.
func sanitizeAPIError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var apiErr *tfe.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("server returned HTTP %d", apiErr.StatusCode)
	}
	return errors.New("API request failed")
}
