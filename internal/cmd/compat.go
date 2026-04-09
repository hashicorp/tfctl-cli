// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"fmt"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"
)

// Ensure we meet the cli interfaces
var _ cli.Command = &CompatibleCommand{}
var _ cli.CommandAutocomplete = &CompatibleCommand{}
var _ cli.CommandHelpTemplate = &CompatibleCommand{}

// CompatibleCommand is a compatibility layer for interopability with the `cli`
// package.
type CompatibleCommand struct {
	c *Command
}

// HelpTemplate implements cli.CommandHelpTemplate.
func (cc *CompatibleCommand) HelpTemplate() string {
	return `{{.Help}}`
}

// AutocompleteArgs implements cli.CommandAutocomplete.
func (cc *CompatibleCommand) AutocompleteArgs() complete.Predictor {
	return cc.c.Args.Autocomplete
}

// AutocompleteFlags implements cli.CommandAutocomplete.
func (cc *CompatibleCommand) AutocompleteFlags() complete.Flags {
	return cc.c.getAutocompleteFlags()
}

// Help implements cli.Command.
func (cc *CompatibleCommand) Help() string {
	return cc.c.help()
}

// Synopsis implements cli.Command.
func (cc *CompatibleCommand) Synopsis() string {
	return cc.c.ShortHelp
}

func (cc *CompatibleCommand) Run(args []string) int {
	return cc.c.Run(args)
}

// ToCommandMap converts a Command and its children to a hashicorp/cli command
// factory map. The passed Command should be the
// root command.
func ToCommandMap(c *Command) map[string]cli.CommandFactory {
	m := make(map[string]cli.CommandFactory)
	for _, child := range c.children {
		toCommandMap("", child, m)
	}

	return m
}

func toCommandMap(parent string, c *Command, m map[string]cli.CommandFactory) {
	// allNames is the commands name and all aliases.
	allNames := map[string]struct{}{c.Name: {}}
	for _, a := range c.Aliases {
		allNames[a] = struct{}{}
	}

	for name := range allNames {
		path := name
		if parent != "" {
			path = fmt.Sprintf("%s %s", parent, name)
		}

		m[path] = func() (cli.Command, error) {
			return &CompatibleCommand{
				c: c,
			}, nil
		}

		for _, child := range c.children {
			toCommandMap(path, child, m)
		}
	}
}

// RootHelpFunc returns a help function that meets the hashicorp/cli interface
// for help functions.
func RootHelpFunc(c *Command) func(map[string]cli.CommandFactory) string {
	return func(map[string]cli.CommandFactory) string {
		return c.help()
	}
}
