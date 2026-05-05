// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package auth

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrowserCmd(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	bin, args := browserCmd("https://example.com/token")

	switch runtime.GOOS {
	case "darwin":
		r.Equal("open", bin)
	default:
		r.Equal("xdg-open", bin)
	}

	r.Equal([]string{"https://example.com/token"}, args)
}

func TestBrowserCmd_URLPreserved(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	url := "https://tfe.example.com/app/settings/tokens?source=cli"
	_, args := browserCmd(url)
	r.Equal([]string{url}, args)
}
