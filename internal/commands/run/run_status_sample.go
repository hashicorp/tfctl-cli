// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
)

// NewCmdRunStatusSample creates the hidden `run status-sample` command.
// It renders a pre-populated RunSummary with all potential failure types for
// visual inspection.
func NewCmdRunStatusSample(ctx *cmd.Context) *cmd.Command {
	return &cmd.Command{
		Name:           "status-sample",
		ShortHelp:      "Display a sample run status with all failure types.",
		Hidden:         true,
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, args []string) error {
			d := &summaryDisplayer{summary: sampleRunSummary(), io: ctx.IO}
			return ctx.Output.Display(d)
		},
	}
}

func sampleRunSummary() *client.RunSummary {
	ctx := "resource \"aws_instance\" \"web\""
	return &client.RunSummary{
		RunID:   "run-sample000000000000",
		Status:  "errored",
		Message: "Plan errored",
		Phase:   "post_plan",
		Diagnostics: []client.Diagnostic{
			{
				Severity: "error",
				Summary:  "Reference to undeclared input variable",
				Detail:   "An input variable with the name \"does_not_exist\" has not been declared. Did you mean \"instance_type\"?",
				Range: &client.DiagnosticRange{
					Filename: "main.tf",
					Start:    client.SourceLocation{Line: 2, Column: 11, Byte: 25},
					End:      client.SourceLocation{Line: 2, Column: 31, Byte: 45},
				},
				Snippet: &client.DiagnosticSnippet{
					Context:              &ctx,
					Code:                 "  ami = var.does_not_exist",
					StartLine:            2,
					HighlightStartOffset: 8,
					HighlightEndOffset:   26,
				},
			},
			{
				Severity: "warning",
				Summary:  "Deprecated attribute",
				Detail:   "The attribute \"instance_state\" is deprecated. Use \"state\" instead.",
				Range: &client.DiagnosticRange{
					Filename: "main.tf",
					Start:    client.SourceLocation{Line: 10, Column: 3, Byte: 100},
				},
			},
		},
		PolicyCheckLog:    "Sentinel Result: false\n\n## Policy 1: deny-all (hard-mandatory)\n\nResult: false\n",
		PolicyCheckScope:  "organization",
		PolicyCheckStatus: "hard_failed",
		PolicyEvaluations: []client.PolicyEvalResult{
			{
				PolicyKind:    "opa",
				PolicySetName: "security-policies",
				Outcomes: []client.PolicyOutcome{
					{
						PolicyName:       "deny-public-access",
						EnforcementLevel: "mandatory",
						Status:           "failed",
						Description:      "Ensures no resources are publicly accessible",
						Output:           []string{"aws_instance.web is publicly accessible via 0.0.0.0/0"},
					},
					{
						PolicyName:       "require-tags",
						EnforcementLevel: "advisory",
						Status:           "failed",
						Description:      "All resources must have required tags",
					},
					{
						PolicyName:       "cost-limit",
						EnforcementLevel: "mandatory",
						Status:           "passed",
					},
				},
			},
		},
		TaskResults: []client.TaskResult{
			{
				TaskName:         "security-scan",
				Status:           "failed",
				Message:          "3 critical vulnerabilities found in container image",
				URL:              "https://example.com/scans/run-sample000000000000",
				EnforcementLevel: "mandatory",
				Stage:            "post_plan",
			},
			{
				TaskName:         "cost-estimate",
				Status:           "passed",
				EnforcementLevel: "advisory",
				Stage:            "post_plan",
			},
			{
				TaskName:         "compliance-check",
				Status:           "failed",
				Message:          "Resource does not meet compliance requirements for PCI-DSS",
				URL:              "https://example.com/compliance/run-sample000000000000",
				EnforcementLevel: "advisory",
				Stage:            "post_plan",
			},
		},
	}
}
