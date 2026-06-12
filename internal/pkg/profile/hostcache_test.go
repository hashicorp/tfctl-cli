// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestFileID_valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		id    FileID
		valid bool
	}{
		{"simple name", FileID("orgs.json"), true},
		{"with path separator", FileID("foo/bar.json"), false},
		{"parent traversal", FileID("../bar.json"), false},
		{"empty string", FileID(""), false},
		{"just a name", FileID("data"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)
			r.Equal(tc.valid, tc.id.valid())
		})
	}
}

func TestHostCacheLoader_Write(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir, logger: hclog.NewNullLogger()}
	now := time.Now()

	// Valid write
	err := loader.Write(FileID("test.json"), []byte(`{"ok":true}`), &now)
	r.NoError(err)

	content, err := os.ReadFile(filepath.Join(dir, "test.json"))
	r.NoError(err)
	r.Equal(`{"ok":true}`, string(content))

	// Invalid file ID
	err = loader.Write(FileID("sub/test.json"), []byte("data"), &now)
	r.ErrorIs(err, ErrInvalidFileID)
}

func TestHostCacheLoader_ReadOrRefresh_CacheMiss(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir, logger: hclog.NewNullLogger()}
	now := time.Now()

	// No cached file exists; check func receives nil mTime and returns new data
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) RefreshResult {
		r.Nil(mTime)
		return RefreshResult{
			DataIfNew:    []byte("fresh"),
			LastModified: &now,
		}
	})
	r.NoError(err)
	r.Equal([]byte("fresh"), data)

	// Verify it was written to cache
	cached, err := os.ReadFile(filepath.Join(dir, "orgs.json"))
	r.NoError(err)
	r.Equal([]byte("fresh"), cached)
}

func TestHostCacheLoader_ReadOrRefresh_CacheHit(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir, logger: hclog.NewNullLogger()}

	// Pre-populate cache
	err := os.WriteFile(filepath.Join(dir, "orgs.json"), []byte("cached"), 0o666)
	r.NoError(err)

	// check func returns nil meaning cache is still valid
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) RefreshResult {
		r.NotNil(mTime)
		return RefreshResult{}
	})
	r.NoError(err)
	r.Equal([]byte("cached"), data)
}

func TestHostCacheLoader_ReadOrRefresh_Refresh(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir, logger: hclog.NewNullLogger()}
	now := time.Now()

	// Pre-populate cache with old data
	err := os.WriteFile(filepath.Join(dir, "orgs.json"), []byte("old"), 0o666)
	r.NoError(err)

	// check func returns new data
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) RefreshResult {
		r.NotNil(mTime)
		return RefreshResult{
			DataIfNew:    []byte("new"),
			LastModified: &now,
		}
	})
	r.NoError(err)
	r.Equal([]byte("new"), data)

	// Verify cache was updated
	cached, err := os.ReadFile(filepath.Join(dir, "orgs.json"))
	r.NoError(err)
	r.Equal([]byte("new"), cached)
}

func TestHostCacheLoader_ReadOrRefresh_CheckError(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir, logger: hclog.NewNullLogger()}

	checkErr := errors.New("upstream failed")
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) RefreshResult {
		return RefreshResult{
			Err: checkErr,
		}
	})
	r.ErrorIs(err, checkErr)
	r.Nil(data)
}

func TestHostCacheLoader_ReadOrRefresh_InvalidFileID(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir, logger: hclog.NewNullLogger()}
	now := time.Now()

	data, err := loader.ReadOrRefresh(FileID("../escape.json"), func(mTime *time.Time) RefreshResult {
		return RefreshResult{
			DataIfNew:    []byte("data"),
			LastModified: &now,
		}
	})
	r.ErrorIs(err, ErrInvalidFileID)
	r.Nil(data)
}
