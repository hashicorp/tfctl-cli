// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

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

	diags := ParseDiagnostics(log)
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

	diags := ParseDiagnostics(log)
	if diags != nil {
		t.Fatalf("expected nil diagnostics for non-structured log, got %d", len(diags))
	}
}

func TestParseDiagnostics_TooShort(t *testing.T) {
	log := `line1
line2
line3`

	diags := ParseDiagnostics(log)
	if diags != nil {
		t.Fatalf("expected nil for short log, got %d", len(diags))
	}
}

func TestParseDiagnostics_NoDiagnosticType(t *testing.T) {
	log := `header1
header2
header3
{"@level":"info","@message":"Apply complete","type":"apply_complete"}`

	diags := ParseDiagnostics(log)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestParseDiagnostics_RealHCPTerraformLog(t *testing.T) {
	log := "\x02Terraform v1.14.9\n" +
		"on linux_amd64\n" +
		"Initializing plugins and modules...\n" +
		`{"@level":"info","@message":"Terraform 1.14.9","@module":"terraform.ui","@timestamp":"2026-04-29T21:25:36.140680Z","terraform":"1.14.9","type":"version","ui":"1.2"}` + "\n" +
		`{"@level":"error","@message":"Error: Reference to undeclared resource","@module":"terraform.ui","@timestamp":"2026-04-29T21:25:37.016838Z","diagnostic":{"severity":"error","summary":"Reference to undeclared resource","detail":"A managed resource \"random_string\" \"example\" has not been declared in the root module.","range":{"filename":"main.tf","start":{"line":73,"column":11,"byte":1375},"end":{"line":73,"column":32,"byte":1396}},"snippet":{"context":"output \"random_string\"","code":"  value = random_string.example.result","start_line":73,"highlight_start_offset":10,"highlight_end_offset":31,"values":[]}},"type":"diagnostic"}` + "\n" +
		"Operation failed: failed running terraform plan (exit 1)\x03"

	diags := ParseDiagnostics(log)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	if diags[0].Severity != "error" {
		t.Errorf("expected severity 'error', got %q", diags[0].Severity)
	}
	if diags[0].Summary != "Reference to undeclared resource" {
		t.Errorf("expected summary 'Reference to undeclared resource', got %q", diags[0].Summary)
	}
	if diags[0].Detail != `A managed resource "random_string" "example" has not been declared in the root module.` {
		t.Errorf("unexpected detail: %q", diags[0].Detail)
	}
}
