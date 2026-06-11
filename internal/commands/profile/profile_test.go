// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfile_IsValidProperty(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	r.ErrorContains(IsValidProperty("defealt_organization"), "default_organization")
	r.ErrorContains(IsValidProperty("no_colr"), "no_color")
}
