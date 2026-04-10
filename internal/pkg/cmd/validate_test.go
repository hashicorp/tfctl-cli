// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
)

func getGoodCommand() *Command {
	var projectID string
	parent := &Command{
		Name:      "parent-cmd",
		Aliases:   []string{"parent"},
		ShortHelp: "This is a short help message.",
		LongHelp:  `The parent-cmd command group lets you do things.`,
		Flags: Flags{
			Persistent: []*Flag{
				{
					Name:         "project",
					Shorthand:    "p",
					Description:  "Project ID.",
					DisplayValue: "ID",
					Value:        flagvalue.Simple("", &projectID),
				},
			},
		},
		io: iostreams.Test(),
	}

	// Add a child command
	var count int
	child := &Command{
		Name:      "child-cmd",
		Aliases:   []string{"child"},
		ShortHelp: "This is a short help message.",
		LongHelp:  `The parent-cmd child-cmd command lets you do things.`,
		Flags: Flags{
			Local: []*Flag{
				{
					Name:         "count",
					Description:  "Count of things to print.",
					DisplayValue: "N",
					Value:        flagvalue.Simple(0, &count),
				},
			},
		},
		Args: PositionalArguments{
			Preamble: "This is a preamble.",
			Args: []PositionalArgument{
				{
					Name:          "TEXT",
					Documentation: "Text to repeatedly print.",
					Optional:      false,
					Repeatable:    false,
				},
				{
					Name:          "PREFIX",
					Documentation: "Prefix to prepend to the text.",
					Optional:      false,
					Repeatable:    false,
				},
			},
		},
		Examples: []Example{
			{
				Preamble: "This is an example invocation:",
				Command:  "$ tfcloud parent child --count 5",
			},
		},
		AdditionalDocs: []DocSection{
			{
				Title:         "More Details",
				Documentation: "This will explain everything.",
			},
		},
		RunF: func(cmd *Command, args []string) error {
			return nil
		},
	}
	parent.AddChild(child)

	return parent
}

func TestCommand_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		command func(c *Command)
		error   string
	}{
		{
			name:    "good",
			command: func(c *Command) {},
			error:   "",
		},
		{
			name: "no io",
			command: func(c *Command) {
				c.io = nil
			},
			error: "io not set on command or any parent command",
		},
		{
			name: "no runf or children",
			command: func(c *Command) {
				c.children[0].RunF = nil
			},
			error: "either RunF or Children must be set",
		},
		{
			name: "both runf and children",
			command: func(c *Command) {
				c.RunF = func(cmd *Command, args []string) error { return nil }
			},
			error: "both RunF and Children cannot be set",
		},
		{
			name: "command group has bad long help",
			command: func(c *Command) {
				// Force a parent since LongHelp verification is disabled for
				// the root command.
				c.parent = &Command{
					Name: "tfcloud",
				}
				c.LongHelp = "Bad prefix"
			},
			error: "invalid command long help prefix.\n\nWANT: \"The tfcloud parent-cmd command group\"\nGOT:",
		},
		{
			name: "command has bad long help",
			command: func(c *Command) {
				c.children[0].LongHelp = "Bad prefix"
			},
			error: "invalid command long help prefix.\n\nWANT: \"The parent-cmd child-cmd command\"\nGOT:",
		},
		{
			name: "siblings have conflicting names",
			command: func(c *Command) {
				child2 := *c.children[0]
				c.AddChild(&child2)
			},
			error: "child command name \"child-cmd\" used by a sibling name or alias",
		},
		{
			name: "siblings have conflicting aliases",
			command: func(c *Command) {
				child2 := *c.children[0]
				child2.Name = "child-two"
				child2.Aliases = []string{c.children[0].Name}
				child2.LongHelp = `The parent-cmd child-two command lets you do things.`
				c.AddChild(&child2)
			},
			error: "child command \"child-two\" has alias \"child-cmd\" already used by a sibling name or alias",
		},
		{
			name: "no name",
			command: func(c *Command) {
				c.Name = ""
			},
			error: "command name cannot be empty",
		},
		{
			name: "bad name characters",
			command: func(c *Command) {
				c.Name = "ThisIsBad"
			},
			error: "only lower case names with hyphens are allowed",
		},
		{
			name: "bad alias characters",
			command: func(c *Command) {
				c.Aliases = append(c.Aliases, "ThisIsBad")
			},
			error: "only lower case names with hyphens are allowed",
		},
		{
			name: "duplicate aliases",
			command: func(c *Command) {
				c.Aliases = append(c.Aliases, "good", "good")
			},
			error: "duplicate alias \"good\" found",
		},
		{
			name: "duplicate name and alias",
			command: func(c *Command) {
				c.Aliases = append(c.Aliases, c.Name)
			},
			error: "command name cannot be an alias",
		},
		{
			name: "no short help",
			command: func(c *Command) {
				c.ShortHelp = ""
			},
			error: "short and long help text must be set",
		},
		{
			name: "no long help",
			command: func(c *Command) {
				c.LongHelp = ""
			},
			error: "short and long help text must be set",
		},
		{
			name: "short help is too long",
			command: func(c *Command) {
				c.ShortHelp = "This is a very long help message that is too long to be valid."
			},
			error: "short help text is too long. Max length is 60; got",
		},
		{
			name: "short help doesn't start with capital",
			command: func(c *Command) {
				c.ShortHelp = "bad short."
			},
			error: "short help text must start with a capital letter, end with a period, and contain only letters, apostrophes, hyphens, and spaces",
		},
		{
			name: "short help doesn't end with a period",
			command: func(c *Command) {
				c.ShortHelp = "Bad short"
			},
			error: "short help text must start with a capital letter, end with a period, and contain only letters, apostrophes, hyphens, and spaces",
		},
		{
			name: "short help has bad char",
			command: func(c *Command) {
				c.ShortHelp = "Bad $hort."
			},
			error: "short help text must start with a capital letter, end with a period, and contain only letters, apostrophes, hyphens, and spaces",
		},
		{
			name: "additional docs has title",
			command: func(c *Command) {
				c.children[0].AdditionalDocs[0].Title = ""
			},
			error: "error validating documentation section 0: title cannot be empty",
		},
		{
			name: "additional docs has no period",
			command: func(c *Command) {
				c.children[0].AdditionalDocs[0].Title = "test."
			},
			error: "error validating documentation section 0: title cannot end with a period",
		},
		{
			name: "additional docs has no docs",
			command: func(c *Command) {
				c.children[0].AdditionalDocs[0].Documentation = ""
			},
			error: "error validating documentation section 0: documentation cannot be empty",
		},
		{
			name: "example preamble set",
			command: func(c *Command) {
				c.children[0].Examples[0].Preamble = ""
			},
			error: "error validating example 0: preamble cannot be empty",
		},
		{
			name: "example preamble start with capital",
			command: func(c *Command) {
				c.children[0].Examples[0].Preamble = "bad preamble:"
			},
			error: "error validating example 0: preamble must start with a capital letter and end with a colon",
		},
		{
			name: "example preamble end with colon",
			command: func(c *Command) {
				c.children[0].Examples[0].Preamble = "Bad preamble"
			},
			error: "error validating example 0: preamble must start with a capital letter and end with a colon",
		},
		{
			name: "examples start with a $",
			command: func(c *Command) {
				c.children[0].Examples[0].Command = "tfcloud parent child --count 5"
			},
			error: "error validating example 0: example command must start with $ or #",
		},
		{
			name: "flag name is set",
			command: func(c *Command) {
				c.Flags.Persistent[0].Name = ""
			},
			error: "error validating persistent flag \"\": name cannot be empty",
		},
		{
			name: "flag name must be lower",
			command: func(c *Command) {
				c.Flags.Persistent[0].Name = "BAD"
			},
			error: "error validating persistent flag \"BAD\": only lower case letters, numbers, and hyphens are allowed",
		},
		{
			name: "flag name can't end in hyphen",
			command: func(c *Command) {
				c.Flags.Persistent[0].Name = "test-"
			},
			error: "error validating persistent flag \"test-\": only lower case letters, numbers, and hyphens are allowed",
		},
		{
			name: "flag name no underscores",
			command: func(c *Command) {
				c.Flags.Persistent[0].Name = "test_flag"
			},
			error: "error validating persistent flag \"test_flag\": only lower case letters, numbers, and hyphens are allowed",
		},
		{
			name: "flag name no special",
			command: func(c *Command) {
				c.Flags.Persistent[0].Name = "test!"
			},
			error: "error validating persistent flag \"test!\": only lower case letters, numbers, and hyphens are allowed",
		},
		{
			name: "flag shorthand must be lower",
			command: func(c *Command) {
				c.Flags.Persistent[0].Shorthand = "B"
			},
			error: "error validating persistent flag \"project\": shorthand \"B\" is not lowercase",
		},
		{
			name: "flag shorthand too long",
			command: func(c *Command) {
				c.Flags.Persistent[0].Shorthand = "bbb"
			},
			error: "error validating persistent flag \"project\": shorthand \"bbb\" must be a single character",
		},
		{
			name: "flag display value must be upper case",
			command: func(c *Command) {
				c.Flags.Persistent[0].DisplayValue = "id"
			},
			error: "error validating persistent flag \"project\": display value \"id\" is not uppercase",
		},
		{
			name: "flag description lowercase start",
			command: func(c *Command) {
				c.Flags.Persistent[0].Description = "this is a description."
			},
			error: "error validating persistent flag \"project\": description must start with a capital letter and end with a period",
		},
		{
			name: "flag description end with period",
			command: func(c *Command) {
				c.Flags.Persistent[0].Description = "This is a description"
			},
			error: "error validating persistent flag \"project\": description must start with a capital letter and end with a period",
		},
		{
			name: "flag description no value",
			command: func(c *Command) {
				c.Flags.Persistent[0].Value = nil
			},
			error: "error validating persistent flag \"project\": value cannot be nil",
		},
		{
			name: "flags don't override parent persistent",
			command: func(c *Command) {
				c.children[0].Flags.Local[0].Name = "project"
			},
			error: "local flag \"project\" overrides inherited persistent flag",
		},
		{
			name: "PositionalArgs preamble is valid",
			command: func(c *Command) {
				c.children[0].Args.Preamble = "bad preamble."
			},
			error: "error validating positional arguments: preable must start with a capital letter and end with a period",
		},
		{
			name: "PositionalArg name is set",
			command: func(c *Command) {
				c.children[0].Args.Args[0].Name = ""
			},
			error: "error validating positional argument 0: name cannot be empty",
		},
		{
			name: "PositionalArg name must be uppercase",
			command: func(c *Command) {
				c.children[0].Args.Args[0].Name = "bad"
			},
			error: "error validating positional argument 0: name \"bad\" is not uppercase",
		},
		{
			name: "PositionalArg documentation must be set",
			command: func(c *Command) {
				c.children[0].Args.Args[0].Documentation = ""
			},
			error: "error validating positional argument 0: documentation cannot be empty",
		},
		{
			name: "PositionalArg documentation must end with a period",
			command: func(c *Command) {
				c.children[0].Args.Args[0].Documentation = "bad docs"
			},
			error: "error validating positional argument 0: documentation must end with a period",
		},
		{
			name: "PositionalArg optional must be last",
			command: func(c *Command) {
				c.children[0].Args.Args[0].Optional = true
			},
			error: "error validating positional argument 0: optional positional argument \"TEXT\" must be the last argument",
		},
		{
			name: "PositionalArg repeated must be last",
			command: func(c *Command) {
				c.children[0].Args.Args[0].Repeatable = true
			},
			error: "error validating positional argument 0: repeatable positional argument \"TEXT\" must be the last argument",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			// Get and modify the good command
			cmd := getGoodCommand()
			c.command(cmd)

			err := cmd.Validate()
			if c.error != "" {
				r.ErrorContains(err, c.error)
			} else {
				r.NoError(err)
			}
		})
	}
}

func TestCommand_Validate_MultiError(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Get and modify the good command
	cmd := getGoodCommand()
	cmd.Name = "ThisIsBad"
	cmd.Aliases = append(cmd.Aliases, "ThisIsBad")
	cmd.Flags.Persistent[0].Name = "test_flag"

	err := cmd.Validate()
	r.ErrorContains(err, "only lower case names with hyphens are allowed")
	r.ErrorContains(err, "command name cannot be an alias")
	r.ErrorContains(err, "error validating persistent flag \"test_flag\": only lower case letters, numbers, and hyphens are allowed")
}
