// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"testing"

	"github.com/hashicorp/tfcloud/internal/iostreams"
	"github.com/stretchr/testify/require"
)

func TestAuthErrorHelp(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()

	commandPath := "tfcloud example"
	args := []string{"simple", "'single-quote'", `escaped \"inner\"`}

	// Get the help text
	helpText := authErrorHelp(io, commandPath, args)
	r.Contains(helpText, `$ tfcloud example simple 'single-quote' "escaped \\\"inner\\\""`)
}
