// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoader_New(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Test that we create the directory if it doesn't yet exist.
	dir := filepath.Join(t.TempDir(), "tfctl")
	l, err := newLoader(dir)
	r.NoError(err)
	r.NotNil(l)

	// Check the directory and the profiles sub-dir was created.
	r.DirExists(dir)
	r.DirExists(filepath.Join(dir, ProfileDir))
}

func TestLoader_GetActiveProfile(t *testing.T) {
	t.Parallel()

	t.Run("no active profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l, err := newLoader(t.TempDir())
		r.NoError(err)
		active, err := l.GetActiveProfile()
		r.Nil(active)
		r.ErrorIs(err, ErrNoActiveProfileFilePresent)
	})

	t.Run("empty active profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)
		active := l.DefaultActiveProfile()
		active.Name = ""
		r.NoError(active.Write())

		p, err := l.GetActiveProfile()
		r.Nil(p)
		r.ErrorIs(err, ErrActiveProfileFileEmpty)
	})

	t.Run("malformed active profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		// Write a bad active profile
		r.NoError(os.WriteFile(filepath.Join(l.configDir, ActiveProfileFileName), []byte("invalid!"), 0x777))

		// Read the malformed profile
		p, err := l.GetActiveProfile()
		r.Nil(p)
		r.Error(err)
	})

	t.Run("valid active profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		active := l.DefaultActiveProfile()
		active.Name = t.Name()
		r.NoError(active.Write())

		p, err := l.GetActiveProfile()
		r.NoError(err)
		r.Equal(t.Name(), p.Name)
	})

}

func TestLoader_ListProfiles(t *testing.T) {
	t.Parallel()

	validProfileNames := []string{"bar", "baz", "foo"}
	slices.Sort(validProfileNames)
	t.Run("empty profiles directory", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)
		profiles, err := l.ListProfiles()
		r.Empty(profiles)
		r.NoError(err)
	})

	t.Run("valid profiles", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		// Create some profiles
		for _, n := range validProfileNames {
			p, err := l.NewProfile(n)
			r.NoError(err)
			r.NoError(p.Write())
		}

		profiles, err := l.ListProfiles()
		slices.Sort(profiles)
		r.Equal(profiles, validProfileNames)
		r.NoError(err)
	})

	t.Run("one invalid profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		// Create some profiles
		for _, n := range validProfileNames {
			p, err := l.NewProfile(n)
			r.NoError(err)
			r.NoError(p.Write())
		}

		// Write an invalid file
		r.NoError(os.WriteFile(filepath.Join(l.configDir, ProfileDir, "not_a_profile.json"), []byte("invalid!"), 0x777))

		profiles, err := l.ListProfiles()
		r.Empty(profiles)
		r.ErrorContains(err, "unexpected non-hcl file")
	})
}

func TestLoader_LoadProfile(t *testing.T) {
	t.Parallel()

	t.Run("no profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		p, err := l.LoadProfile(context.Background(), "test")
		r.Nil(p)
		r.ErrorIs(err, ErrNoProfileFilePresent)
	})

	t.Run("invalid profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		// Write an invalid profile to disk
		name := "test"
		path := filepath.Join(l.configDir, ProfileDir, fmt.Sprintf("%s.hcl", name))
		r.NoError(os.WriteFile(path, []byte("invalid!"), 0x777))

		p, err := l.LoadProfile(context.Background(), name)
		r.Nil(p)
		r.ErrorContains(err, "failed to decode profile")
	})

	t.Run("mismatched profile name", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		// Write an invalid profile to disk
		name := "test"
		path := filepath.Join(l.configDir, ProfileDir, fmt.Sprintf("%s.hcl", name))
		r.NoError(os.WriteFile(path, []byte(`name = "other"
default_organization = "123"`,
		), 0x777))

		p, err := l.LoadProfile(context.Background(), name)
		r.Nil(p)
		r.ErrorContains(err, "profile path name does not match name in file")
	})

	t.Run("valid profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		p, err := l.NewProfile("test")
		r.NoError(err)
		p.DefaultOrganization = "123"
		r.NoError(p.Write())

		out, err := l.LoadProfile(context.Background(), p.Name)
		r.NotNil(out)
		r.Equal(p.Name, out.Name)
		r.Equal(p.DefaultOrganization, out.DefaultOrganization)
		r.NoError(err)
	})

	t.Run("invalid profile name", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		_, err := l.NewProfile("test!@#$")
		r.ErrorContains(err, "profile name may only include")
	})
}

func TestLoader_GetDeviceID(t *testing.T) {
	tmpDir := t.TempDir()
	l, err := newLoader(tmpDir)
	require.NoError(t, err)

	id := l.GetDeviceID(context.Background())
	require.NotEmpty(t, id)

	// device ID matches file contents
	id2 := l.GetDeviceID(context.Background())

	data, err := os.ReadFile(filepath.Join(tmpDir, "device_id"))
	require.NoError(t, err)
	require.Equal(t, id, string(data))
	require.Equal(t, id, id2)
}

func TestTerraformTokenEnvVar(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		hostname string
		expected string
	}{
		{
			name:     "hcp terraform hostname uses lowercase, matching terraform",
			hostname: "app.terraform.io",
			expected: "TF_TOKEN_app_terraform_io",
		},
		{
			name:     "mixed-case hostname is normalized to lowercase",
			hostname: "App.Terraform.IO",
			expected: "TF_TOKEN_app_terraform_io",
		},
		{
			name:     "hyphens are encoded as double underscores",
			hostname: "my-tfe.example.com",
			expected: "TF_TOKEN_my__tfe_example_com",
		},
		{
			name:     "invalid hostname returns an empty string",
			hostname: "invalid/hostname",
			expected: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, c.expected, terraformTokenEnvVar(c.hostname))
		})
	}
}

//nolint:paralleltest
func TestLoader_LoadProfileEnv(t *testing.T) {
	// These tests aren't parallel because they manipulate the environment
	// and can't run concurrently.

	//nolint:paralleltest
	t.Run("default profile, env set", func(t *testing.T) {
		t.Setenv(envVarOrganization, "xyz")

		r := require.New(t)
		l, err := newLoader(t.TempDir())
		r.NoError(err)
		prof := l.DefaultProfile(context.Background())

		r.Equal("xyz", prof.DefaultOrganization)
	})

	t.Run("default profile, hostname env set", func(t *testing.T) {
		t.Setenv(envVarHostname, "https://example.com/with/path")

		r := require.New(t)
		l, err := newLoader(t.TempDir())
		r.NoError(err)
		prof := l.DefaultProfile(context.Background())

		r.Equal("example.com", prof.Hostname)
	})

	t.Run("default profile, hostname with port env set", func(t *testing.T) {
		t.Setenv(envVarHostname, "example.com:8080")

		r := require.New(t)
		l, err := newLoader(t.TempDir())
		r.NoError(err)
		prof := l.DefaultProfile(context.Background())

		r.Equal("example.com:8080", prof.Hostname)
	})

	t.Run("default profile, invalid hostname env set", func(t *testing.T) {
		t.Setenv(envVarHostname, "example/youtube")

		r := require.New(t)
		l, err := newLoader(t.TempDir())
		r.NoError(err)
		prof := l.DefaultProfile(context.Background())

		r.Equal(DefaultHostname, prof.Hostname)
	})

	//nolint:paralleltest
	t.Run("valid active profile, env set", func(t *testing.T) {
		r := require.New(t)
		l := TestLoader(t)

		p, err := l.NewProfile("test")
		r.NoError(err)
		r.NoError(p.Write())

		t.Setenv(envVarOrganization, "xyz")

		out, err := l.LoadProfile(context.Background(), p.Name)
		r.NoError(err)
		r.NotNil(out)
		r.Equal("xyz", out.DefaultOrganization)
	})
}

func TestLoader_DeleteProfile(t *testing.T) {
	t.Parallel()

	t.Run("no profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		p, err := l.NewProfile("test")
		r.NoError(err)
		p.DefaultOrganization = "123"
		r.NoError(p.Write())

		r.NoError(l.DeleteProfile("test"))
	})

	t.Run("existing profile", func(t *testing.T) {
		t.Parallel()
		r := require.New(t)
		l := TestLoader(t)

		// Write an invalid profile to disk
		name := "test"
		path := filepath.Join(l.configDir, ProfileDir, fmt.Sprintf("%s.hcl", name))
		r.NoError(os.WriteFile(path, []byte("invalid!"), 0x777))

		p, err := l.LoadProfile(context.Background(), name)
		r.Nil(p)
		r.ErrorContains(err, "failed to decode profile")
	})
}
