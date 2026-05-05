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
