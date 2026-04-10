// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"fmt"
	"testing"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
	"github.com/stretchr/testify/require"
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
