// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"context"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/posener/complete"
	"github.com/stretchr/testify/require"
)

func TestPropertyNames(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	properties := PropertyNames()
	r.NotEmpty(properties)
	r.Contains(properties, "name")
	r.Contains(properties, "default_organization")
	r.Contains(properties, "token")
	r.Contains(properties, "hostname")
}

func TestProfile_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name    string
		Profile *Profile
		Error   string
	}{
		{
			Name:    "empty",
			Profile: &Profile{},
			Error:   "profile name may only include",
		},
		{
			Name: "name too long",
			Profile: &Profile{
				Name: strings.Repeat("test", 100),
			},
			Error: "profile name may only include",
		},
		{
			Name: "valid",
			Profile: &Profile{
				Name:                "test",
				DefaultOrganization: "123",
			},
			Error: "",
		},
	}

	for _, c := range cases {
		// Capture the test case
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			err := c.Profile.Validate()
			if c.Error == "" {
				r.NoError(err)
			} else {
				r.ErrorContains(err, c.Error)
			}
		})
	}
}

func TestProfile_Predict(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name     string
		Args     complete.Args
		Expected []string
	}{
		{
			Name: "empty",
			Args: complete.Args{
				All: []string{""},
			},
			Expected: []string{"default_organization", "no_color", "hostname", "token", "telemetry"},
		},
	}

	for _, c := range cases {
		// Capture the test case
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			// Create a profile
			p := &Profile{}

			// Predict
			out := p.Predict(c.Args)
			r.Equal(c.Expected, out)
		})
	}
}

func TestCore_Getters(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Instantiate a non-empty profile
	v := true
	p := &Profile{
		NoColor:      &v,
		tokenFromEnv: "token-from-env",
	}
	r.Equal(v, *p.NoColor)
	r.Equal(DefaultHostname, p.GetHostname())
	r.Equal("token-from-env", p.GetToken())
}

func TestProfile_HostCache(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create a profile with an invalid hostname to force an error when getting the host cache.
	p := &Profile{
		Hostname:     "example.com",
		hostCacheDir: t.TempDir(),
	}
	h, err := p.HostCache(context.Background())
	r.NoError(err)

	now := time.Now()
	err = h.Write(FileID("test.json"), []byte(`{"ok":true}`), &now)
	r.NoError(err)

	r.FileExists(path.Join(h.dir, "test.json"))
}
