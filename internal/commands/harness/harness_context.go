// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package harness

import (
	"bufio"
	"bytes"
	"fmt"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/skills"
	"github.com/hashicorp/tfctl-cli/version"
)

// ContextOpts defines the options for the `harness context` command.
type ContextOpts struct {
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Output  *format.Outputter
}

// NewCmdHarnessContext creates the `harness context` command.
func NewCmdHarnessContext(ctx *cmd.Context) *cmd.Command {
	contextOpts := ContextOpts{
		IO:      ctx.IO,
		Profile: ctx.Profile,
		Output:  ctx.Output,
	}

	cmd := &cmd.Command{
		Name:      "context",
		ShortHelp: "Print coding agent context for tfctl, suitable for AGENTS.md.",
		LongHelp: heredoc.New(ctx.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s harness context" }} command prints coding agent context for tfctl. This context can be used as part of AGENTS.md or other documentation which makes coding agents more effective at using {{ Bold "tfctl" }}.
		`, version.Name),
		RunF: func(_ *cmd.Command, _ []string) error {
			return runContext(&contextOpts)
		},
	}

	return cmd
}

func runContext(opts *ContextOpts) error {
	skill, err := skills.FS.Open("tfctl/SKILL.md")
	if err != nil {
		return fmt.Errorf("failed to open embedded SKILL.md file: %w", err)
	}

	var b bytes.Buffer
	scanner := bufio.NewScanner(skill)
	scanner.Split(bufio.ScanLines)

	scanner.Scan()
	if scanner.Text() != "---" {
		return fmt.Errorf("failed to find frontmatter in embedded SKILL.md file")
	}

	afterFrontMatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			afterFrontMatter = true
			continue
		}

		if afterFrontMatter {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	disp := markdownDisplayer{
		Markdown: b.String(),
		IO:       opts.IO,
	}

	return opts.Output.Display(&disp)
}

type markdownDisplayer struct {
	Markdown string
	IO       iostreams.IOStreams
}

func (d *markdownDisplayer) DefaultFormat() format.Format {
	return format.Markdown
}

func (d *markdownDisplayer) FieldTemplates() []format.Field {
	return nil
}

func (d *markdownDisplayer) Payload() any {
	return map[string]string{
		"content": d.Markdown,
	}
}

func (d *markdownDisplayer) StringPayload(_ format.Format) string {
	if !d.IO.IsOutputTTY() {
		return d.Markdown
	}

	// TODO: Decorate markdown for terminal output.
	return d.Markdown
}
