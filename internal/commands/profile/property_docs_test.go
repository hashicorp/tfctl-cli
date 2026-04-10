// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

func TestProfile_AvailableProperties_Coverage(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()

	all := profile.PropertyNames()
	delete(all, "name")
	b := availableProperties(io)

	for component, properties := range b.properties {
		for property := range properties {
			name := fmt.Sprintf("%s/%s", component, property)
			if component == "" {
				name = property
			}

			delete(all, name)
		}
	}

	r.Empty(all, "A property was added to the profile without documentation.")
}
