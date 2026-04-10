// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTFVarsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "vars.tfvars")
	content := []byte("name = \"example\"\ncount = 3\nenabled = true\nsettings = { env = \"prod\" }\nsecret_token = \"abc\"\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	vars, err := ParseTFVarsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 5 {
		t.Fatalf("got %d vars", len(vars))
	}

	seen := map[string]ImportedVariable{}
	for _, variable := range vars {
		seen[variable.Key] = variable
	}
	if seen["name"].HCL {
		t.Fatal("expected string variable to stay non-HCL")
	}
	if !seen["count"].HCL {
		t.Fatal("expected number variable to use HCL mode")
	}
	if !seen["settings"].HCL {
		t.Fatal("expected object variable to use HCL mode")
	}
	if !seen["secret_token"].Sensitive {
		t.Fatal("expected secret_token to be marked sensitive")
	}
}
