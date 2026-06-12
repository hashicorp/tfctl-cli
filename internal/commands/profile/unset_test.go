// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestUnset(t *testing.T) {
	t.Parallel()
	defaultProfile := func(p *profile.Profile) {
		p.DefaultOrganization = "123"
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
			Error:    "to update a profile name use tfctl profile profiles rename",
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
			Property: "default_organization",
			CheckProfile: func(p *profile.Profile, r *require.Assertions) {
				r.Empty(p.DefaultOrganization)
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

			err = unsetRun(context.Background(), o)
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

func TestUnsetDryRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	p, err := l.NewProfile("test")
	r.NoError(err)
	p.DefaultOrganization = "keep-me"
	r.NoError(p.Write())

	io := iostreams.Test()
	o := &UnsetOpts{
		IO:       io,
		Profile:  p,
		Profiles: l,
		Property: "default_organization",
	}

	o.DryRun = true
	r.NoError(unsetRun(context.Background(), o))
	r.Contains(io.Error.String(), `would unset profile property "default_organization"`)

	reloaded, err := l.LoadProfile("test")
	r.NoError(err)
	r.Equal("keep-me", reloaded.DefaultOrganization)
}
