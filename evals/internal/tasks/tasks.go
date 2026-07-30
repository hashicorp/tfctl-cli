// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package tasks loads, filters, and grades evaluation tasks.
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

	tasks := make([]Task, 0, len(paths))
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
			if err == nil {
				return nil, fmt.Errorf("parse task %q: multiple YAML documents are not allowed", path)
			}
			return nil, fmt.Errorf("parse task %q: %w", path, err)
		}

		task := Task{Prompt: parsed.Prompt, Tags: parsed.Tags, Accept: parsed.Accept, Reject: parsed.Reject, Turns: 10}
		if parsed.Turns != nil {
			task.Turns = *parsed.Turns
		}
		task.Filename = filepath.Base(path)
		task.ID = strings.TrimSuffix(orderingPrefix.ReplaceAllString(task.Filename, ""), filepath.Ext(task.Filename))
		if task.ID == "" {
			return nil, fmt.Errorf("derive task ID from %q: ID must not be empty", task.Filename)
		}
		if err := validateTask(task); err != nil {
			return nil, fmt.Errorf("validate task %q: %w", task.Filename, err)
		}
		if previous, ok := ids[task.ID]; ok {
			return nil, fmt.Errorf("duplicate task ID %q in %q and %q", task.ID, previous, task.Filename)
		}
		ids[task.ID] = task.Filename
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func validateTask(task Task) error {
	if strings.TrimSpace(task.Prompt) == "" {
		return fmt.Errorf("task prompt must not be empty")
	}
	if len(task.Accept)+len(task.Reject) == 0 {
		return fmt.Errorf("at least one accept or reject check is required")
	}
	for _, check := range append(append([]string(nil), task.Accept...), task.Reject...) {
		if strings.TrimSpace(check) == "" {
			return fmt.Errorf("checks must not be empty")
		}
		if _, err := regexp.Compile("(?i)" + check); err != nil {
			return fmt.Errorf("invalid check regexp %q: %w", check, err)
		}
	}
	if task.Turns <= 0 {
		return fmt.Errorf("turns must be positive")
	}
	return nil
}

// Grade evaluates all case-insensitive regular expression checks for a task.
func Grade(task Task, output string) ([]CheckResult, bool) {
	checks := make([]CheckResult, 0, len(task.Accept)+len(task.Reject))
	passed := true
	for _, expected := range task.Accept {
		matched, valid := matches(expected, output)
		checkPassed := valid && matched
		checks = append(checks, CheckResult{Type: CheckAccept, Expected: expected, Passed: checkPassed})
		passed = passed && checkPassed
	}
	for _, expected := range task.Reject {
		matched, valid := matches(expected, output)
		checkPassed := valid && !matched
		checks = append(checks, CheckResult{Type: CheckReject, Expected: expected, Passed: checkPassed})
		passed = passed && checkPassed
	}
	return checks, passed
}

func matches(pattern, output string) (bool, bool) {
	expression, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return false, false
	}
	return expression.MatchString(output), true
}

// Filter selects tasks by filename glob or substring and optional tags.
func Filter(tasks []Task, taskGlob string, tags []string) ([]Task, error) {
	pattern := taskGlob
	if pattern != "" && !strings.ContainsAny(pattern, "*?[") {
		pattern = "*" + pattern + "*"
	}
	if pattern != "" {
		if _, err := filepath.Match(pattern, "validation"); err != nil {
			return nil, fmt.Errorf("invalid task glob %q: %w", taskGlob, err)
		}
	}

	filtered := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if pattern != "" {
			filenameMatch, _ := filepath.Match(pattern, task.Filename)
			idMatch, _ := filepath.Match(pattern, task.ID)
			if !filenameMatch && !idMatch {
				continue
			}
		}
		if len(tags) > 0 && !hasAnyTag(task.Tags, tags) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

func hasAnyTag(taskTags, filters []string) bool {
	for _, filter := range filters {
		for _, tag := range taskTags {
			if strings.EqualFold(tag, filter) {
				return true
			}
		}
	}
	return false
}
