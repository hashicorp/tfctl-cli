// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package tasks loads and grades evaluation tasks.
package tasks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var orderingPrefix = regexp.MustCompile(`^\d+-`)

type taskFile struct {
	Prompt string   `yaml:"task"`
	Tags   []string `yaml:"tags,omitempty"`
	Accept []string `yaml:"accept,omitempty"`
	Reject []string `yaml:"reject,omitempty"`
	Turns  *int     `yaml:"turns,omitempty"`
}

// Load reads and strictly validates sorted YAML tasks from dir.
func Load(dir string) ([]Task, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	sort.Strings(paths)

	loaded := make([]Task, 0, len(paths))
	ids := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read task %q: %w", path, err)
		}
		var parsed taskFile
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&parsed); err != nil {
			return nil, fmt.Errorf("parse task %q: %w", path, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("parse task %q: multiple YAML documents are not allowed", path)
		}
		task := Task{Prompt: parsed.Prompt, Tags: parsed.Tags, Accept: parsed.Accept, Reject: parsed.Reject, Turns: 10}
		if parsed.Turns != nil {
			task.Turns = *parsed.Turns
		}
		task.Filename = filepath.Base(path)
		task.ID = strings.TrimSuffix(orderingPrefix.ReplaceAllString(task.Filename, ""), filepath.Ext(task.Filename))
		if err := validate(task); err != nil {
			return nil, fmt.Errorf("validate task %q: %w", task.Filename, err)
		}
		if previous, ok := ids[task.ID]; ok {
			return nil, fmt.Errorf("duplicate task ID %q in %q and %q", task.ID, previous, task.Filename)
		}
		ids[task.ID] = task.Filename
		loaded = append(loaded, task)
	}
	return loaded, nil
}

func validate(task Task) error {
	if strings.TrimSpace(task.Prompt) == "" || task.ID == "" {
		return fmt.Errorf("task prompt and ID must not be empty")
	}
	if len(task.Accept)+len(task.Reject) == 0 {
		return fmt.Errorf("at least one accept or reject check is required")
	}
	if task.Turns <= 0 {
		return fmt.Errorf("turns must be positive")
	}
	for _, pattern := range append(append([]string(nil), task.Accept...), task.Reject...) {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("checks must not be empty")
		}
		if _, err := regexp.Compile("(?i)" + pattern); err != nil {
			return fmt.Errorf("invalid check regexp %q: %w", pattern, err)
		}
	}
	return nil
}

// Grade matches all accept expressions and excludes all reject expressions.
func Grade(task Task, invocation string) ([]CheckResult, bool) {
	checks := make([]CheckResult, 0, len(task.Accept)+len(task.Reject))
	passed := true
	for _, pattern := range task.Accept {
		matched := regexp.MustCompile("(?i)" + pattern).MatchString(invocation)
		checks = append(checks, CheckResult{Type: "accept", Expected: pattern, Passed: matched})
		passed = passed && matched
	}
	for _, pattern := range task.Reject {
		matched := regexp.MustCompile("(?i)" + pattern).MatchString(invocation)
		checks = append(checks, CheckResult{Type: "reject", Expected: pattern, Passed: !matched})
		passed = passed && !matched
	}
	return checks, passed
}

// Filter selects tasks by filename glob or substring and optional tags.
func Filter(all []Task, filter string, tags []string) ([]Task, error) {
	pattern := filter
	if pattern != "" && !strings.ContainsAny(pattern, "*?[") {
		pattern = "*" + pattern + "*"
	}
	if pattern != "" {
		if _, err := filepath.Match(pattern, "validation"); err != nil {
			return nil, fmt.Errorf("invalid task glob %q: %w", filter, err)
		}
	}
	var filtered []Task
	for _, task := range all {
		filenameMatch, _ := filepath.Match(pattern, task.Filename)
		idMatch, _ := filepath.Match(pattern, task.ID)
		if pattern != "" && !filenameMatch && !idMatch {
			continue
		}
		if len(tags) > 0 && !hasTag(task.Tags, tags) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

func hasTag(taskTags, filters []string) bool {
	for _, filter := range filters {
		for _, tag := range taskTags {
			if strings.EqualFold(tag, filter) {
				return true
			}
		}
	}
	return false
}
