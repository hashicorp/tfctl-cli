// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"testing"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
	"github.com/stretchr/testify/require"
)

func TestUnset(t *testing.T) {
	t.Parallel()
	defaultProfile := func(p *profile.Profile) {
		p.Organization = "123"
		info := "info"
		p.Verbosity = &info
	}

	cases := []struct {
		Name         string
		Property     string
		CheckProfile func(p *profile.Profile, r *require.Assertions)
		Error        string
	}{
		{
			Name:     "can't set name",
			Property: "name",
			Error:    "to update a profile name use tfcloud profile profiles rename",
		},
		{
			Name:     "unset invalid top-level property",
			Property: "random",
			Error:    "property with name \"random\" does not exist",
		},
		{
			Name:     "unset top-level",
			Property: "hostname",
			CheckProfile: func(p *profile.Profile, r *require.Assertions) {
				r.Empty(p.Hostname)
			},
		},
		{
			Name:     "unset basic property",
			Property: "verbosity",
			CheckProfile: func(p *profile.Profile, r *require.Assertions) {
				r.Nil(p.Verbosity)
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			// Create a profile loader and generate the starting profile
			l := profile.TestLoader(t)
			p, err := l.NewProfile("test")
			r.NoError(err)

			defaultProfile(p)
			r.NoError(p.Write())

			io := iostreams.Test()
			o := &UnsetOpts{
				IO:       io,
				Profile:  p,
				Profiles: l,
				Property: c.Property,
			}

			err = unsetRun(o)
			if c.Error != "" {
				r.ErrorContains(err, c.Error)
				return
			}

			// Ensure there is no error
			r.NoError(err)

			// Load the profile from disk
			reread, err := l.LoadProfile("test")
			r.NoError(err)
			c.CheckProfile(reread, r)
		})
	}
}
