// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"testing"
)

func TestParseDiagnostics_Structured(t *testing.T) {
	log := `Terraform v1.5.0
on linux_amd64
Initializing plugins...
{"@level":"error","@message":"Error: Reference to undeclared resource","type":"diagnostic","diagnostic":{"severity":"error","summary":"Reference to undeclared resource","detail":"A managed resource \"random_string\" \"example\" has not been declared in the root module."}}
{"@level":"info","@message":"some info","type":"info"}
{"@level":"error","@message":"Error: Missing required argument","type":"diagnostic","diagnostic":{"severity":"error","summary":"Missing required argument","detail":"The argument \"name\" is required."}}`

	diags := parseDiagnostics(log)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	if diags[0].Summary != "Reference to undeclared resource" {
		t.Errorf("unexpected summary: %s", diags[0].Summary)
	}
	if diags[1].Summary != "Missing required argument" {
		t.Errorf("unexpected summary: %s", diags[1].Summary)
	}
}

func TestParseDiagnostics_NotStructured(t *testing.T) {
	log := `Terraform v1.5.0
on linux_amd64
Initializing plugins...
Error: something went wrong
This is plain text output, not JSON.`

	diags := parseDiagnostics(log)
	if diags != nil {
		t.Fatalf("expected nil diagnostics for non-structured log, got %d", len(diags))
	}
}

func TestParseDiagnostics_TooShort(t *testing.T) {
	log := `line1
line2
line3`

	diags := parseDiagnostics(log)
	if diags != nil {
		t.Fatalf("expected nil for short log, got %d", len(diags))
	}
}

func TestParseDiagnostics_NoDiagnosticType(t *testing.T) {
	log := `header1
header2
header3
{"@level":"info","@message":"Apply complete","type":"apply_complete"}`

	diags := parseDiagnostics(log)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}
