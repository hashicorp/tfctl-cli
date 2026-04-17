// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

func TestDisplay(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	output := format.New(io)
	p := profile.TestProfile(t)
	p.Organization = "123"
	p.Hostname = "app.eu.terraform.io"
	p.NoColor = func() *bool { b := true; return &b }()

	t.Run("default", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		opts := &DisplayOpts{
			IO:      io,
			Profile: p,
			Output:  output,
		}

		r.NoError(displayRun(opts))
		r.Contains(io.Output.String(), "hostname")
		r.Contains(io.Output.String(), "no_color")
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)

		output.SetFormat(format.JSON)

		opts := &DisplayOpts{
			IO:      io,
			Profile: p,
			Output:  output,
		}
		r.NoError(displayRun(opts))
		r.Contains(io.Output.String(), "\"Hostname\": \"app.eu.terraform.io\"")
		r.Contains(io.Output.String(), "\"NoColor\": true")
	})
}
