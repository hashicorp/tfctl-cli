// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mitchellh/go-wordwrap"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
)

// NewCmdRunStatus creates the `run status` command.
func NewCmdRunStatus(ctx *cmd.Context) *cmd.Command {
	var organization string

	cmd := &cmd.Command{
		Name:      "status",
		ShortHelp: "Show the status of a run, printing diagnostics if it failed.",
		LongHelp: heredoc.New(ctx.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s run status" }} command inspects an HCP Terraform run and prints its current status. If the run has errored, it fetches the plan or apply log and extracts diagnostic messages.

		The ID argument can be:
		- A run ID ({{ template "mdCodeOrBold" "run-..." }})
		- A workspace ID ({{ template "mdCodeOrBold" "ws-..." }}) to get the latest run
		- A workspace name to get the latest run (may require {{ template "mdCodeOrBold" "--organization" }})
		`, config.Name),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "ID",
					Documentation: "Run ID, workspace ID, or workspace name",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:        "organization",
					Description: "Organization name (defaults to profile or terraform cloud config context)",
					Value:       flagvalue.Simple("", &organization),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Check status of a run by ID",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run status run-abc123`, config.Name),
			},
			{
				Preamble: "Check the latest run in a workspace by name",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s run status my-workspace --organization my-org`, config.Name),
			},
		},
		RunF: func(c *cmd.Command, args []string) error {
			if len(args) != 1 {
				return cmd.ErrDisplayUsage
			}

			org := organization
			if org == "" {
				org = ctx.Profile.Organization
			}
			if org == "" {
				cfg, err := terraformcfg.FindCloudConfig(".")
				if err == nil && cfg.Organization != "" {
					org = cfg.Organization
				}
			}

			apiClient, err := ctx.NewAPIClient(c.Logger())
			if err != nil {
				return fmt.Errorf("unable to create API client: %w", err)
			}

			resolver := client.NewResolver(apiClient, false, false)

			id := args[0]
			resourceType := "workspaces"
			switch {
			case strings.HasPrefix(id, "run-"):
				resourceType = "runs"
			case strings.HasPrefix(id, "ws-"):
				resourceType = "workspaces"
			default:
				if org == "" {
					return fmt.Errorf("--organization is required when specifying a workspace name")
				}
			}

			logger := c.Logger()
			logger.Debug("resolving run", "id", id, "type", resourceType, "organization", org)

			runID, err := resolver.RunOrCurrentRun(ctx.ShutdownCtx, org, resourceType, id)
			if err != nil {
				return err
			}

			logger.Debug("fetching run summary", "run_id", runID)

			summary, err := client.NewRunSummary(ctx.ShutdownCtx, apiClient.TFE.API, runID)
			if err != nil {
				return err
			}

			if err := ctx.Output.Display(&summaryDisplayer{summary: summary, io: ctx.IO}); err != nil {
				return err
			}

			if summary.Status == "errored" {
				return cmd.ErrUnderlyingError
			}
			return nil
		},
	}

	return cmd
}

// summaryDisplayer implements format.Displayer and format.StringPayload.
type summaryDisplayer struct {
	summary *client.RunSummary
	io      iostreams.IOStreams
}

var (
	_ format.Displayer     = (*summaryDisplayer)(nil)
	_ format.StringPayload = (*summaryDisplayer)(nil)
)

func (d *summaryDisplayer) DefaultFormat() format.Format { return format.Pretty }
func (d *summaryDisplayer) Payload() any                 { return d.summary }
func (d *summaryDisplayer) FieldTemplates() []format.Field {
	return nil
}

// StringPayload returns pre-formatted output tailored to the given format.
func (d *summaryDisplayer) StringPayload(f format.Format) string {
	if len(d.summary.Diagnostics) > 0 {
		return d.formatDiagnostics(f)
	}
	if d.summary.RawLog != "" {
		if f == format.Markdown {
			return stripANSI(d.summary.RawLog)
		}
		return d.summary.RawLog
	}
	return d.summary.Message
}

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func (d *summaryDisplayer) formatDiagnostics(f format.Format) string {
	switch f {
	case format.Markdown:
		return d.formatDiagnosticsMarkdown()
	default:
		return d.formatDiagnosticsPretty()
	}
}

func (d *summaryDisplayer) formatDiagnosticsPretty() string {
	cs := d.io.ColorScheme()
	const leftRuleWidth = 2
	wrapWidth := d.io.TerminalWidth() - leftRuleWidth

	var out strings.Builder
	for i, diag := range d.summary.Diagnostics {
		if i > 0 {
			out.WriteString("\n")
		}

		color := cs.Red()
		label := "Error"
		if diag.Severity == "warning" {
			color = cs.Orange()
			label = "Warning"
		}

		var body strings.Builder
		body.WriteString(cs.String(fmt.Sprintf("%s: ", label)).Color(color).Bold().String())
		body.WriteString(cs.String(diag.Summary).Bold().String())
		body.WriteString("\n")

		if diag.Range != nil {
			body.WriteString("\n")
			loc := fmt.Sprintf("  on %s line %d", diag.Range.Filename, diag.Range.Start.Line)
			if diag.Snippet != nil && diag.Snippet.Context != nil {
				loc += fmt.Sprintf(", in %s", *diag.Snippet.Context)
			}
			loc += ":"
			body.WriteString(loc)
			body.WriteString("\n")
		}

		if diag.Snippet != nil {
			body.WriteString(formatSnippet(cs, diag.Snippet))
		}

		if diag.Detail != "" {
			body.WriteString("\n")
			for _, line := range strings.Split(diag.Detail, "\n") {
				if wrapWidth > 0 && line != "" && line[0] != ' ' {
					line = wordwrap.WrapString(line, uint(wrapWidth))
				}
				body.WriteString(line)
				body.WriteString("\n")
			}
		}

		rule := cs.String("│").Color(color).String()
		out.WriteString(cs.String("╷").Color(color).String())
		out.WriteString("\n")
		for _, line := range strings.Split(strings.TrimRight(body.String(), "\n"), "\n") {
			out.WriteString(rule)
			if line != "" {
				out.WriteString(" ")
				out.WriteString(line)
			}
			out.WriteString("\n")
		}
		out.WriteString(cs.String("╵").Color(color).String())
	}
	return out.String()
}

func (d *summaryDisplayer) formatDiagnosticsMarkdown() string {
	var out strings.Builder
	for i, diag := range d.summary.Diagnostics {
		if i > 0 {
			out.WriteString("\n\n---\n\n")
		}
		label := "Error"
		if diag.Severity == "warning" {
			label = "Warning"
		}
		fmt.Fprintf(&out, "**%s: %s**\n", label, diag.Summary)
		if diag.Range != nil {
			loc := fmt.Sprintf("on %s line %d", diag.Range.Filename, diag.Range.Start.Line)
			if diag.Snippet != nil && diag.Snippet.Context != nil {
				loc += fmt.Sprintf(", in %s", *diag.Snippet.Context)
			}
			fmt.Fprintf(&out, "\n%s:\n", loc)
		}
		if diag.Snippet != nil {
			fmt.Fprintf(&out, "\n```hcl\n%s\n```\n", diag.Snippet.Code)
		}
		if diag.Detail != "" {
			fmt.Fprintf(&out, "\n%s\n", diag.Detail)
		}
	}
	return out.String()
}

// formatSnippet renders a code snippet with ANSI underline highlighting,
// matching Terraform's diagnostic output style.
func formatSnippet(cs *iostreams.ColorScheme, snippet *client.DiagnosticSnippet) string {
	var out strings.Builder

	code := snippet.Code
	start := clamp(snippet.HighlightStartOffset, 0, len(code))
	end := clamp(snippet.HighlightEndOffset, start, len(code))

	// Apply underline to the highlighted range.
	var rendered string
	if end > start {
		before := code[:start]
		highlight := code[start:end]
		after := code[end:]
		rendered = before + cs.String(highlight).Underline().String() + after
	} else {
		rendered = code
	}

	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		fmt.Fprintf(&out, "  %4d: %s\n", snippet.StartLine+i, line)
	}

	return out.String()
}

func clamp(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}
