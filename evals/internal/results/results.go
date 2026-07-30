// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package results owns evaluation result persistence and comparison.
package results

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Save atomically writes a result as JSON.
func Save(path string, result Result) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create result directory: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(dir, ".eval-result-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary result: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return fmt.Errorf("set result permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write result: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync result: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close result: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace result: %w", err)
	}
	return nil
}

// Load reads a result with a supported schema version.
func Load(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read result: %w", err)
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, fmt.Errorf("parse result: %w", err)
	}
	if result.SchemaVersion != resultSchemaVersion {
		return Result{}, fmt.Errorf("unsupported result schema %d", result.SchemaVersion)
	}
	return result, nil
}

// Compare classifies task status changes between two results.
func Compare(baseline, current Result, filtered bool) Comparison {
	comparison := Comparison{
		BaselineProvider: baseline.Provider,
		BaselineModel:    baseline.Model,
		ProviderChanged:  baseline.Provider != current.Provider,
		ModelChanged:     baseline.Model != current.Model,
	}
	baselineByID := make(map[string]TaskResult, len(baseline.Tasks))
	for _, task := range baseline.Tasks {
		baselineByID[task.ID] = task
	}
	seen := make(map[string]bool, len(current.Tasks))
	for _, task := range current.Tasks {
		seen[task.ID] = true
		previous, ok := baselineByID[task.ID]
		if !ok {
			comparison.New++
			comparison.Tasks = append(comparison.Tasks, ComparisonTask{ID: task.ID, Status: ComparisonNew})
			continue
		}
		previousPassed := previous.Status == StatusPassed
		currentPassed := task.Status == StatusPassed
		status := ComparisonUnchanged
		switch {
		case previousPassed && !currentPassed:
			status = ComparisonRegression
			comparison.Regressions++
		case !previousPassed && currentPassed:
			status = ComparisonImprovement
			comparison.Improvements++
		default:
			comparison.Unchanged++
		}
		comparison.Tasks = append(comparison.Tasks, ComparisonTask{ID: task.ID, Status: status})
	}
	if !filtered {
		for _, task := range baseline.Tasks {
			if !seen[task.ID] {
				comparison.Removed++
				comparison.Tasks = append(comparison.Tasks, ComparisonTask{ID: task.ID, Status: ComparisonRemoved})
			}
		}
	}
	return comparison
}
