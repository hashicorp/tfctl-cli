// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// newFakeTFE returns an httptest.Server that mimics the TFE /account/details endpoint.
// If username is empty, it returns a 401.
func newFakeTFE(t *testing.T, username string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/account/details", func(w http.ResponseWriter, r *http.Request) {
		if username == "" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"errors":[{"status":"401","title":"unauthorized"}]}`)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		fmt.Fprintf(w, `{"data":{"id":"user-abc","type":"users","attributes":{"username":%q}}}`, username)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runLogin mimics the RunF flow by creating a cmd.Invocation and calling loginRun.
// It injects a no-op browser opener into the options so tests never launch a
// real browser and never mutate shared package state (keeping them race-free
// under t.Parallel()).
func runLogin(t *testing.T, opts *LoginOpts) error {
	t.Helper()
	if opts.OpenBrowser == nil {
		opts.OpenBrowser = func(string) error { return nil }
	}
	inv := &cmd.Invocation{
		IO:          opts.IO,
		Profile:     opts.Profile,
		ShutdownCtx: context.Background(),
	}
	return loginRun(inv.ShutdownCtx, inv, opts)
}

func TestLoginFromStdin(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "testuser")
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("my-test-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(runLogin(t, opts))
	r.Contains(io.Error.String(), "Successfully logged in")
	r.Contains(io.Error.String(), "testuser")

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal("my-test-token", loaded.Token)
}

func TestLoginFromStdin_CustomHostname(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "admin")
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("custom-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(runLogin(t, opts))
	r.Contains(io.Error.String(), "Successfully logged in")
	r.Contains(io.Error.String(), srv.URL)
}

func TestLoginFromStdin_EmptyToken(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	err := runLogin(t, opts)
	r.Error(err)
	r.Contains(err.Error(), "token is empty")
}

func TestLoginFromStdin_NoInput(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	io := iostreams.Test()

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	err := runLogin(t, opts)
	r.Error(err)
	r.Contains(err.Error(), "no token provided on stdin")
}

func TestLoginFromStdin_WhitespaceToken(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("   \n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	err := runLogin(t, opts)
	r.Error(err)
	r.Contains(err.Error(), "token is empty")
}

func TestLoginFromStdin_TokenWithWhitespace(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "testuser")
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("  my-token-with-spaces  \n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(runLogin(t, opts))

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal("my-token-with-spaces", loaded.Token)
}

func TestLoginInteractive_NoTTY(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	io := iostreams.Test()

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   false,
	}

	err := runLogin(t, opts)
	r.Error(err)
	r.Contains(err.Error(), "interactive login requires a terminal")
}

func TestLoginInteractive_Success(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "interactive-user")
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	io := iostreams.Test()
	io.InputTTY = true
	io.ErrorTTY = true
	io.Input.WriteString("interactive-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   false,
	}

	r.NoError(runLogin(t, opts))
	r.Contains(io.Error.String(), "Opening browser")
	r.Contains(io.Error.String(), "Successfully logged in")
	r.Contains(io.Error.String(), "interactive-user")

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal("interactive-token", loaded.Token)
}

func TestLoginFromStdin_DifferentProfile(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "multi-user")
	l := profile.TestLoader(t)
	host := srv.URL

	p1, err := l.NewProfile("production")
	r.NoError(err)
	p1.Hostname = host
	r.NoError(p1.Write())

	p2, err := l.NewProfile("staging")
	r.NoError(err)
	p2.Hostname = host
	r.NoError(p2.Write())

	// Login to production
	io := iostreams.Test()
	io.Input.WriteString("prod-token\n")
	r.NoError(runLogin(t, &LoginOpts{IO: io, Profile: p1, Token: true}))

	// Login to staging
	io = iostreams.Test()
	io.Input.WriteString("staging-token\n")
	r.NoError(runLogin(t, &LoginOpts{IO: io, Profile: p2, Token: true}))

	// Verify tokens were saved to the correct profiles
	loadedProd, err := l.LoadProfile("production")
	r.NoError(err)
	r.Equal("prod-token", loadedProd.Token)

	loadedStaging, err := l.LoadProfile("staging")
	r.NoError(err)
	r.Equal("staging-token", loadedStaging.Token)
}

func TestLoginFromStdin_DryRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "testuser")
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	initial, err := l.LoadProfile(p.Name)
	r.NoError(err)
	initialToken := initial.Token

	io := iostreams.Test()
	io.Input.WriteString("my-new-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
		DryRun:  true,
	}

	r.NoError(runLogin(t, opts))
	r.Contains(io.Error.String(), "would save token")
	r.Contains(io.Error.String(), p.Name)

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal(initialToken, loaded.Token)
	r.NotEqual("my-new-token", loaded.Token)
}

func TestLoginInteractive_DryRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "testuser")
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	initial, err := l.LoadProfile(p.Name)
	r.NoError(err)
	initialToken := initial.Token

	io := iostreams.Test()
	io.InputTTY = true
	io.ErrorTTY = true
	io.Input.WriteString("interactive-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   false,
		DryRun:  true,
	}

	r.NoError(runLogin(t, opts))
	r.Contains(io.Error.String(), "would save token")

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal(initialToken, loaded.Token)
	r.NotEqual("interactive-token", loaded.Token)
}

func TestLoginFromStdin_QuietMode(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "testuser")
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("my-token\n")
	io.SetQuiet(true)

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(runLogin(t, opts))
	r.Empty(io.Error.String())

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal("my-token", loaded.Token)
}

func TestLoginFromStdin_VerifyFails(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	srv := newFakeTFE(t, "") // empty username → 401
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = srv.URL
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("bad-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	err := runLogin(t, opts)
	r.Error(err)
	r.Contains(err.Error(), "failed to verify token")

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.NotEqual("bad-token", loaded.Token)
}
