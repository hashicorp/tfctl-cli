// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
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

// StatusOpts stores the options parsed from flags for the run status command.
type StatusOpts struct {
	IO           iostreams.IOStreams
	ShutdownCtx  context.Context
	Output       *format.Outputter
	Client       *client.Client
	Organization string
	ID           string
}

// NewCmdRunStatus creates the `run status` command.
func NewCmdRunStatus(ctx *cmd.Context) *cmd.Command {
	opts := &StatusOpts{
		IO: ctx.IO,
	}
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

			apiClient, err := ctx.NewAPIClient(c.Logger(ctx))
			if err != nil {
				return fmt.Errorf("unable to create API client: %w", err)
			}

			opts.ShutdownCtx = ctx.ShutdownCtx
			opts.Output = ctx.Output
			opts.Client = apiClient
			opts.Organization = org
			opts.ID = args[0]

			return runStatus(opts)
		},
	}

	return cmd
}

func runStatus(opts *StatusOpts) error {
	resolver := client.NewResolver(opts.Client, false, false)

	id := opts.ID
	resourceType := "workspaces"
	switch {
	case strings.HasPrefix(id, "run-"):
		resourceType = "runs"
	case strings.HasPrefix(id, "ws-"):
		resourceType = "workspaces"
	default:
		if opts.Organization == "" {
			return fmt.Errorf("--organization is required when specifying a workspace name")
		}
	}

	runID, err := resolver.RunOrCurrentRun(opts.ShutdownCtx, opts.Organization, resourceType, id)
	if err != nil {
		return err
	}

	summary, err := client.NewRunSummary(opts.ShutdownCtx, opts.Client, runID)
	if err != nil {
		return err
	}

	if err := opts.Output.Display(&summaryDisplayer{summary: summary, io: opts.IO}); err != nil {
		return err
	}

	switch summary.Status {
	case "errored", "policy_soft_failed", "policy_override":
		return cmd.ErrUnderlyingError
	}
	return nil
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
	if d.summary.PolicyCheckLog != "" {
		if f == format.Markdown {
			return d.formatPolicyCheckLogMarkdown()
		}
		return d.formatPolicyCheckLogPretty()
	}
	if len(d.summary.PolicyEvaluations) > 0 {
		return d.formatPolicyEvaluations(f)
	}
	if len(d.summary.TaskResults) > 0 {
		return d.formatTaskResults(f)
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

// --- Policy check log (legacy Sentinel) ---

func policyScopeLabel(scope string) string {
	switch scope {
	case "organization":
		return "Organization Policy Check"
	case "workspace":
		return "Workspace Policy Check"
	default:
		if scope != "" {
			return fmt.Sprintf("Policy Check (%s)", scope)
		}
		return "Policy Check"
	}
}

// cleanPolicyCheckLog cleans up the Sentinel runner's policy set header line
// when the policy set name is empty (non-VCS policy sets produce
// "<empty policy set name>" as the name).
var policySetHeaderRe = regexp.MustCompile(`(?m)^=+ Results for policy set: <empty policy set name> =+\n`)

func cleanPolicyCheckLog(log string) string {
	return policySetHeaderRe.ReplaceAllString(log, "")
}

func (d *summaryDisplayer) formatPolicyCheckLogPretty() string {
	cs := d.io.ColorScheme()
	var out strings.Builder

	out.WriteString("------------------------------------------------------------------------\n")
	header := policyScopeLabel(d.summary.PolicyCheckScope)
	out.WriteString(cs.String(header + ":").Bold().String())
	out.WriteString("\n\n")
	out.WriteString(cleanPolicyCheckLog(d.summary.PolicyCheckLog))

	// Add status footer.
	switch d.summary.PolicyCheckStatus {
	case "hard_failed":
		out.WriteString("\n")
		out.WriteString(cs.String(header + " hard failed.").Color(cs.Red()).String())
		out.WriteString("\n")
	case "soft_failed":
		out.WriteString("\n")
		out.WriteString(cs.String(header + " soft failed.").Color(cs.Orange()).String())
		out.WriteString("\n")
	case "errored":
		out.WriteString("\n")
		out.WriteString(cs.String(header + " errored.").Color(cs.Red()).String())
		out.WriteString("\n")
	}

	return out.String()
}

func (d *summaryDisplayer) formatPolicyCheckLogMarkdown() string {
	var out strings.Builder

	header := policyScopeLabel(d.summary.PolicyCheckScope)
	fmt.Fprintf(&out, "## %s\n\n", header)
	fmt.Fprintf(&out, "```\n%s\n```\n", stripANSI(cleanPolicyCheckLog(d.summary.PolicyCheckLog)))

	switch d.summary.PolicyCheckStatus {
	case "hard_failed":
		fmt.Fprintf(&out, "\n**%s hard failed.**\n", header)
	case "soft_failed":
		fmt.Fprintf(&out, "\n**%s soft failed.**\n", header)
	case "errored":
		fmt.Fprintf(&out, "\n**%s errored.**\n", header)
	}

	return out.String()
}

func policyKindLabel(kind string) string {
	if kind == "sentinel" {
		return "Sentinel"
	}
	return "OPA"
}

// Unicode symbols matching Terraform CLI output.
const (
	symbolTick      = "\u2713" // ✓
	symbolCross     = "\u00d7" // ×
	symbolInfo      = "\u24be" // Ⓘ
	symbolArrow     = "\u2192" // →
	symbolDownArrow = "\u21b3" // ↳
	symbolDash      = "\u2e3a" // ⸺
)

// policyIcon returns a colored icon for a policy outcome, matching TF CLI.
func policyIcon(cs *iostreams.ColorScheme, status, enforcementLevel string) iostreams.String {
	switch status {
	case "passed":
		return cs.String(symbolTick).Color(cs.Green()).Bold()
	case "failed":
		if enforcementLevel == "advisory" {
			return cs.String(symbolInfo).Color(cs.Orange()).Bold()
		}
		return cs.String(symbolCross).Color(cs.Red()).Bold()
	default:
		return cs.String("-")
	}
}

// policyStatusLabel returns the display label for a policy status, matching TF CLI.
func policyStatusLabel(status, enforcementLevel string) string {
	switch status {
	case "passed":
		return "Passed"
	case "failed":
		if enforcementLevel == "advisory" {
			return "Advisory"
		}
		return "Failed"
	default:
		return status
	}
}

// taskStatusLabel returns a colored status string for a run task, matching TF CLI.
func taskStatusLabel(cs *iostreams.ColorScheme, status, enforcementLevel string) iostreams.String {
	switch status {
	case "passed":
		return cs.String("Passed").Color(cs.Green())
	case "failed":
		label := "Failed"
		if enforcementLevel != "" {
			label += " (" + strings.ToUpper(enforcementLevel[:1]) + enforcementLevel[1:] + ")"
		}
		return cs.String(label).Color(cs.Red())
	default:
		return cs.String(status)
	}
}

// --- Policy evaluations ---

func (d *summaryDisplayer) formatPolicyEvaluations(f format.Format) string {
	switch f {
	case format.Markdown:
		return d.formatPolicyEvaluationsMarkdown()
	default:
		return d.formatPolicyEvaluationsPretty()
	}
}

func (d *summaryDisplayer) formatPolicyEvaluationsPretty() string {
	cs := d.io.ColorScheme()
	var out strings.Builder

	out.WriteString(cs.String("Policy Evaluations").Bold().String())
	out.WriteString("\n")

	for i, eval := range d.summary.PolicyEvaluations {
		out.WriteString("\n")
		kind := policyKindLabel(eval.PolicyKind)
		out.WriteString(cs.String(kind + " Policy Evaluation").Bold().String())
		out.WriteString("\n")

		if eval.Error != "" {
			fmt.Fprintf(&out, "%s %s %s\n",
				cs.String(symbolArrow+symbolArrow).Bold(),
				cs.String("Overall Result:").Bold(),
				cs.String("ERRORED").Color(cs.Red()).Bold())
			fmt.Fprintf(&out, "  %s\n", cs.String(eval.Error).Faint())
			if eval.PolicySetName != "" {
				fmt.Fprintf(&out, "\n%s Policy set 1: %s\n",
					cs.String(symbolArrow).Bold(),
					cs.String(eval.PolicySetName).Bold())
			}
			continue
		}

		// Compute overall result.
		overallResult := "PASSED"
		overallColor := cs.Green()
		hasAdvisoryFail := false
		hasMandatoryFail := false
		for _, oc := range eval.Outcomes {
			if oc.Status == "failed" {
				if oc.EnforcementLevel == "advisory" {
					hasAdvisoryFail = true
				} else {
					hasMandatoryFail = true
				}
			}
		}
		if hasMandatoryFail {
			overallResult = "FAILED"
			overallColor = cs.Red()
		} else if hasAdvisoryFail {
			overallResult = "PASSED (with advisory)"
			overallColor = cs.Green()
		}

		fmt.Fprintf(&out, "%s %s %s\n",
			cs.String(symbolArrow+symbolArrow).Bold(),
			cs.String("Overall Result:").Bold(),
			cs.String(overallResult).Color(overallColor).Bold())
		if hasMandatoryFail {
			fmt.Fprintf(&out, "  %s\n", cs.String("This result means that one or more OPA policies failed").Faint())
		} else if hasAdvisoryFail {
			fmt.Fprintf(&out, "  %s\n", cs.String("This result means that all OPA policies passed and the protected behavior is allowed").Faint())
		}
		fmt.Fprintf(&out, "%d policies evaluated\n", len(eval.Outcomes))

		if eval.PolicySetName != "" {
			fmt.Fprintf(&out, "\n%s Policy set %d: %s (%d)\n",
				cs.String(symbolArrow).Bold(),
				i+1,
				cs.String(eval.PolicySetName).Bold(),
				len(eval.Outcomes))
		}

		for _, oc := range eval.Outcomes {
			icon := policyIcon(cs, oc.Status, oc.EnforcementLevel)
			label := policyStatusLabel(oc.Status, oc.EnforcementLevel)
			fmt.Fprintf(&out, "  %s Policy name: %s\n", cs.String(symbolDownArrow).Bold(), cs.String(oc.PolicyName).Bold())
			fmt.Fprintf(&out, "     | %s %s\n", icon, label)
			if oc.Description != "" {
				fmt.Fprintf(&out, "     | %s\n", cs.String(oc.Description).Faint())
			}
			for _, line := range oc.Output {
				fmt.Fprintf(&out, "     | %s\n", line)
			}
		}
	}

	return out.String()
}

func (d *summaryDisplayer) formatPolicyEvaluationsMarkdown() string {
	var out strings.Builder

	out.WriteString("## Policy Evaluations\n\n")

	for i, eval := range d.summary.PolicyEvaluations {
		kind := policyKindLabel(eval.PolicyKind)
		fmt.Fprintf(&out, "### %s Policy Evaluation\n\n", kind)

		if eval.Error != "" {
			fmt.Fprintf(&out, "**Overall Result: ERRORED**\n\n")
			fmt.Fprintf(&out, "%s\n\n", eval.Error)
			if eval.PolicySetName != "" {
				fmt.Fprintf(&out, "**Policy set:** %s\n\n", eval.PolicySetName)
			}
			continue
		}

		// Compute overall result.
		overallResult := "PASSED"
		hasAdvisoryFail := false
		hasMandatoryFail := false
		for _, oc := range eval.Outcomes {
			if oc.Status == "failed" {
				if oc.EnforcementLevel == "advisory" {
					hasAdvisoryFail = true
				} else {
					hasMandatoryFail = true
				}
			}
		}
		if hasMandatoryFail {
			overallResult = "FAILED"
		} else if hasAdvisoryFail {
			overallResult = "PASSED (with advisory)"
		}

		fmt.Fprintf(&out, "**Overall Result: %s**\n\n", overallResult)
		fmt.Fprintf(&out, "%d policies evaluated\n\n", len(eval.Outcomes))

		if eval.PolicySetName != "" {
			fmt.Fprintf(&out, "**Policy set %d:** %s (%d)\n\n", i+1, eval.PolicySetName, len(eval.Outcomes))
		}

		for _, oc := range eval.Outcomes {
			label := policyStatusLabel(oc.Status, oc.EnforcementLevel)
			fmt.Fprintf(&out, "- **%s** — %s\n", oc.PolicyName, label)
			if oc.Description != "" {
				fmt.Fprintf(&out, "  %s\n", oc.Description)
			}
			for _, line := range oc.Output {
				fmt.Fprintf(&out, "  - %s\n", line)
			}
		}
		out.WriteString("\n")
	}

	return out.String()
}

// --- Task results ---

func (d *summaryDisplayer) formatTaskResults(f format.Format) string {
	switch f {
	case format.Markdown:
		return d.formatTaskResultsMarkdown()
	default:
		return d.formatTaskResultsPretty()
	}
}

func (d *summaryDisplayer) formatTaskResultsPretty() string {
	cs := d.io.ColorScheme()
	var out strings.Builder

	// Count passed and failed.
	passed, failed := 0, 0
	var mandatoryFailed []string
	for _, tr := range d.summary.TaskResults {
		if tr.Status == "passed" {
			passed++
		} else {
			failed++
			if tr.EnforcementLevel != "advisory" {
				mandatoryFailed = append(mandatoryFailed, tr.TaskName)
			}
		}
	}

	// Summary line.
	fmt.Fprintf(&out, "All tasks completed! %d passed, %d failed\n", passed, failed)

	// Per-task output.
	for _, tr := range d.summary.TaskResults {
		out.WriteString("\n")
		status := taskStatusLabel(cs, tr.Status, tr.EnforcementLevel)
		fmt.Fprintf(&out, "  %s %s   %s\n", cs.String(tr.TaskName).Bold(), symbolDash, status)
		if tr.Message != "" {
			fmt.Fprintf(&out, "  %s\n", cs.String(tr.Message).Faint())
		}
		if tr.URL != "" {
			fmt.Fprintf(&out, "  %s\n", cs.String("Details: "+tr.URL).Faint())
		}
	}

	// Error footer for mandatory failures.
	if len(mandatoryFailed) > 0 {
		out.WriteString("\n")
		if len(mandatoryFailed) == 1 {
			fmt.Fprintf(&out, "%s %s\n",
				cs.String("Error:").Color(cs.Red()),
				cs.String(fmt.Sprintf("the run failed because the run task, %s, is required to succeed", mandatoryFailed[0])).Bold())
		} else {
			fmt.Fprintf(&out, "%s %s\n",
				cs.String("Error:").Color(cs.Red()),
				cs.String(fmt.Sprintf("the run failed because %d mandatory tasks are required to succeed", len(mandatoryFailed))).Bold())
		}
	}

	// Overall result.
	out.WriteString("\n")
	overallLabel := "Passed"
	overallColor := cs.Green()
	if len(mandatoryFailed) > 0 {
		overallLabel = "Failed"
		overallColor = cs.Red()
	} else if failed > 0 {
		overallLabel = "Passed with advisory failures"
	}
	fmt.Fprintf(&out, "%s %s\n",
		cs.String("Overall Result:").Bold(),
		cs.String(overallLabel).Color(overallColor).Bold())

	return out.String()
}

func (d *summaryDisplayer) formatTaskResultsMarkdown() string {
	var out strings.Builder

	// Count passed and failed.
	passed, failed := 0, 0
	var mandatoryFailed []string
	for _, tr := range d.summary.TaskResults {
		if tr.Status == "passed" {
			passed++
		} else {
			failed++
			if tr.EnforcementLevel != "advisory" {
				mandatoryFailed = append(mandatoryFailed, tr.TaskName)
			}
		}
	}

	fmt.Fprintf(&out, "## Run Tasks\n\n")
	fmt.Fprintf(&out, "All tasks completed! %d passed, %d failed\n\n", passed, failed)

	for _, tr := range d.summary.TaskResults {
		label := "Passed"
		if tr.Status == "failed" {
			label = fmt.Sprintf("Failed (%s)", tr.EnforcementLevel)
		}
		fmt.Fprintf(&out, "- **%s** — %s\n", tr.TaskName, label)
		if tr.Message != "" {
			fmt.Fprintf(&out, "  %s\n", tr.Message)
		}
		if tr.URL != "" {
			fmt.Fprintf(&out, "  Details: %s\n", tr.URL)
		}
	}

	if len(mandatoryFailed) > 0 {
		out.WriteString("\n")
		if len(mandatoryFailed) == 1 {
			fmt.Fprintf(&out, "**Error:** the run failed because the run task, %s, is required to succeed\n", mandatoryFailed[0])
		} else {
			fmt.Fprintf(&out, "**Error:** the run failed because %d mandatory tasks are required to succeed\n", len(mandatoryFailed))
		}
	}

	overallLabel := "Passed"
	if len(mandatoryFailed) > 0 {
		overallLabel = "Failed"
	} else if failed > 0 {
		overallLabel = "Passed with advisory failures"
	}
	fmt.Fprintf(&out, "\n**Overall Result: %s**\n", overallLabel)

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
