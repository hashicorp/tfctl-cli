package harness

import (
	"bufio"
	"bytes"
	"fmt"

	"charm.land/glamour/v2"
	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/skills"
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
		ShortHelp: "Print coding agent context for tfctl.",
		LongHelp: heredoc.New(ctx.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s harness context" }} command prints coding agent context for tfctl. This context is used to provide information to coding agents which makes them more effective at using {{ Bold "tfctl" }}.
		`, config.Name),
		RunF: func(c *cmd.Command, args []string) error {
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

func (d *markdownDisplayer) StringPayload(f format.Format) string {
	if !d.IO.IsOutputTTY() {
		return d.Markdown
	}

	out, err := glamour.Render(d.Markdown, "dark")
	if err != nil {
		return d.Markdown
	}

	return out
}
