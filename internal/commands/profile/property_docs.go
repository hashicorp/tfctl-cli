// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/muesli/reflow/indent"
	"golang.org/x/exp/maps"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

// availableProperties returns a document section describing all the available
// properties to be set on the profile.
func availablePropertiesDoc(io iostreams.IOStreams) cmd.DocSection {
	return cmd.DocSection{
		Title:         "Available Properties",
		Documentation: availableProperties(io).build(),
	}
}

func availableProperties(io iostreams.IOStreams) *availablePropertiesBuilder {
	b := newAvailablePropertiesBuilder(io)
	addCoreProperties(b)
	return b
}

func addCoreProperties(b *availablePropertiesBuilder) {
	b.AddProperty("", "default_organization", "Organization of the HCP Terraform or Terraform Enterprise organization to operate on by default.")
	b.AddProperty("", "hostname", `
		Default hostname API endpoints, if different from HCP Terraform (app.terraform.io). This affects which regional
		endpoints are used for HCP services. For eu regions, use app.eu.terraform.io.`)
	b.AddProperty("", "token", "The API token to use for all requests.")
	b.AddProperty("", "no_color", "If True, color will not be used when printing messages in the terminal.")
	b.AddProperty("", "telemetry", `
		Controls telemetry behavior. Set to {{ template "mdCodeOrBold" "false" }} or
		{{ template "mdCodeOrBold" "disabled" }} to disable telemetry, {{ template "mdCodeOrBold" "log" }}
		to output span data to stderr, or any other value to enable OTLP export.`)
}

type availablePropertiesBuilder struct {
	io         iostreams.IOStreams
	properties map[string]map[string]string
}

func newAvailablePropertiesBuilder(io iostreams.IOStreams) *availablePropertiesBuilder {
	return &availablePropertiesBuilder{
		io:         io,
		properties: make(map[string]map[string]string),
	}
}

func (b availablePropertiesBuilder) AddProperty(component, property, description string, args ...any) {
	c, ok := b.properties[component]
	if !ok {
		b.properties[component] = make(map[string]string)
		c = b.properties[component]
	}

	c[property] = heredoc.New(b.io).Mustf(description, args...)
}

func (b availablePropertiesBuilder) build() string {
	if _, ok := b.io.(iostreams.IsMarkdownOutput); ok {
		return b.buildMD()
	}

	return b.buildCLI()
}

func (b availablePropertiesBuilder) buildCLI() string {
	var buf bytes.Buffer
	cs := b.io.ColorScheme()

	// Start with the core section first
	topLevel, ok := b.properties[""]
	if ok {
		keys := maps.Keys(topLevel)
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintln(&buf, k)
			fmt.Fprintln(&buf, indent.String(topLevel[k], 2))
			fmt.Fprintln(&buf)
		}
	}

	allComponents := maps.Keys(b.properties)
	slices.Sort(allComponents)
	for _, c := range allComponents {
		if c == "" {
			continue
		}

		// Print the component
		fmt.Fprintln(&buf, cs.String(c).Underline().String())

		keys := maps.Keys(b.properties[c])
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintln(&buf, indent.String(k, 2))
			fmt.Fprintln(&buf, indent.String(b.properties[c][k], 4))
			fmt.Fprintln(&buf)
		}
	}

	return buf.String()
}

func (b availablePropertiesBuilder) buildMD() string {
	var buf bytes.Buffer
	cs := b.io.ColorScheme()

	// Start with the core section first
	topLevel, ok := b.properties[""]
	if ok {
		keys := maps.Keys(topLevel)
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintf(&buf, "* `%s`\n", k)
			fmt.Fprintln(&buf, indent.String(fmt.Sprintf("* %s", topLevel[k]), 4))
			fmt.Fprintln(&buf)
		}
	}

	allComponents := maps.Keys(b.properties)
	slices.Sort(allComponents)
	for _, c := range allComponents {
		if c == "" {
			continue
		}

		// Print the component
		fmt.Fprintf(&buf, "* `%s`\n\n", c)

		keys := maps.Keys(b.properties[c])
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintln(&buf, indent.String(fmt.Sprintf("* `%s` - %s", cs.String(k), b.properties[c][k]), 4))
			fmt.Fprintln(&buf)
		}
	}

	return buf.String()
}
