// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tasks

// Task is a validated evaluation task.
type Task struct {
	ID       string
	Filename string
	Prompt   string   `yaml:"task"`
	Tags     []string `yaml:"tags,omitempty"`
	Accept   []string `yaml:"accept,omitempty"`
	Reject   []string `yaml:"reject,omitempty"`
	Turns    int      `yaml:"turns,omitempty"`
}

// CheckResult records one deterministic regular expression check.
type CheckResult struct {
	Type     string `json:"type"`
	Expected string `json:"expected"`
	Passed   bool   `json:"passed"`
}
