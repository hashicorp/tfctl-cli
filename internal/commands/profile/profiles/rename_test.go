// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"testing"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
	"github.com/stretchr/testify/require"
)

func TestRename(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name         string
		Active       string
		Create       []string
		ExistingName string
		NewName      string
		Error        string
	}{
		{
			Name:         "Rename to self",
			Active:       "foo",
			Create:       []string{"foo", "bar"},
			ExistingName: "bar",
			NewName:      "bar",
			Error:        "new name must be different from the existing name",
		},
		{
			Name:         "Rename to invalid",
			Active:       "foo",
			Create:       []string{"foo", "bar"},
			ExistingName: "bar",
			NewName:      "$ad",
			Error:        "invalid new name \"$ad\": profile name may only include",
		},
		{
			Name:         "Rename non-existent profile",
			Active:       "foo",
			Create:       []string{"foo", "bar"},
			ExistingName: "bad",
			NewName:      "new-name",
			Error:        "profile \"bad\" does not exist",
		},
		{
			Name:         "Rename currently active profile",
			Active:       "foo",
			Create:       []string{"foo", "bar"},
			ExistingName: "foo",
			NewName:      "baz",
		},
		{
			Name:         "Rename non-active",
			Active:       "foo",
			Create:       []string{"foo", "bar"},
			ExistingName: "bar",
			NewName:      "baz",
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

			opts := &RenameOpts{
				IO:           io,
				Profiles:     l,
				ExistingName: c.ExistingName,
				NewName:      c.NewName,
			}

			err = renameRun(opts)
			if c.Error != "" {
				r.ErrorContains(err, c.Error)
				return
			}

			r.NoError(err)

			newProfiles, err := l.ListProfiles()
			r.NoError(err)

			// Check we deleted the old name
			r.NotContains(newProfiles, c.ExistingName)

			// Check the new name exists
			r.Contains(newProfiles, c.NewName)

			// If the old was active, check we updated the active
			if c.Active == c.ExistingName {
				newActive, err := l.GetActiveProfile()
				r.NoError(err)
				r.Equal(newActive.Name, c.NewName)
			}
		})
	}
}
