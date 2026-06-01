package profile

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"time"
)

// HostCacheLoader is responsible for loading and refreshing cached data for a host.
type HostCacheLoader struct {
	dir string
}

// FileID is an identifier for a specific cache file. It should only contain the file name
// and no path components.
type FileID string

// ErrInvalidFileID is returned when a FileID contains path components, is a directory, or is
// otherwise invalid.
var ErrInvalidFileID = errors.New("invalid file ID: must be a file name without path components")

// CheckRefreshFunc checks the upstream source for new data, returning nil if the
// cached data is still valid, or the new data if it is not. Return an error if
// the refresh check fails.
type CheckRefreshFunc func(mTime *time.Time) ([]byte, error)

func (f FileID) valid() bool {
	isFilename := string(f) == path.Base(string(f))
	if !isFilename {
		return false
	}

	info, err := os.Lstat(string(f))
	if err != nil {
		return true
	}

	return !info.IsDir()
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

	data, err := check(mTime)
	if err != nil {
		return nil, err
	}

	if data == nil {
		// This means the data is not newer than what is cached, read from cache
		return h.read(fileID)
	}

	if err := h.Write(fileID, data); err != nil {
		return nil, err
	}

	return data, nil
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
func (h HostCacheLoader) Write(fileID FileID, data []byte) error {
	if !fileID.valid() {
		return ErrInvalidFileID
	}

	path := filepath.Join(h.dir, string(fileID))
	return os.WriteFile(path, data, 0o666)
}

func (h HostCacheLoader) read(fileID FileID) ([]byte, error) {
	path := filepath.Join(h.dir, string(fileID))
	return os.ReadFile(path)
}
