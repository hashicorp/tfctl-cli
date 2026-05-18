// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"slices"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
)

func TestToCommandMap(t *testing.T) {
	r := require.New(t)
	t.Parallel()

	cCtx := &Context{
		IO: iostreams.Test(),
	}

	// Create a command tree
	root := &Command{
		Name: "tfctl",
	}
	c1 := &Command{
		Name:    "child1",
		Aliases: []string{"c1", "childOne"},
	}
	c2 := &Command{
		Name:    "child2",
		Aliases: []string{"c2", "childTwo"},
	}

	c1n1 := &Command{
		Name:    "nested1",
		Aliases: []string{"n1", "nestedOne"},
	}
	c1n2 := &Command{
		Name:    "nested2",
		Aliases: []string{"n2", "nestedTwo"},
	}

	c2n1 := &Command{
		Name:    "nested1",
		Aliases: []string{"n1", "nestedOne"},
	}
	c2n2 := &Command{
		Name: "nested2",
	}

	root.AddChild(c1)
	root.AddChild(c2)
	c1.AddChild(c1n1)
	c1.AddChild(c1n2)
	c2.AddChild(c2n1)
	c2.AddChild(c2n2)

	// Build the command map
	m := ToCommandMap(root, cCtx)

	// Expected values
	expectedCommands := []string{
		"child1",
		"c1",
		"childOne",

		"child2",
		"c2",
		"childTwo",

		"child1 nested1",
		"c1 nested1",
		"childOne nested1",

		"child1 n1",
		"c1 n1",
		"childOne n1",

		"child1 nestedOne",
		"c1 nestedOne",
		"childOne nestedOne",

		"child1 nested2",
		"c1 nested2",
		"childOne nested2",

		"child1 n2",
		"c1 n2",
		"childOne n2",

		"child1 nestedTwo",
		"c1 nestedTwo",
		"childOne nestedTwo",

		"child2 nested1",
		"c2 nested1",
		"childTwo nested1",

		"child2 n1",
		"c2 n1",
		"childTwo n1",

		"child2 nestedOne",
		"c2 nestedOne",
		"childTwo nestedOne",

		"child2 nested2",
		"c2 nested2",
		"childTwo nested2",
	}

	// Sort all
	slices.Sort(expectedCommands)

	actualCommands := maps.Keys(m)
	slices.Sort(actualCommands)

	// Check the actual and expected match
	r.Equal(expectedCommands, actualCommands, "commands")
}
