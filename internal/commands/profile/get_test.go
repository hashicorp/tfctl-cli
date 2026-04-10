// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"strings"
	"testing"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	p := profile.TestProfile(t)
	p.Organization = "123"
	p.NoColor = new(bool)

	*p.NoColor = true

	expect := map[string]string{
		"organization_id": "123",
		"no_color":        "true",
	}

	for k, v := range expect {
		opts := &GetOpts{
			IO:       io,
			Profile:  p,
			Property: k,
		}
		r.NoError(getRun(opts))
		r.Equal(strings.TrimSpace(io.Output.String()), v)
		io.Output.Reset()
	}

	// Get an unset property
	opts := &GetOpts{
		IO:       io,
		Profile:  p,
		Property: "verbosity",
	}
	r.ErrorContains(getRun(opts), "property \"verbosity\" is not set")
}
