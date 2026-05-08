// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package heredoc

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestTemplate_NoWrap(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	input := `
	This is a line in
	a longer paragraph.

	Versus this is a new paragraph since it is separated by a blank line.
	`
	expected := "This is a line in a longer paragraph.\n\nVersus this is a new paragraph since it is separated by a blank line."

	io := iostreams.Test()
	f := New(io, WithNoWrap())

	out, err := f.Doc(input)
	r.NoError(err)
	r.Equal(expected, out)
}

func TestTemplate_AutoParagraph(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	input := `
	This is a line in
	a longer paragraph.

	Versus this is a new paragraph since it is separated by a blank line.
	`
	expectedAuto := "This is a line in a longer paragraph.\n\nVersus this is a new paragraph since it is separated by a blank line."
	expectedPreserve := "This is a line in\na longer paragraph.\n\nVersus this is a new paragraph since it is separated by a blank line."

	io := iostreams.Test()
	f := New(io)

	out, err := f.Doc(input)
	r.NoError(err)
	r.Equal(expectedAuto, out)

	f = New(io, WithPreserveNewlines())
	out, err = f.Doc(input)
	r.NoError(err)
	r.Equal(expectedPreserve, out)
}

func TestTemplate_PreserveNewLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name: "Longer example",
			Input: `
	Input before preserve new lines.

	{{ PreserveNewLines }}
	This is a line in
	a longer, preserved paragraph.
	{{ PreserveNewLines }}

	Versus this is a new paragraph since it is separated by a blank line. But since it is long, it gets split.
	`,
			Expected: "Input before preserve new lines.\n\nThis is a line in\na longer, preserved paragraph.\n\nVersus this is a new paragraph since it is separated by a blank line. But since\nit is long, it gets split.",
		},
		{
			Name: "Bullets",
			Input: `
The name of the group to delete. The name may be specified as either:

{{ PreserveNewLines }}
  * The group's resource name. Formatted as:
    {{ Italic "iam/organization/ORG_ID/group/GROUP_NAME" }}
  * The resource name suffix, GROUP_NAME.
{{ PreserveNewLines }}`,
			Expected: `The name of the group to delete. The name may be specified as either:

  * The group's resource name. Formatted as:
    iam/organization/ORG_ID/group/GROUP_NAME
  * The resource name suffix, GROUP_NAME.`,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)
			io := iostreams.Test()
			f := New(io)

			out, err := f.Doc(c.Input)
			r.NoError(err)
			r.Equal(c.Expected, out)
		})
	}
}

func TestTemplate_Format(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	input := `lets %s and a number %d`
	expected := `lets format a string and a number 42`

	io := iostreams.Test()
	f := New(io)
	out, err := f.Docf(input, "format a string", 42)
	r.NoError(err)
	r.Equal(expected, out)
}

func TestTemplate_StripWhitespace(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	input := `
start after first line

`
	expected := `start after first line`

	io := iostreams.Test()
	f := New(io)
	out, err := f.Doc(input)
	r.NoError(err)
	r.Equal(expected, out)
}

func TestTemplate_Wrapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name     string
		Input    string
		Expected string
		Width    int
	}{
		{
			Name:  "Wrap long",
			Width: 11,
			Input: `this is too long by a bit.`,
			Expected: `this is too
long by a
bit.`,
		},
		{
			Name:  "Wrap at the word that would push the line over rather than cutting it.",
			Width: 15,
			Input: `this is too long by a bit.`,
			Expected: `this is too
long by a bit.`,
		},
		{
			Name:  "Realistic Example",
			Width: 80,
			Input: `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.

Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.

Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.`,
			Expected: `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor
incididunt ut labore et dolore magna aliqua.

Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut
aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in
voluptate velit esse cillum dolore eu fugiat nulla pariatur.

Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia
deserunt mollit anim id est laborum.`,
		},

		{
			// TODO
			Name:  "Wrap a long single line",
			Width: 80,
			Input: `Really long lines will automatically get wrapped for you. So you don't need to worry about being super rigorous about where you wrap a line.`,
			Expected: `Really long lines will automatically get wrapped for you. So you don't need to
worry about being super rigorous about where you wrap a line.`,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			io := iostreams.Test()
			f := New(io, WithWidth(c.Width))
			out, err := f.Doc(c.Input)
			r.NoError(err)
			r.Equal(c.Expected, out)
		})
	}
}

func TestTemplate_Dedent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Name     string
		Input    string
		Expected string
	}{
		{
			Name: "Dedent all",
			Input: `
			Has starting spaces.
			Has starting spaces.
			Has starting spaces.
			`,
			Expected: `Has starting spaces.
Has starting spaces.
Has starting spaces.`,
		},
		{
			Name: "Mixed indent",
			Input: `
			Has starting spaces.
				Further Indent.
			Has starting spaces.
				Further Indent.
				  More Further Indent.
			`,
			Expected: `Has starting spaces.
  Further Indent.
Has starting spaces.
  Further Indent.
    More Further Indent.`,
		},
		{
			Name: "Shared prefix has mixed tabs and spaces",
			Input: `
			Has starting tabs.
` + strings.Repeat(" ", 6) + `Has starting spaces.
			`,
			Expected: `Has starting tabs.
Has starting spaces.`,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			io := iostreams.Test()
			f := New(io, WithPreserveNewlines())
			out, err := f.Doc(c.Input)
			r.NoError(err)
			r.Equal(c.Expected, out)
		})
	}
}

func TestTemplate_Colors(t *testing.T) {
	t.Parallel()
	tmpl := `{{ Color "red" "test" }}
{{ Color "ReD" "test" }}
{{ Color "white" "orange"  "White on orange" }}
{{ Color "#ff00aa" "rgb color" }}
{{ Bold "Bold" }}
{{ Faint "Faint" }}
{{ Italic "Italic" }}
{{ Underline "Underline" }}
{{ Blink "Blink" }}
{{ CrossOut "CrossOut" }}
{{ Color "ReD" (Bold (Italic "chained")) }}`

	// Test acts more as a detector for breaking changes. Important part is that
	// Ascii has no escape sequences, and between Ansi, 256, TrueColor, they are
	// different and contain escape sequences.
	cases := []struct {
		Name     string
		Profile  termenv.Profile
		Expected string
	}{
		{
			Name:     "No color",
			Profile:  termenv.Ascii,
			Expected: "test\ntest\nWhite on orange\nrgb color\nBold\nFaint\nItalic\nUnderline\nBlink\nCrossOut\nchained",
		},
		{
			Name:     "ANSI",
			Profile:  termenv.ANSI,
			Expected: "\x1b[91mtest\x1b[0m\n\x1b[91mtest\x1b[0m\n\x1b[37;101mWhite on orange\x1b[0m\n\x1b[95mrgb color\x1b[0m\n\x1b[1mBold\x1b[0m\n\x1b[2mFaint\x1b[0m\n\x1b[3mItalic\x1b[0m\n\x1b[4mUnderline\x1b[0m\n\x1b[5mBlink\x1b[0m\n\x1b[9mCrossOut\x1b[0m\n\x1b[91m\x1b[1m\x1b[3mchained\x1b[0m\x1b[0m\x1b[0m",
		},
		{
			Name:     "256",
			Profile:  termenv.ANSI256,
			Expected: "\x1b[38;5;160mtest\x1b[0m\n\x1b[38;5;160mtest\x1b[0m\n\x1b[37;48;5;130mWhite on orange\x1b[0m\n\x1b[38;5;199mrgb color\x1b[0m\n\x1b[1mBold\x1b[0m\n\x1b[2mFaint\x1b[0m\n\x1b[3mItalic\x1b[0m\n\x1b[4mUnderline\x1b[0m\n\x1b[5mBlink\x1b[0m\n\x1b[9mCrossOut\x1b[0m\n\x1b[38;5;160m\x1b[1m\x1b[3mchained\x1b[0m\x1b[0m\x1b[0m",
		},
		{
			Name:     "TrueColor",
			Profile:  termenv.TrueColor,
			Expected: "\x1b[38;2;229;34;40mtest\x1b[0m\n\x1b[38;2;229;34;40mtest\x1b[0m\n\x1b[37;48;2;187;89;0mWhite on orange\x1b[0m\n\x1b[38;2;255;0;170mrgb color\x1b[0m\n\x1b[1mBold\x1b[0m\n\x1b[2mFaint\x1b[0m\n\x1b[3mItalic\x1b[0m\n\x1b[4mUnderline\x1b[0m\n\x1b[5mBlink\x1b[0m\n\x1b[9mCrossOut\x1b[0m\n\x1b[38;2;229;34;40m\x1b[1m\x1b[3mchained\x1b[0m\x1b[0m\x1b[0m",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			io := iostreams.Test()
			io.ForcedColorProfile(c.Profile)

			f := New(io, WithPreserveNewlines())
			out, err := f.Doc(tmpl)
			r.NoError(err)
			r.Equal(c.Expected, out)
		})
	}
}
