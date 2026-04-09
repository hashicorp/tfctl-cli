// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iostreams

import (
	"fmt"

	"github.com/muesli/termenv"
)

// IsMarkdownOutput is an interface that when met, indicates that the IOStreams
// instance is configured to output markdown.
type IsMarkdownOutput interface {
	IOStreams

	// SetMD sets the IOStreams instance to output markdown or not. This can be
	// useful to temporarily change the output format.
	SetMD(bool)
}

type mdStream struct {
	*Testing

	// enabled is true if the IOStreams instance is configured to output markdown.
	enabled bool
}

// MD returns a new IOStreams instance that is configured to output markdown.
func MD() IOStreams {
	return &mdStream{
		Testing: Test(),
		enabled: true,
	}
}

// SetMD sets the IOStreams instance to output markdown or not.
func (m *mdStream) SetMD(enable bool) {
	m.enabled = enable
}

func (m *mdStream) ColorScheme() *ColorScheme {
	return &ColorScheme{
		profile: termenv.Ascii,
		md:      m.enabled,
	}
}

// markdownString returns a string that is formatted for markdown.
func (s String) markdownString() string {
	mdText := s.rawString

	for _, e := range s.emphases {
		switch e {
		case EmphasisBold:
			mdText = "**" + mdText + "**"
		case EmphasisItalic:
			mdText = "_" + mdText + "_"
		case EmphasisUnderline:
			mdText = "<u>" + mdText + "</u>"
		case EmphasisCrossOut:
			mdText = "~~" + mdText + "~~"
		case EmphasisCode:
			mdText = "`" + mdText + "`"
		case EmphasisCodeBlock:
			mdText = fmt.Sprintf("```%s\n%s\n```", s.codeBlockExtension, mdText)
		}
	}

	return mdText
}
