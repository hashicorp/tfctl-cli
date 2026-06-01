package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		{"dot", FileID("."), false},
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
	loader := HostCacheLoader{dir: dir}

	// Valid write
	err := loader.Write(FileID("test.json"), []byte(`{"ok":true}`))
	r.NoError(err)

	content, err := os.ReadFile(filepath.Join(dir, "test.json"))
	r.NoError(err)
	r.Equal(`{"ok":true}`, string(content))

	// Invalid file ID
	err = loader.Write(FileID("sub/test.json"), []byte("data"))
	r.ErrorIs(err, ErrInvalidFileID)
}

func TestHostCacheLoader_ReadOrRefresh_CacheMiss(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir}

	// No cached file exists; check func receives nil mTime and returns new data
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) ([]byte, error) {
		r.Nil(mTime)
		return []byte("fresh"), nil
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
	loader := HostCacheLoader{dir: dir}

	// Pre-populate cache
	err := os.WriteFile(filepath.Join(dir, "orgs.json"), []byte("cached"), 0o666)
	r.NoError(err)

	// check func returns nil meaning cache is still valid
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) ([]byte, error) {
		r.NotNil(mTime)
		return nil, nil
	})
	r.NoError(err)
	r.Equal([]byte("cached"), data)
}

func TestHostCacheLoader_ReadOrRefresh_Refresh(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir}

	// Pre-populate cache with old data
	err := os.WriteFile(filepath.Join(dir, "orgs.json"), []byte("old"), 0o666)
	r.NoError(err)

	// check func returns new data
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) ([]byte, error) {
		r.NotNil(mTime)
		return []byte("new"), nil
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
	loader := HostCacheLoader{dir: dir}

	checkErr := errors.New("upstream failed")
	data, err := loader.ReadOrRefresh(FileID("orgs.json"), func(mTime *time.Time) ([]byte, error) {
		return nil, checkErr
	})
	r.ErrorIs(err, checkErr)
	r.Nil(data)
}

func TestHostCacheLoader_ReadOrRefresh_InvalidFileID(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	dir := t.TempDir()
	loader := HostCacheLoader{dir: dir}

	data, err := loader.ReadOrRefresh(FileID("../escape.json"), func(mTime *time.Time) ([]byte, error) {
		return []byte("data"), nil
	})
	r.ErrorIs(err, ErrInvalidFileID)
	r.Nil(data)
}
