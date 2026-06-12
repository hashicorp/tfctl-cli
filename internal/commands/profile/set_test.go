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

func TestSet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name         string
		Property     string
		Value        string
		Error        string
		SetupProfile func(p *profile.Profile) // Add setup function
		CheckProfile func(p *profile.Profile, r *require.Assertions)
	}{
		{
			Name:     "can't set name",
			Property: "name",
			Value:    "test",
			Error:    "to update a profile name use tfctl profile profiles rename",
		},
		{
			Name:     "invalid top-level key",
			Property: "unknown-top-level",
			Value:    "test",
			Error:    "property with name \"unknown-top-level\" does not exist",
		},
		{
			Name:     "basic top-level property",
			Property: "default_organization",
			Value:    "123",
			CheckProfile: func(p *profile.Profile, r *require.Assertions) {
				r.Equal("123", p.DefaultOrganization)
			},
		},
		{
			Name:     "basic core property",
			Property: "no_color",
			Value:    "true",
			CheckProfile: func(p *profile.Profile, r *require.Assertions) {
				r.True(*p.NoColor)
			},
		},
		{
			Name:     "basic core property - invalid type conversion",
			Property: "no_color",
			Value:    "bad-value",
			Error:    "cannot parse 'no_color' as bool",
		},
		{
			Name:     "hostname change clears org and project",
			Property: "hostname",
			Value:    "app.eu.terraform.io",
			SetupProfile: func(p *profile.Profile) {
				// Set initial org and project values
				p.DefaultOrganization = "test-org-123"
				p.Token = "test"
			},
			CheckProfile: func(p *profile.Profile, r *require.Assertions) {
				// Verify geography is set and org/project are cleared
				r.Equal("app.eu.terraform.io", p.Hostname)
				r.Equal("", p.DefaultOrganization)
				r.Equal("", p.Token)
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			io := iostreams.Test()
			profile := profile.TestProfile(t)

			// Setup profile if needed
			if c.SetupProfile != nil {
				c.SetupProfile(profile)
			}

			o := &SetOpts{
				IO:       io,
				Profile:  profile,
				Property: c.Property,
				Value:    c.Value,
			}

			err := setRun(context.Background(), o)
			if c.Error == "" {
				r.NoError(err)
				if c.CheckProfile != nil {
					c.CheckProfile(o.Profile, r)
				}
			} else {
				r.ErrorContains(err, c.Error)
			}
		})
	}
}

func TestSet_Organization(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	l := profile.TestLoader(t)
	p := l.DefaultProfile()
	r.NoError(p.Write())
	o := &SetOpts{
		IO:       io,
		Profile:  p,
		Property: "default_organization",
	}

	setup := func(quiet, tty, authed bool, projectID string) {
		o.Value = projectID
		io.SetQuiet(quiet)
		io.InputTTY = tty
		io.ErrorTTY = tty
		io.Input.Reset()
		io.Error.Reset()
		io.Output.Reset()
	}

	checkOrg := func(expected string) {
		loadedProfile, err := l.LoadProfile(p.Name)
		r.NoError(err)
		r.Equal(expected, loadedProfile.DefaultOrganization)
	}

	// Run with quiet off, TTY's, authenticated, and return that the user has access to the project
	{
		setup(false, true, true, "123")
		r.NoError(setRun(context.Background(), o))
		checkOrg("123")
	}

}

func TestSetDryRun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	l := profile.TestLoader(t)
	io := iostreams.Test()
	p, err := l.NewProfile("test")
	r.NoError(err)
	p.DefaultOrganization = "original-org"
	r.NoError(p.Write())
	o := &SetOpts{
		IO:       io,
		Profile:  p,
		Property: "default_organization",
		Value:    "dry-run-org",
	}

	o.DryRun = true
	r.NoError(setRun(context.Background(), o))
	r.Equal("dry-run-org", o.Profile.DefaultOrganization)
	r.Contains(io.Error.String(), `would set profile property "default_organization" to "dry-run-org"`)

	reloaded, err := l.LoadProfile("test")
	r.NoError(err)
	r.Equal("original-org", reloaded.DefaultOrganization)
}
