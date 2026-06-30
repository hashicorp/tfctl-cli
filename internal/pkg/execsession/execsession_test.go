// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package execsession

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateLoadRoundTrip(t *testing.T) {
	t.Parallel()

	store := &Store{Dir: t.TempDir()}
	perms := Permissions{AllowDelete: []string{"workspaces", "runs"}}

	handle, err := store.Create(perms, 4242)
	require.NoError(t, err)
	require.NotNil(t, handle)

	token := handle.Token()
	require.NotEmpty(t, token)
	assert.Regexp(t, `^[A-Z2-7]+$`, token)

	// The file is written with 0600 permissions.
	path := filepath.Join(store.Dir, token+".hcl")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Load round-trips the fields.
	sess, err := store.Load(token)
	require.NoError(t, err)
	assert.Equal(t, sessionVersion, sess.Version)
	assert.Equal(t, token, sess.Token)
	assert.Equal(t, 4242, sess.PID)
	assert.Equal(t, []string{"workspaces", "runs"}, sess.AllowDelete)
	assert.NotEmpty(t, sess.CreatedAt)

	// Close releases the lock and removes the file.
	require.NoError(t, handle.Close())
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "file should be removed after Close")
}

func TestStoreLoadMissing(t *testing.T) {
	t.Parallel()

	store := &Store{Dir: t.TempDir()}
	_, err := store.Load("MISSINGTOKEN234567ABCDEF")
	assert.True(t, os.IsNotExist(err), "missing token should be os.IsNotExist")
}

func TestStoreLoadInvalidTokenIsNotExist(t *testing.T) {
	t.Parallel()

	store := &Store{Dir: t.TempDir()}
	// A token outside the base32 alphabet (e.g. path traversal) must be
	// rejected without touching the filesystem path it implies.
	for _, bad := range []string{"../../etc/passwd", "lower-case", "has/slash", ""} {
		_, err := store.Load(bad)
		assert.Truef(t, os.IsNotExist(err), "invalid token %q should be os.IsNotExist", bad)
	}
}

func TestNewTokenFormatAndUniqueness(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		tok, err := newToken()
		require.NoError(t, err)
		assert.Regexp(t, `^[A-Z2-7]+$`, tok)
		assert.False(t, seen[tok], "tokens must be unique")
		seen[tok] = true
	}
}

func TestEnvAuthorizerAuthorizeDelete(t *testing.T) {
	t.Parallel()

	// newStoreWithSession creates a real session file in a temp dir and returns
	// the store plus the issued token. The handle is closed on cleanup.
	newStoreWithSession := func(t *testing.T, perms Permissions, pid int) (*Store, string) {
		t.Helper()
		store := &Store{Dir: t.TempDir()}
		h, err := store.Create(perms, pid)
		require.NoError(t, err)
		t.Cleanup(func() { _ = h.Close() })
		return store, h.Token()
	}

	t.Run("no env returns no-session", func(t *testing.T) {
		t.Parallel()
		a := &EnvAuthorizer{
			Store:    &Store{Dir: t.TempDir()},
			Getenv:   func(string) string { return "" },
			Self:     100,
			Ancestry: fakeAncestry(map[int]int{}),
		}
		d, err := a.AuthorizeDelete("workspaces")
		require.NoError(t, err)
		assert.False(t, d.Allowed)
		assert.Equal(t, ReasonNoSession, d.Reason)
		assert.Empty(t, d.Token)
	})

	t.Run("env set but file missing returns stale", func(t *testing.T) {
		t.Parallel()
		a := &EnvAuthorizer{
			Store:    &Store{Dir: t.TempDir()},
			Getenv:   func(string) string { return "GHOSTTOKEN234567ABCDEF" },
			Self:     100,
			Ancestry: fakeAncestry(map[int]int{}),
		}
		d, err := a.AuthorizeDelete("workspaces")
		require.NoError(t, err)
		assert.False(t, d.Allowed)
		assert.Equal(t, ReasonStale, d.Reason)
	})

	t.Run("pid not an ancestor is denied", func(t *testing.T) {
		t.Parallel()
		store, token := newStoreWithSession(t, Permissions{AllowDelete: []string{"workspaces"}}, 4242)
		a := &EnvAuthorizer{
			Store:    store,
			Getenv:   func(string) string { return token },
			Self:     100,
			Ancestry: fakeAncestry(map[int]int{100: 50, 50: 1}), // 4242 absent
		}
		d, err := a.AuthorizeDelete("workspaces")
		require.NoError(t, err)
		assert.False(t, d.Allowed)
		assert.Equal(t, ReasonNotAnAncestor, d.Reason)
		assert.Equal(t, token, d.Token, "token surfaced for audit even when denied")
	})

	t.Run("ancestor ok but class not granted is denied", func(t *testing.T) {
		t.Parallel()
		store, token := newStoreWithSession(t, Permissions{AllowDelete: []string{"runs"}}, 4242)
		a := &EnvAuthorizer{
			Store:    store,
			Getenv:   func(string) string { return token },
			Self:     100,
			Ancestry: fakeAncestry(map[int]int{100: 4242, 4242: 1}),
		}
		d, err := a.AuthorizeDelete("workspaces")
		require.NoError(t, err)
		assert.False(t, d.Allowed)
		assert.Equal(t, ReasonClassNotGranted, d.Reason)
		assert.Equal(t, token, d.Token)
	})

	t.Run("ancestor ok and class granted is allowed", func(t *testing.T) {
		t.Parallel()
		store, token := newStoreWithSession(t, Permissions{AllowDelete: []string{"workspaces"}}, 4242)
		a := &EnvAuthorizer{
			Store:    store,
			Getenv:   func(string) string { return token },
			Self:     100,
			Ancestry: fakeAncestry(map[int]int{100: 4242, 4242: 1}),
		}
		d, err := a.AuthorizeDelete("workspaces")
		require.NoError(t, err)
		assert.True(t, d.Allowed)
		assert.Equal(t, ReasonGranted, d.Reason)
		assert.Equal(t, token, d.Token)
	})

	t.Run("irreversible class with only reversible grant is denied", func(t *testing.T) {
		t.Parallel()
		store, token := newStoreWithSession(t, Permissions{AllowDelete: []string{SentinelReversible}}, 4242)
		a := &EnvAuthorizer{
			Store:    store,
			Getenv:   func(string) string { return token },
			Self:     100,
			Ancestry: fakeAncestry(map[int]int{100: 4242, 4242: 1}),
		}
		d, err := a.AuthorizeDelete("projects")
		require.NoError(t, err)
		assert.False(t, d.Allowed)
		assert.Equal(t, ReasonClassNotGranted, d.Reason)
	})

	t.Run("deep descendant is allowed", func(t *testing.T) {
		t.Parallel()
		store, token := newStoreWithSession(t, Permissions{AllowDelete: []string{"workspaces"}}, 4242)
		a := &EnvAuthorizer{
			Store:  store,
			Getenv: func(string) string { return token },
			Self:   100,
			// 100 -> 90 -> 80 -> 4242 -> 1
			Ancestry: fakeAncestry(map[int]int{100: 90, 90: 80, 80: 4242, 4242: 1}),
		}
		d, err := a.AuthorizeDelete("workspaces")
		require.NoError(t, err)
		assert.True(t, d.Allowed)
		assert.Equal(t, ReasonGranted, d.Reason)
	})
}
