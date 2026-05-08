// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestList(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	io := iostreams.Test()
	l := profile.TestLoader(t)
	output := format.New(io)

	opts := &ListOpts{
		IO:       io,
		Output:   output,
		Profiles: l,
	}

	// Create a few profiles
	p1, err := l.NewProfile("alpha")
	r.NoError(err)
	p1.Organization = "alpha-org-id"
	r.NoError(p1.Write())

	p2, err := l.NewProfile("beta")
	r.NoError(err)
	p2.Organization = "beta-org-id"
	r.NoError(p2.Write())

	p3, err := l.NewProfile("zed")
	r.NoError(err)
	p3.Organization = "zed-org-id"
	r.NoError(p3.Write())

	// Set beta as active
	active, err := l.GetActiveProfile()
	r.NoError(err)
	active.Name = "beta"
	r.NoError(active.Write())

	// Call list
	r.NoError(listRun(opts))

	// Check we got the output we expected
	expected := [][]string{
		{"Name", "Active", "Organization"},
		{p1.Name, "false", p1.Organization},
		{p2.Name, "true", p2.Organization},
		{p3.Name, "false", p3.Organization},
	}

	lines := strings.Split(io.Output.String(), "\n")
	r.Len(lines, 5)
	r.Empty(lines[4])
	for i, expectedFields := range expected {
		for _, field := range expectedFields {
			r.Contains(lines[i], field)
		}
	}

}
