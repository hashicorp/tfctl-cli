package profile

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
)

// HostCacheLoader is responsible for loading and refreshing cached data for a host.
type HostCacheLoader struct {
	dir    string
	logger hclog.Logger
}

// FileID is an identifier for a specific cache file. It should only contain the file name
// and no path components.
type FileID string

// ErrInvalidFileID is returned when a FileID contains path components, is a directory, or is
// otherwise invalid.
var ErrInvalidFileID = errors.New("invalid file ID: must be a file name without path components")

// RefreshResult represents the result of a network refresh check, including the new data (if any).
type RefreshResult struct {
	DataIfNew    []byte
	Err          error
	LastModified *time.Time
}

// CheckRefreshFunc checks the upstream source for new data, returning nil if the
// cached data is still valid, or the new data if it is not. Return an error if
// the refresh check fails.
type CheckRefreshFunc func(mTime *time.Time) RefreshResult

// NewHostCacheLoader creates a new HostCacheLoader for the given hostname, using the provided logger for logging.
func NewHostCacheLoader(baseDir, hostname string, logger hclog.Logger) (*HostCacheLoader, error) {
	hostDir := filepath.Join(baseDir, normalizeHostname(hostname))
	if err := os.MkdirAll(hostDir, 0o766); err != nil {
		return nil, err
	}

	return &HostCacheLoader{
		dir:    hostDir,
		logger: logger.ResetNamed("hostcache").With("hostname", hostname),
	}, nil
}

func (f FileID) valid() bool {
	return string(f) == path.Base(string(f))
}

// ReadOrRefresh checks if the cached data for the given fileID is still valid by calling the
// provided refresh function with the modification time of the cached data. If the refresh function
// returns nil data, the cached data is still valid and will be returned. If the refresh function
// returns new data, it will be written to the cache and returned. An error is returned if there
// is an issue checking the cache, refreshing the data, or writing new data to the cache.
func (h HostCacheLoader) ReadOrRefresh(fileID FileID, check CheckRefreshFunc) ([]byte, error) {
	if !fileID.valid() {
		return nil, ErrInvalidFileID
	}

	mTime, err := h.mtime(fileID)
	if err != nil {
		return nil, err
	}

	if mTime != nil {
		h.logger.Debug("Checking remote against cache", "file", string(fileID), "modTime", mTime)
	} else {
		h.logger.Debug("No cached file found, fetching new data", "file", string(fileID))
	}

	result := check(mTime)
	if result.Err != nil {
		h.logger.Debug("Failed to check remote against cache", "file", string(fileID), "error", result.Err)
		return nil, result.Err
	}

	if result.DataIfNew == nil {
		h.logger.Debug("Cached file is up-to-date", "file", string(fileID))
		// This means the data is not newer than what is cached, read from cache
		return h.read(fileID)
	}

	h.logger.Debug("Cached file is stale; updating cache", "file", string(fileID))
	if err := h.Write(fileID, result.DataIfNew, result.LastModified); err != nil {
		return nil, err
	}

	return result.DataIfNew, nil
}

func (h HostCacheLoader) mtime(fileID FileID) (*time.Time, error) {
	path := filepath.Join(h.dir, string(fileID))
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	modTime := info.ModTime()
	return &modTime, nil
}

// Write writes the given data to the cache with the given fileID, overwriting any existing data.
func (h HostCacheLoader) Write(fileID FileID, data []byte, lastModified *time.Time) error {
	if !fileID.valid() {
		return ErrInvalidFileID
	}

	path := filepath.Join(h.dir, string(fileID))
	err := os.WriteFile(path, data, 0o666)
	if err != nil {
		return err
	}

	if lastModified != nil {
		return os.Chtimes(path, time.Now(), *lastModified)
	}

	return nil
}

func (h HostCacheLoader) read(fileID FileID) ([]byte, error) {
	path := filepath.Join(h.dir, string(fileID))
	return os.ReadFile(path)
}
