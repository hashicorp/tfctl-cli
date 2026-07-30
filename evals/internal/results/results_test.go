// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package results

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareResultsClassifiesChanges(t *testing.T) {
	baseline := Result{SchemaVersion: resultSchemaVersion, Provider: "old", Model: "old-model", Tasks: []TaskResult{
		{ID: "regression", Status: StatusPassed},
		{ID: "improvement", Status: StatusFailed},
		{ID: "same", Status: StatusPassed},
		{ID: "removed", Status: StatusPassed},
	}}
	current := Result{SchemaVersion: resultSchemaVersion, Provider: "new", Model: "new-model", Tasks: []TaskResult{
		{ID: "regression", Status: StatusFailed},
		{ID: "improvement", Status: StatusPassed},
		{ID: "same", Status: StatusPassed},
		{ID: "new", Status: StatusPassed},
	}}

	comparison := Compare(baseline, current, false)
	require.Equal(t, 1, comparison.Regressions)
	require.Equal(t, 1, comparison.Improvements)
	require.Equal(t, 1, comparison.Unchanged)
	require.Equal(t, 1, comparison.New)
	require.Equal(t, 1, comparison.Removed)
	require.True(t, comparison.ProviderChanged)
	require.True(t, comparison.ModelChanged)

	filtered := Compare(baseline, current, true)
	require.Zero(t, filtered.Removed)
}

func TestSaveAndLoadResultRoundTripAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "result.json")
	want := Result{
		SchemaVersion: resultSchemaVersion, Suite: suiteName, Provider: "bedrock", Model: "model", Total: 1, Passed: 1,
		Tasks: []TaskResult{{ID: "task", ToolUsage: "tfctl api /account/details", Trace: []TraceEntry{
			{Type: "message", Turn: 1, Role: "user", Content: "prompt"},
			{Type: "response", Turn: 1, ResponseID: "resp_1", Status: "completed", InputTokens: 2, OutputTokens: 3, ItemTypes: []string{"function_call"}},
			{Type: "response_item", Turn: 1, ItemID: "reasoning_1", ItemType: "reasoning", Summary: "Inspect the account first."},
			{Type: "tool_use", Turn: 1, ItemID: "item_1", CallID: "call_1", Name: "tfctl", Args: []string{"api", "/account/details"}},
			{Type: "tool_result", Turn: 1, CallID: "call_1", Name: "tfctl", ExitCode: intPtr(3), Stderr: "token expired"},
		}}},
	}
	require.NoError(t, Save(path, want))

	got, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, want, got)

	want.Model = "replacement"
	require.NoError(t, Save(path, want))
	got, err = Load(path)
	require.NoError(t, err)
	require.Equal(t, want, got)

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func intPtr(value int) *int {
	return &value
}

func TestLoadResultRejectsUnsupportedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"schema_version":99}`), 0o600))
	_, err := Load(path)
	require.ErrorContains(t, err, "unsupported result schema")
}
