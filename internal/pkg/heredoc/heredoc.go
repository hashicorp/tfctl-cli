// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package heredoc provides utilities for formatting text that is intended to be
// output to users and includes text/template formatting and word-wrapping.
package heredoc

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/lithammer/dedent"
	"github.com/muesli/reflow/wordwrap"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
)

// Config stores configuration for the formatter. The values can be set by
// constructing the formatter with ConfigOptions.
type Config struct {
	width         int
	autoParagraph bool
}

// defaultConfig returns the default configuration.
func defaultConfig() *Config {
	return &Config{
		width:         80,
		autoParagraph: true,
	}
}

// ConfigOption allow configuring the formatters configuration.
type ConfigOption func(c *Config)

// WithWidth sets the maximum width at which lines are word-wrapped.
func WithWidth(w int) ConfigOption {
	return func(c *Config) {
		c.width = w
	}
}

// WithNoWrap does not wrap the line. This is useful when the output will be
// passed to other functions that have their own wrapping.
func WithNoWrap() ConfigOption {
	return func(c *Config) {
		c.width = -1
	}
}

// WithPreserveNewlines is used to disable the auto-formation of paragraphs for
// the entire template. To preserve new lines over just a section of the
// template, use the {{ PreserveNewLines }} template function.
func WithPreserveNewlines() ConfigOption {
	return func(c *Config) {
		c.autoParagraph = false
	}
}

// Formatter is used to format a template such that it can be outputted to
// users. The formatter wraps lines and ignores any initial indentation.
// Further, it supplements the text/template string with functions for
// colorizing and styling output.
type Formatter struct {
	c  *Config
	io iostreams.IOStreams
}

// New returns a new formatter given the IOStreams and passed options.
func New(io iostreams.IOStreams, options ...ConfigOption) *Formatter {
	f := &Formatter{
		io: io,
		c:  defaultConfig(),
	}

	for _, o := range options {
		o(f.c)
	}

	return f
}

// Must invokes Doc and panics if an error occurs.
func (f *Formatter) Must(tmpl string) string {
	doc, err := f.Doc(tmpl)
	if err != nil {
		panic(err)
	}

	return doc
}

// Mustf invokes Docf and panics if an error occurs.
func (f *Formatter) Mustf(tmpl string, args ...any) string {
	doc, err := f.Docf(tmpl, args...)
	if err != nil {
		panic(err)
	}

	return doc
}

// Docf takes a text/template string and a series of arguments that are
// fmt.Sprintf into the template before the interpolatted string is passed to
// Doc function. To see the format of the tmpl, see Doc's documentation.
func (f *Formatter) Docf(tmpl string, args ...any) (string, error) {
	interpolatted := fmt.Sprintf(tmpl, args...)
	return f.Doc(interpolatted)
}

// Doc takes a text/template string and renders it. The formatter adds the
// following functions that are available to the template string.
//
//   - PreserveNewLines: Must be paired. Any text between the two calls will have
//     new lines preserved.
//   - Color <text-color> "string"
//   - Color <text-color> <background-color> "string"
//   - Bold "string"
//   - Italic "string"
//   - Faint "string"
//   - Underline "string"
//   - Blink "string"
//   - CrossOut "string"
//   - Link "name" "url" {{ Link "
//   - Code "string"
//   - CodeBlock "string" "extension" (shell-session, json, go)
//   - IsMD: Returns true if the output is markdown.
//
// Valid Color values are: "red", "green", "yellow", "orange", "gray", white",
// "black" (case insensitive), or "#<hex>".
//
// These functions can be chained such as:
// {{ Color "Red" ( Italic (CrossOut "example" ) ) }}
//
// Additionally, the following templates are made available:
//
//   - mdCodeOrBold: If the output is markdown, it will return the string in a
//     code stanza. Otherwise, it will return the string in bold.
//     An example usage is:
//     {{ template "mdCodeOrBold" "tfcloud projects iam read-policy --format=json" }}
//
// After rendering the template following manipulations are made:
// - The text is dedented. This allows you to use a Go string literal and not
// worry about the indentation. e.g. ` your docs`. -> `your docs`
// - Long lines are wrapped at word boundaries. The default wrapping length can
// be overridden using WithWidth.
// - Starting and ending blank spaces are stripped.
func (f *Formatter) Doc(tmpl string) (string, error) {
	// Replace tabs with spaces
	tmpl = strings.ReplaceAll(tmpl, "\t", "  ")

	// Parse the string as template
	tpl := template.New("tpl")
	tpl.Funcs(f.templateFuncs(tpl))
	tpl, err := tpl.Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse input as a text/template: %w", err)
	}

	if err := addTemplates(tpl); err != nil {
		return "", fmt.Errorf("failed to add templates: %w", err)
	}

	// Run the template
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, nil); err != nil {
		return "", fmt.Errorf("failed executing text/template: %w", err)
	}

	dedented := dedent.Dedent(buf.String())

	// Form paragraphs automatically
	chunked := dedented
	if f.c.autoParagraph {
		// Look for any preserve line sentinels
		preserveGroups := strings.Split(dedented, preserveNewLinesToken)
		if len(preserveGroups)%2 != 1 {
			return "", fmt.Errorf("{{ PreserveNewLines }} calls are not balanced")
		}

		// [Normal, Preserve, Normal, Preserve, ...]
		text := ""
		for i, group := range preserveGroups {
			if i%2 == 1 {
				// Should be preserved but strip the new lines that come from
				// the {{ PreserveNewLines }} call.
				group, _ = strings.CutPrefix(group, "\n")
				group, _ = strings.CutSuffix(group, "\n")
				text += group
				continue
			}

			// Drop new lines within the paragraph.
			paragraphs := strings.Split(group, "\n\n")
			for i, p := range paragraphs {
				paragraphs[i] = strings.ReplaceAll(p, "\n", " ")
			}

			text += strings.Join(paragraphs, "\n\n")
		}

		chunked = text
	}

	// Word wrap
	wrapped := chunked
	if f.c.width > 0 {
		wrapped = wordWrap(chunked, f.c.width)
	}

	// Strip any whitespace at the start/end.
	stripped := strings.TrimSpace(wrapped)

	return stripped, nil
}

// wordWrap wraps an input at the given wrap length. It uses a customized
// wordwrap.Writer that is more appropriate for splitting command line flags.
func wordWrap(input string, wrap int) string {
	w := wordwrap.NewWriter(wrap)
	w.Breakpoints = []rune{}
	_, _ = w.Write([]byte(input))
	_ = w.Close()
	return w.String()
}
