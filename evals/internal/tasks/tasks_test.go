// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStrictlyValidatesTasks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01-test.yaml")
	if err := os.WriteFile(path, []byte("task: test\naccept: [api]\nextra: no\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("Load() succeeded for an unknown field")
	}
}
