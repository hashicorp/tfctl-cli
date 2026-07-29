// Package checkpoint provides functionality for interacting with HashiCorp's
// Checkpoint service to check for new versions and alerts related to the
// current version of the CLI.
package checkpoint

import (
	"context"
	"path/filepath"

	"github.com/hashicorp/go-checkpoint"

	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

func init() {
	checkpointResult = make(chan *checkpoint.CheckResponse, 1)
}

var checkpointResult chan *checkpoint.CheckResponse

// VersionCheckInfo holds information about the current version of the CLI, including whether it
// is outdated, the latest version available, and any alerts related to the current version.
type VersionCheckInfo struct {
	Outdated bool
	Latest   string
	Alerts   []string
}

// Run begins a HashiCorp Checkpoint request, which checks for new versions of the
// CLI using the HashiCorp Checkpoint service. Read about checkpoint
// at https://checkpoint.hashicorp.com/
func Run(ctx context.Context, disabled bool) {
	logger := logging.FromContext(ctx)

	// If the user doesn't want checkpoint at all, then return.
	if disabled {
		logger.Debug("Checkpoint disabled.")
		checkpointResult <- nil
		return
	}

	configDir, err := profile.ConfigDir()
	if err != nil {
		logger.Debug("Checkpoint setup error", "error", err)
		checkpointResult <- nil
		return
	}

	resp, err := checkpoint.Check(&checkpoint.CheckParams{
		Product:       version.Name,
		Version:       version.Version,
		SignatureFile: filepath.Join(configDir, "checkpoint_signature"),
		CacheFile:     filepath.Join(configDir, "checkpoint_cache"),
	})
	if err != nil {
		logger.Debug("Checkpoint error: %s", err)
		resp = nil
	}

	checkpointResult <- resp
}

// WaitForVersionCheck waits for the result of a Checkpoint request and returns
// the version information. If the request failed, it returns an empty VersionCheckInfo.
func WaitForVersionCheck() VersionCheckInfo {
	// Wait for the result to come through
	info := <-checkpointResult
	if info == nil {
		var zero VersionCheckInfo
		return zero
	}

	// Build the alerts that we may have received about our version
	alerts := make([]string, len(info.Alerts))
	for i, a := range info.Alerts {
		alerts[i] = a.Message
	}

	return VersionCheckInfo{
		Outdated: info.Outdated,
		Latest:   info.CurrentVersion,
		Alerts:   alerts,
	}
}
