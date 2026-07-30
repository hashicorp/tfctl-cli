// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tasks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadTasksStrictValidation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "06-count-by-version.yaml"), []byte(`
task: Count versions
tags: [pagination, jq]
accept: ['--(?:all|page-size)', '--jq']
reject: ['\|\s*jq\b']
turns: 3
`), 0o600))

	tasks, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, []Task{{
		ID: "count-by-version", Filename: "06-count-by-version.yaml", Prompt: "Count versions",
		Tags: []string{"pagination", "jq"}, Accept: []string{"--(?:all|page-size)", "--jq"}, Reject: []string{`\|\s*jq\b`}, Turns: 3,
	}}, tasks)

	tests := map[string]string{
		"unknown key":       "task: ok\naccept: [yes]\nextra: no\n",
		"empty prompt":      "task: '  '\naccept: [yes]\n",
		"no checks":         "task: ok\ntags: [one]\n",
		"empty check":       "task: ok\naccept: ['']\n",
		"invalid regexp":    "task: ok\naccept: ['[']\n",
		"nonpositive turns": "task: ok\naccept: [yes]\nturns: 0\n",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "01-task.yaml"), []byte(body), 0o600))
			_, err := Load(dir)
			require.Error(t, err)
		})
	}
}

func TestLoadTasksDefaultsTurnsAndRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "01-same.yaml"), []byte("task: first\naccept: [ok]\n"), 0o600))
	tasks, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, 10, tasks[0].Turns)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "02-same.yaml"), []byte("task: second\nreject: [bad]\n"), 0o600))
	_, err = Load(dir)
	require.ErrorContains(t, err, "duplicate task ID")
}

func TestGradeRequiresEveryAcceptRegexpAndForbidsEveryRejectRegexp(t *testing.T) {
	task := Task{
		Accept: []string{`tfctl\s+api`, `--(?:all|page-size)\b`, `/(?:plans|applies)/`},
		Reject: []string{`\|\s*jq\b`, `\bcurl\b`},
	}
	checks, passed := Grade(task, "TFCTL API /runs/id/plans/ --PAGE-SIZE 100")
	require.True(t, passed)
	require.Equal(t, []CheckResult{
		{Type: CheckAccept, Expected: `tfctl\s+api`, Passed: true},
		{Type: CheckAccept, Expected: `--(?:all|page-size)\b`, Passed: true},
		{Type: CheckAccept, Expected: `/(?:plans|applies)/`, Passed: true},
		{Type: CheckReject, Expected: `\|\s*jq\b`, Passed: true},
		{Type: CheckReject, Expected: `\bcurl\b`, Passed: true},
	}, checks)

	_, passed = Grade(Task{Accept: []string{`one`, `tw[o0]`}}, "ONE only")
	require.False(t, passed)
	_, passed = Grade(Task{Reject: []string{`nev(?:er|ah)`}}, "never do this")
	require.False(t, passed)
	checks, passed = Grade(Task{Reject: []string{`[`}}, "")
	require.False(t, passed)
	require.False(t, checks[0].Passed)
}

func TestFilterTasksByFilenameGlobAndTags(t *testing.T) {
	tasks := []Task{
		{ID: "alpha", Filename: "01-alpha.yaml", Tags: []string{"api", "safe"}},
		{ID: "beta", Filename: "02-beta.yaml", Tags: []string{"mutation"}},
		{ID: "gamma", Filename: "03-gamma.yaml", Tags: []string{"api"}},
	}

	filtered, err := Filter(tasks, "*a*.yaml", []string{"api"})
	require.NoError(t, err)
	require.Equal(t, []Task{tasks[0], tasks[2]}, filtered)

	filtered, err = Filter(tasks, "beta", nil)
	require.NoError(t, err)
	require.Equal(t, []Task{tasks[1]}, filtered)

	_, err = Filter(tasks, "[", nil)
	require.Error(t, err)
}

func TestRepositoryTasksLoad(t *testing.T) {
	tasks, err := Load("../../tasks")
	require.NoError(t, err)
	require.Len(t, tasks, 32)
	require.Equal(t, "list-workspaces-pagination", tasks[0].ID)

	var logTask Task
	for _, task := range tasks {
		if task.ID == "get-run-logs-completed" {
			logTask = task
			break
		}
	}
	require.Contains(t, logTask.Accept, `(?:/applies/)|(?:/plans/)`)
	require.Contains(t, logTask.Reject, `\|\s*jq`)
}
