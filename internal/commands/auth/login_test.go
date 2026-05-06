// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

func TestLoginFromStdin(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("my-test-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "Successfully logged in")
	r.Contains(io.Error.String(), p.Hostname)

	// Verify the token was persisted
	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal("my-test-token", loaded.Token)
}

func TestLoginFromStdin_CustomHostname(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = "tfe.example.com"
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("custom-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "Successfully logged in")
	r.Contains(io.Error.String(), "tfe.example.com")
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

	err := loginRun(opts)
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
	// Don't write anything to input

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	err := loginRun(opts)
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

	err := loginRun(opts)
	r.Error(err)
	r.Contains(err.Error(), "token is empty")
}

func TestLoginFromStdin_TokenWithWhitespace(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("  my-token-with-spaces  \n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(loginRun(opts))

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
	// Don't set InputTTY or ErrorTTY, so CanPrompt() returns false

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   false,
	}

	err := loginRun(opts)
	r.Error(err)
	r.Contains(err.Error(), "interactive login requires a terminal")
}

func TestLoginInteractive_Success(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
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

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "Opening browser")
	r.Contains(io.Error.String(), "https://app.terraform.io/app/settings/tokens")
	r.Contains(io.Error.String(), "Successfully logged in")

	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal("interactive-token", loaded.Token)
}

func TestLoginDefaultHostname(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	p.Hostname = ""
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("my-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "app.terraform.io")
}

func TestLoginFromStdin_DifferentProfile(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)

	// Create two profiles
	p1, err := l.NewProfile("production")
	r.NoError(err)
	p1.Hostname = "app.terraform.io"
	r.NoError(p1.Write())

	p2, err := l.NewProfile("staging")
	r.NoError(err)
	p2.Hostname = "tfe.staging.example.com"
	r.NoError(p2.Write())

	// Login to production profile
	io := iostreams.Test()
	io.Input.WriteString("prod-token\n")

	opts := &LoginOpts{
		IO:      io,
		Profile: p1,
		Token:   true,
	}

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "app.terraform.io")

	// Login to staging profile
	io = iostreams.Test()
	io.Input.WriteString("staging-token\n")

	opts = &LoginOpts{
		IO:      io,
		Profile: p2,
		Token:   true,
	}

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "tfe.staging.example.com")

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

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	// Record the initial token (may come from credentials file)
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

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "would save token")
	r.Contains(io.Error.String(), p.Name)

	// Verify the token was NOT changed on disk
	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal(initialToken, loaded.Token)
	r.NotEqual("my-new-token", loaded.Token)
}

func TestLoginInteractive_DryRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	// Record the initial token
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

	r.NoError(loginRun(opts))
	r.Contains(io.Error.String(), "would save token")
	r.Contains(io.Error.String(), p.Name)

	// Verify the token was NOT changed on disk
	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal(initialToken, loaded.Token)
	r.NotEqual("interactive-token", loaded.Token)
}

func TestLoginFromStdin_QuietMode(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())

	io := iostreams.Test()
	io.Input.WriteString("my-token\n")
	io.SetQuiet(true)

	opts := &LoginOpts{
		IO:      io,
		Profile: p,
		Token:   true,
	}

	r.NoError(loginRun(opts))

	// Quiet mode suppresses stderr output
	r.Empty(io.Error.String())

	// But the token is still saved
	loaded, err := l.LoadProfile(p.Name)
	r.NoError(err)
	r.Equal("my-token", loaded.Token)
}
