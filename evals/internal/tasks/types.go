// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tasks

// Task is a validated evaluation task.
type Task struct {
	ID       string   `yaml:"-"`
	Filename string   `yaml:"-"`
	Prompt   string   `yaml:"task"`
	Tags     []string `yaml:"tags,omitempty"`
	Accept   []string `yaml:"accept,omitempty"`
	Reject   []string `yaml:"reject,omitempty"`
	Turns    int      `yaml:"turns,omitempty"`
}

// CheckType identifies an acceptance or rejection check.
type CheckType string

// Supported task check types.
const (
	CheckAccept CheckType = "accept"
	CheckReject CheckType = "reject"
)

// CheckResult records one deterministic regular expression check.
type CheckResult struct {
	Type     CheckType `json:"type"`
	Expected string    `json:"expected"`
	Passed   bool      `json:"passed"`
}
