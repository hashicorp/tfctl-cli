// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package configupload creates and uploads config versions from local directories.
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

var configVersionID = regexp.MustCompile(`^cv-[a-zA-Z0-9-]+$`)

// Options controls creation and waiting after upload.
type Options struct {
	Speculative  bool
	Provisional  bool
	AutoQueueRun bool
	NoWait       bool
	Timeout      time.Duration
	OnStatus     func(id, status string)
}

// Result contains safe config version facts, including the last known status.
type Result struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspace_id"`
	Status       string `json:"status"`
	Speculative  bool   `json:"speculative"`
	Provisional  bool   `json:"provisional"`
	AutoQueueRun bool   `json:"auto_queue_run"`
}

// Uploader uses go-tfe to pack and upload a config directory.
type Uploader struct {
	api *tfe.Client
	put func(context.Context, models.ConfigurationVersionsable, *tfe.PackedConfig) error
}

// NewUploader uses api to create, upload, and read config versions.
func NewUploader(api *tfe.Client) *Uploader {
	return &Uploader{api: api, put: api.UploadConfig}
}

// Upload packs before creating a config version. After creation, it returns the
// config version ID even if the upload or wait fails. It never deletes a version.
func (u *Uploader) Upload(ctx context.Context, workspaceID, configDir string, opts Options) (Result, error) {
	packed, err := tfe.PackConfig(ctx, configDir)
	if err != nil {
		return Result{}, err
	}
	// Close after the PUT (or a failed POST), before waiting for remote processing.
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
	if err := u.wait(waitCtx, &result, opts.OnStatus); err != nil {
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
		return Result{}, fmt.Errorf("create config version: %w", safeAPIError(ctx, err))
	}
	if response == nil || response.GetData() == nil || response.GetData().GetId() == nil || !configVersionID.MatchString(*response.GetData().GetId()) {
		return Result{}, errors.New("create config version: response has no valid config version ID")
	}
	version := response.GetData()
	result := Result{
		ID: *version.GetId(), WorkspaceID: workspaceID, Status: "pending",
		Speculative: opts.Speculative, Provisional: opts.Provisional, AutoQueueRun: opts.AutoQueueRun,
	}
	if attrs := version.GetAttributes(); attrs != nil && attrs.GetStatus() != nil {
		result.Status = attrs.GetStatus().String()
	}
	if err := u.put(ctx, version, packed); err != nil {
		return result, fmt.Errorf("config version %s was created, but its upload failed: %w", result.ID, err)
	}
	return result, nil
}

func (u *Uploader) wait(ctx context.Context, result *Result, onStatus func(string, string)) error {
	version := u.api.API.ConfigurationVersions().ByConfiguration_version_id(result.ID)
	last := ""
	for {
		response, err := version.Get(ctx, nil)
		if err != nil {
			return fmt.Errorf("read status: %w", safeAPIError(ctx, err))
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

// Error details from the API can include the upload URL. Do not retain them.
func safeAPIError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var apiErr *tfe.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("server returned HTTP %d", apiErr.StatusCode)
	}
	return errors.New("API request failed")
}
