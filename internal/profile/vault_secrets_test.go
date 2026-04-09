// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVaultSecretsConf_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name    string
		Profile *VaultSecretsConf
		Error   string
	}{
		{
			Name:    "nil",
			Profile: nil,
			Error:   "",
		},
		{
			Name:    "empty",
			Profile: &VaultSecretsConf{},
			Error:   "app must be set",
		},
	}

	for _, c := range cases {
		// Capture the test case
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			require := require.New(t)

			err := c.Profile.Validate()
			if c.Error == "" {
				require.NoError(err)
			} else {
				require.ErrorContains(err, c.Error)
			}
		})
	}
}

func TestVaultSecretsConf_IsEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name    string
		Profile *VaultSecretsConf
		Empty   bool
	}{
		{
			Name:    "nil",
			Profile: nil,
			Empty:   true,
		},
		{
			Name:    "empty",
			Profile: &VaultSecretsConf{},
			Empty:   true,
		},
		{
			Name: "empty",
			Profile: &VaultSecretsConf{
				AppName: "test",
			},
		},
	}

	for _, c := range cases {
		// Capture the test case
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			require := require.New(t)
			require.Equal(c.Empty, c.Profile.isEmpty())
		})
	}
}
