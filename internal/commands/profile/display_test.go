// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestDisplay(t *testing.T) {
	t.Parallel()

	p := profile.TestProfile(t)
	p.DefaultOrganization = "123"
	p.Hostname = "app.eu.terraform.io"
	p.NoColor = func() *bool { b := true; return &b }()

	t.Run("default", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		io := iostreams.Test()

		opts := &DisplayOpts{
			IO:      io,
			Profile: p,
			Output:  format.New(io),
		}

		r.NoError(displayRun(opts))
		r.Contains(io.Output.String(), "hostname")
		r.Contains(io.Output.String(), "no_color")
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		io := iostreams.Test()
		output := format.New(io)
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
