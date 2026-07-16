// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package execsession manages short-lived, process-scoped permission grants
// that allow nested tfctl invocations to perform noninteractive deletes.
//
// This is a safety rail, not a security boundary: the granting process and any
// nested tfctl run as the same OS user, so the value provided is a deliberate
// human opt-in that auto-reverts when the session ends. A hard guarantee that an
// agent cannot delete must come from the API token scope server-side.
package execsession

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// EnvVar is the environment variable a wrapper sets so nested tfctl invocations
// can discover the active exec session token.
const EnvVar = "TFCTL_EXEC_SESSION"

// sessionVersion is the on-disk schema version for a session file.
const sessionVersion = 1

// tokenBytes is the number of random bytes in a session token before base32
// encoding (160 bits of entropy).
const tokenBytes = 20

// Decision reason codes returned by Authorizer implementations.
const (
	// ReasonNoSession indicates no session env var was set.
	ReasonNoSession = "no-session"
	// ReasonStale indicates the env var was set but the session file is gone.
	ReasonStale = "stale"
	// ReasonNotLive indicates the granting process is no longer alive.
	ReasonNotLive = "not-live"
	// ReasonClassNotGranted indicates the resource class was not permitted.
	ReasonClassNotGranted = "class-not-granted"
	// ReasonGranted indicates the delete is authorized.
	ReasonGranted = "granted"
)

// tokenPattern matches a well-formed session token (base32, no padding, upper).
// It is used to reject malformed/hostile tokens (e.g. path traversal) supplied
// via the environment before they are used to build a filesystem path.
var tokenPattern = regexp.MustCompile(`^[A-Z2-7]+$`)

var tokenEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Permissions is the set of capabilities granted to a session.
type Permissions struct {
	// AllowDelete holds normalized resource classes, and may contain the
	// reversible/all sentinels.
	AllowDelete []string
}

// Session is the on-disk record for an active grant.
type Session struct {
	Version     int      `hcl:"version"`
	Token       string   `hcl:"token"`
	PID         int      `hcl:"pid"`
	CreatedAt   string   `hcl:"created_at"`
	AllowDelete []string `hcl:"allow_delete"`
}

// Handle is a live grant held by the wrapper process. Close releases the lock
// and removes the file.
type Handle struct {
	token string
	file  *os.File
	path  string
}

// Token returns the session token to expose to descendant processes.
func (h *Handle) Token() string { return h.token }

// Close releases the advisory lock and removes the session file. It is safe to
// call once; subsequent calls are no-ops.
func (h *Handle) Close() error {
	if h == nil || h.file == nil {
		return nil
	}

	// Best-effort: always attempt to remove the file even if unlocking fails.
	_ = releaseLock(h.file)
	closeErr := h.file.Close()
	h.file = nil

	removeErr := os.Remove(h.path)
	if removeErr != nil && os.IsNotExist(removeErr) {
		removeErr = nil
	}

	if removeErr != nil {
		return removeErr
	}
	return closeErr
}

// Store abstracts the directory holding session files so tests can use a temp
// dir.
type Store struct {
	// Dir is the directory session files live in. The default is the
	// <config-dir>/exec subdirectory resolved by profile.ConfigDir.
	Dir string
}

// DefaultStore returns a Store rooted at <config-dir>/exec, creating the
// directory with 0700 permissions if needed. The config dir is resolved by
// profile.ConfigDir, so it honors TFCTL_CONFIG_DIR and stays consistent with
// where the rest of tfctl reads its configuration.
func DefaultStore() (*Store, error) {
	configDir, err := profile.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve exec session directory: %w", err)
	}
	dir := filepath.Join(configDir, "exec")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create exec session directory %q: %w", dir, err)
	}
	return &Store{Dir: dir}, nil
}

// path returns the session file path for a token. It returns ok=false if the
// token is malformed, so callers never build a path from hostile input.
func (s *Store) path(token string) (string, bool) {
	if !tokenPattern.MatchString(token) {
		return "", false
	}
	return filepath.Join(s.Dir, token+".hcl"), true
}

// Create issues a new token, writes the session file with 0600 permissions, and
// acquires a shared advisory lock held open in the returned Handle. The lock is
// held for the process lifetime so authorizers can detect liveness.
func (s *Store) Create(perms Permissions, pid int) (*Handle, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create exec session directory %q: %w", s.Dir, err)
	}

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	path, ok := s.path(token)
	if !ok {
		// Should be impossible: newToken only emits valid tokens.
		return nil, fmt.Errorf("generated invalid session token")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to create session file: %w", err)
	}

	if err := acquireLock(f); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("failed to lock session file: %w", err)
	}

	sess := &Session{
		Version:     sessionVersion,
		Token:       token,
		PID:         pid,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		AllowDelete: perms.AllowDelete,
	}

	hf := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(sess, hf.Body())
	if _, err := f.Write(hf.Bytes()); err != nil {
		_ = releaseLock(f)
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("failed to write session file: %w", err)
	}

	return &Handle{token: token, file: f, path: path}, nil
}

// Load reads and decodes the session for token. A missing or malformed token
// returns an error satisfying os.IsNotExist.
func (s *Store) Load(token string) (*Session, error) {
	path, ok := s.path(token)
	if !ok {
		// Return the sentinel unwrapped so os.IsNotExist treats a malformed
		// token the same as a missing file (both mean "no usable session").
		return nil, os.ErrNotExist
	}

	if _, err := os.Stat(path); err != nil {
		return nil, err // preserves os.IsNotExist for missing files
	}

	var sess Session
	if err := hclsimple.DecodeFile(path, nil, &sess); err != nil {
		return nil, fmt.Errorf("failed to decode session file: %w", err)
	}
	return &sess, nil
}

// newToken returns a fresh base32 (no padding, upper) token with 160 bits of
// entropy.
func newToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate session token: %w", err)
	}
	return tokenEncoding.EncodeToString(buf), nil
}

// Authorizer reports whether a noninteractive DELETE of a resource class is
// permitted by an active, live session. It is the seam the api command depends
// on so its behavior is testable.
type Authorizer interface {
	AuthorizeDelete(class string) (Decision, error)
}

// Decision is the outcome of an authorization check.
type Decision struct {
	// Allowed reports whether the delete may proceed without a prompt.
	Allowed bool
	// Token is the session token, surfaced for audit logging (empty if none).
	Token string
	// Reason is a machine-ish explanation; see the Reason* constants.
	Reason string
}

// LivenessFn reports whether the process that granted the session file at path
// is still alive. It is the seam that lets authorization be tested without real
// processes. It reports alive=true when the granting process still holds its
// shared lock on the file, and alive=false once that lock has been released
// (the process exited or was killed).
type LivenessFn func(path string) (alive bool, err error)

// EnvAuthorizer is the runtime Authorizer. It reads the session token from the
// environment, loads the session, and verifies the granting process is still
// alive before checking the granted classes.
type EnvAuthorizer struct {
	Store    *Store
	Getenv   func(string) string // default os.Getenv
	Liveness LivenessFn          // default probeLiveness
}

func (a *EnvAuthorizer) getenv(key string) string {
	if a.Getenv != nil {
		return a.Getenv(key)
	}
	return os.Getenv(key)
}

func (a *EnvAuthorizer) liveness() LivenessFn {
	if a.Liveness != nil {
		return a.Liveness
	}
	return probeLiveness
}

// AuthorizeDelete implements Authorizer.
func (a *EnvAuthorizer) AuthorizeDelete(class string) (Decision, error) {
	token := a.getenv(EnvVar)
	if token == "" {
		return Decision{Allowed: false, Reason: ReasonNoSession}, nil
	}

	sess, err := a.Store.Load(token)
	if err != nil {
		if os.IsNotExist(err) {
			// Env present but file gone: a dead or cleaned-up session.
			return Decision{Allowed: false, Reason: ReasonStale}, nil
		}
		return Decision{}, fmt.Errorf("failed to load exec session: %w", err)
	}

	// Liveness: the granting process holds a shared lock on the session file for
	// its lifetime. If we can take an exclusive lock, the holder is gone and the
	// grant no longer applies. The token itself is the authorization boundary;
	// possession plus a live holder is sufficient.
	path, ok := a.Store.path(token)
	if !ok {
		// Load already validated the token, so this should be unreachable.
		return Decision{Allowed: false, Reason: ReasonStale}, nil
	}
	alive, err := a.liveness()(path)
	if err != nil {
		return Decision{}, fmt.Errorf("failed to check exec session liveness: %w", err)
	}
	if !alive {
		return Decision{Allowed: false, Token: sess.Token, Reason: ReasonNotLive}, nil
	}

	if !AllowsDelete(sess.AllowDelete, class) {
		return Decision{Allowed: false, Token: sess.Token, Reason: ReasonClassNotGranted}, nil
	}

	return Decision{Allowed: true, Token: sess.Token, Reason: ReasonGranted}, nil
}
