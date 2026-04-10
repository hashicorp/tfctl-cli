// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

func TestActivate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name     string
		Active   string
		Create   []string
		Activate string
		Error    string
	}{
		{
			Name:     "Activate non-existent profile",
			Active:   "foo",
			Create:   []string{"foo", "bar"},
			Activate: "baz",
			Error:    "profile \"baz\" does not exist",
		},
		{
			Name:     "Activate currently active profile",
			Active:   "foo",
			Create:   []string{"foo", "bar"},
			Activate: "foo",
			Error:    "profile \"foo\" is already the active profile",
		},
		{
			Name:     "Activate good",
			Active:   "foo",
			Create:   []string{"foo", "bar"},
			Activate: "bar",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)
			l := profile.TestLoader(t)
			io := iostreams.Test()

			// Create the profiles
			for _, name := range c.Create {
				p, err := l.NewProfile(name)
				r.NoError(err)
				r.NoError(p.Write())
			}

			// Mark the correct profile as active
			active, err := l.GetActiveProfile()
			r.NoError(err)
			active.Name = c.Active
			r.NoError(active.Write())

			opts := &ActivateOpts{
				IO:       io,
				Profiles: l,
				Name:     c.Activate,
			}

			err = activateRun(opts)
			if c.Error != "" {
				r.ErrorContains(err, c.Error)
				return
			}

			r.NoError(err)

			// Check we activated properly
			newActive, err := l.GetActiveProfile()
			r.NoError(err)
			r.Equal(c.Activate, newActive.Name)
		})
	}
}
