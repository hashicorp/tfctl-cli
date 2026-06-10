// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if diags[0].Range == nil {
		t.Fatal("expected range to be parsed")
	}
	if diags[0].Range.Filename != "main.tf" {
		t.Errorf("expected filename 'main.tf', got %q", diags[0].Range.Filename)
	}
	if diags[0].Range.Start.Line != 73 {
		t.Errorf("expected start line 73, got %d", diags[0].Range.Start.Line)
	}
	if diags[0].Snippet == nil {
		t.Fatal("expected snippet to be parsed")
	}
	if diags[0].Snippet.Code != "  value = random_string.example.result" {
		t.Errorf("unexpected snippet code: %q", diags[0].Snippet.Code)
	}
	if diags[0].Snippet.StartLine != 73 {
		t.Errorf("expected snippet start line 73, got %d", diags[0].Snippet.StartLine)
	}
	if diags[0].Snippet.HighlightStartOffset != 10 || diags[0].Snippet.HighlightEndOffset != 31 {
		t.Errorf("unexpected highlight offsets: %d-%d", diags[0].Snippet.HighlightStartOffset, diags[0].Snippet.HighlightEndOffset)
	}
}

func TestParseDiagnostics_RealConsoleLog(t *testing.T) {
	// Real console-mode plan log (pre-0.15.3 format) with \x02/\x03 framing.
	log := "\x02Terraform v0.14.11\n" +
		"on linux_amd64\n" +
		"Configuring remote state backend...\n" +
		"Initializing provider plugins...\n" +
		"- Using previously-installed hashicorp/aws v3.74.3\n" +
		"- Using previously-installed hashicorp/random v3.1.0\n" +
		"\n" +
		"Terraform has been successfully initialized!\n" +
		"\n" +
		"\n" +
		"------------------------------------------------------------------------\n" +
		"\n" +
		"Error: Reference to undeclared resource\n" +
		"\n" +
		"  on main.tf line 73, in output \"random_string\":\n" +
		"  73:   value = random_string.example.result\n" +
		"\n" +
		"A managed resource \"random_string\" \"example\" has not been declared in the\n" +
		"root module.\n" +
		"\n" +
		"\n" +
		"Error: Missing required argument\n" +
		"\n" +
		"  on main.tf line 15, in resource \"aws_instance\" \"web\":\n" +
		"  15:   ami = \"\"\n" +
		"\n" +
		"The argument \"ami\" is required, but no definition was found.\n" +
		"\n" +
		"Operation failed: failed running terraform plan (exit 1)\x03"

	diags := ParseDiagnostics(log)
	if diags != nil {
		t.Fatalf("expected nil diagnostics for console-mode log, got %d", len(diags))
	}
}

func Test_PopulatePolicyCheckSummary(t *testing.T) {
	t.Parallel()

	const wantBody = "redirect target content"

	// Target server that serves the final content.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("target Received request for %s\n", r.URL.Path)
		w.Header().Set("Content-Type", "application/octet-stream")
		fmt.Fprint(w, wantBody)
	}))
	t.Cleanup(target.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/runs/run-1/policy-checks" {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			fmt.Fprint(w, `{"data":[{"id":"polchk-1","attributes":{"status":"hard_failed"}}]}`)
			return
		}
		if r.URL.Path == "/api/v2/policy-checks/polchk-1/output" {
			t.Logf("api Received request for %s\n", r.URL.Path)
			http.Redirect(w, r, target.URL+"/log.txt", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(api.Close)

	c, err := New(context.Background(), api.URL, "test-token", nil)
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	result := &RunSummary{}

	ctx := context.Background()
	err = populatePolicyCheckSummary(ctx, c, "run-1", result)
	if err != nil {
		t.Fatalf("populatePolicyCheckSummary: %v", err)
	}

	if result.PolicyCheckLog != wantBody {
		t.Errorf("got %q, want %q", result.PolicyCheckLog, wantBody)
	}
}
