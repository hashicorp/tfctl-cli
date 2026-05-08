// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestAuthErrorHelp(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()

	commandPath := "tfctl example"
	args := []string{"simple", "'single-quote'", `escaped \"inner\"`}

	// Get the help text
	helpText := authErrorHelp(io, commandPath, args)
	r.Contains(helpText, `$ tfctl example simple 'single-quote' "escaped \\\"inner\\\""`)
}
