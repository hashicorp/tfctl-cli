// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"testing"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
	"github.com/stretchr/testify/require"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	l := profile.TestLoader(t)
	io := iostreams.Test()

	p1, p2 := "test", "test-other"
	opts := &CreateOpts{
		IO:         io,
		Profiles:   l,
		Name:       p1,
		NoActivate: false,
	}

	r.NoError(createRun(opts))
	r.Contains(io.Error.String(), "created")
	r.Contains(io.Error.String(), "activated")

	// Set no activate
	opts.Name = p2
	opts.NoActivate = true
	io.Error.Reset()
	r.NoError(createRun(opts))
	r.Contains(io.Error.String(), "created")
	r.NotContains(io.Error.String(), "activated")

	// Get the written profiles
	profiles, err := l.ListProfiles()
	r.NoError(err)
	r.Len(profiles, 2)
	r.Contains(profiles, p1)
	r.Contains(profiles, p2)
}
