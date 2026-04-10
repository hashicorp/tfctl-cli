// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"testing"

	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
	"github.com/stretchr/testify/require"
)

func TestDisplay(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	p := profile.TestProfile(t)
	p.Organization = "123"
	p.Hostname = "app.eu.terraform.io"
	p.NoColor = new(bool)

	t.Run("default", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		opts := &DisplayOpts{
			IO:      io,
			Profile: p,
		}
		r.NoError(displayRun(opts))
		r.Contains(io.Output.String(), "Hostname")
		r.Contains(io.Output.String(), "No Color")
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		opts := &DisplayOpts{
			IO:      io,
			Profile: p,
			Format:  format.JSON,
		}
		r.NoError(displayRun(opts))
		r.Contains(io.Output.String(), "hostname")
		r.Contains(io.Output.String(), "no_color")
	})
}
