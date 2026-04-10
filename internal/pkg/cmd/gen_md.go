// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
)

const markdownExtension = ".mdx"

// LinkHandler is a function that can be used to modify the links in the
// generated markdown. The path string is the unmodified path to the file.
type LinkHandler func(path string) string

// GenMarkdownTree creates a markdown file for the command and all of its children.
func GenMarkdownTree(c *Command, dir string, link LinkHandler) error {
	// Create the directory if it doesn't exist
	if err := os.MkdirAll(dir, 0766); err != nil {
		return err
	}

	// Determine the filename. If the command is a command group parent and has
	// no run function, we create an index file, otherwise we name the file
	// after the command name.
	filename := "index" + markdownExtension
	if c.RunF != nil {
		filename = c.Name + markdownExtension
	}

	// Create the file
	f, err := os.Create(filepath.Join(dir, filename))
	if err != nil {
		return err
	}
	defer f.Close()

	// Generate the markdown
	if err := GenMarkdown(c, f, link); err != nil {
		return err
	}

	for _, c := range c.children {
		dir := dir
		if len(c.children) > 0 {
			dir = filepath.Join(dir, c.Name)
		}

		if err := GenMarkdownTree(c, dir, link); err != nil {
			return err
		}
	}

	return nil
}

// GenMarkdown creates custom markdown output.
func GenMarkdown(c *Command, w io.Writer, link LinkHandler) error {
	mdIO, ok := c.getRawIO().(iostreams.IsMarkdownOutput)
	if !ok {
		return fmt.Errorf("IOStream instance must be configured for markdown output")
	}

	buf := new(bytes.Buffer)
	name := c.commandPath()

	buf.WriteString("---\n")
	fmt.Fprintf(buf, "page_title: %s\n", name)
	fmt.Fprintf(buf, "description: |-\n  The \"%s\" command lets you %s\n", name, (strings.ToLower(c.ShortHelp[:1]) + c.ShortHelp[1:]))
	buf.WriteString("---\n\n")

	_, err := buf.WriteTo(w)
	if err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	buf.WriteString("# " + name + "\n\n")
	fmt.Fprintf(buf, "Command: `%s` \n\n", name)

	// Description
	buf.WriteString(c.LongHelp + "\n\n")

	// Disable markdown escaping and then re-enable. This is needed because
	// the flags / args would otherwise generate markdown that will not render
	// because we are using a code block.
	mdIO.SetMD(false)
	buf.WriteString("## Usage\n\n")
	fmt.Fprintf(buf, "```shell-session\n$ %s\n```\n\n", c.useLine())
	mdIO.SetMD(true)

	// Aliases
	if len(c.Aliases) > 0 {
		buf.WriteString("## Aliases\n\n")
		for a, u := range c.aliasUsages() {
			fmt.Fprintf(buf, "- `%s`. For example: `%s`\n", a, u)
		}
		buf.WriteString("\n")
	}

	// Examples
	if len(c.Examples) > 0 {
		buf.WriteString("## Examples\n\n")

		for _, e := range c.Examples {
			fmt.Fprintf(buf, "%s\n\n", e.Preamble)
			fmt.Fprintf(buf, "```shell-session\n%s\n```\n\n", e.Command)
		}
	}

	// Children commands
	if len(c.children) > 0 {
		var commands, groups []string
		for _, c := range c.children {
			path := strings.ReplaceAll(c.commandPath(), " ", "/")
			entry := fmt.Sprintf("- [`%s`](%s) - %s", c.Name, link(path), c.ShortHelp)

			if c.RunF != nil {
				commands = append(commands, entry)
			} else {
				groups = append(groups, entry)
			}
		}
		if len(groups) > 0 {
			buf.WriteString("## Command groups\n\n")
			buf.WriteString(strings.Join(groups, "\n") + "\n\n")
		}

		if len(commands) > 0 {
			buf.WriteString("## Commands\n\n")
			buf.WriteString(strings.Join(commands, "\n") + "\n\n")
		}
	}

	// Positional arguments
	genMarkdownPositionalArgs(c, buf)

	// Print flags
	genMarkdownFlags(c, buf)

	// Additional docs
	for _, d := range c.AdditionalDocs {
		fmt.Fprintf(buf, "## %s\n", d.Title)
		fmt.Fprintf(buf, "%s\n", d.Documentation)
	}

	_, err = buf.WriteTo(w)
	return err
}

func genMarkdownPositionalArgs(c *Command, buf *bytes.Buffer) {
	if len(c.Args.Args) == 0 {
		return
	}

	cs := c.getIO().ColorScheme()
	buf.WriteString("## Positional arguments\n\n")
	p := c.Args
	if p.Preamble != "" {
		fmt.Fprintln(buf, p.Preamble)
	}

	for _, a := range p.Args {
		nameUpper := strings.ToUpper(a.Name)
		repeatable := ""
		if a.Repeatable {
			repeatable = fmt.Sprintf(" [%s ...]", nameUpper)
		}
		fmt.Fprintf(buf, "- `%s%s` - ", nameUpper, repeatable)

		if a.Optional {
			fmt.Fprintln(buf, cs.String("Optional argument\n").Italic().String())
		}
		fmt.Fprintln(buf, strings.ReplaceAll(a.Documentation, "\n", "\n\t"))
		fmt.Fprintln(buf)
	}
}

func genMarkdownFlags(c *Command, buf *bytes.Buffer) {
	// If we are the root command, just print global flags.
	if c.parent == nil && c.RunF == nil {
		buf.WriteString("## Global flags\n\n")
		genMarkdownFlagsetUsage(c.globalFlags(), buf)
	}

	// Print flags only if the command is runnable
	if c.RunF == nil {
		return
	}

	flagSets := []struct {
		flags *pflag.FlagSet
		name  string
	}{
		{
			flags: c.localFlags(),
			name:  "",
		},
		{
			flags: c.inheritedFlags(),
			name:  "Inherited ",
		},
	}

	for _, set := range flagSets {
		required, optional := splitRequiredFlags(set.flags)
		if required.HasFlags() {
			fmt.Fprintf(buf, "## Required %sflags\n\n", set.name)
			genMarkdownFlagsetUsage(required, buf)

			if optional.HasFlags() {
				fmt.Fprintf(buf, "## Optional %sflags\n\n", set.name)
				genMarkdownFlagsetUsage(optional, buf)
			}
		} else if optional.HasFlags() {
			fmt.Fprintf(buf, "## %sFlags\n\n", set.name)
			genMarkdownFlagsetUsage(optional, buf)
		}
	}
}

func genMarkdownFlagsetUsage(flags *pflag.FlagSet, buf *bytes.Buffer) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}

		longDisplay := flagString(flag)
		if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
			fmt.Fprintf(buf, "- `-%s, %s` - ", flag.Shorthand, longDisplay)
		} else {
			fmt.Fprintf(buf, "- `%s` - ", longDisplay)
		}

		// Add the usage
		fmt.Fprintf(buf, "%s\n\n", strings.ReplaceAll(flag.Usage, "\n", "\n\t"))
	})
}
