# tfctl skill evals

Automated evals for the tfctl SKILL.md using [Microsoft Waza](https://github.com/microsoft/waza).

Tests whether models correctly follow the skill instructions when given common
tfctl prompts. Covers API usage patterns, error handling, safety rules, and
endpoint correctness.

## Setup

Install waza (Go binary, no dependencies):

```bash
curl -fsSL https://raw.githubusercontent.com/microsoft/waza/main/install.sh | bash
```

Authenticate with GitHub Copilot (one-time device flow):

```bash
~/Library/Caches/copilot-sdk/copilot_1.0.49 login
```

Note: if you have `GITHUB_TOKEN` set as a classic PAT (`ghp_...`), unset it
before running evals. Copilot rejects classic PATs, it needs the OAuth token
from the device flow above.

## Running evals

```bash
# full suite against claude sonnet
unset GITHUB_TOKEN
waza run evals/tfctl/eval.yaml --model claude-sonnet-4.6

# single task (glob on task ID)
waza run evals/tfctl/eval.yaml --task "refuse*" -v

# by tag
waza run evals/tfctl/eval.yaml --tags "safety" -v

# save results
waza run evals/tfctl/eval.yaml --model claude-sonnet-4.6 -o results/sonnet.json

# compare models
waza run evals/tfctl/eval.yaml --model gpt-4.1 -o results/gpt4.json
waza compare results/sonnet.json results/gpt4.json

# validate yaml structure without tokens (mock executor)
# change executor to 'mock' in eval.yaml, then:
waza run evals/tfctl/eval.yaml -v

# dashboard
waza serve
```

Full suite takes ~15 min sequential. Use `--parallel --workers 4` to speed up.

## What's tested

29 tasks adapted from v22 eval suite for TF agentic workflow skills.

| Category | Count | Examples |
|----------|-------|---------|
| API patterns | 16 | correct endpoints, `--all` for pagination, `--jq` filtering, `-p` name resolution |
| Error handling | 4 | stop on exit 2 (not found), exit 3 (auth expired), no retries |
| Safety | 5 | refuse deletes, never pivot to wrong org, stop on missing resources |
| Relationships | 3 | follow JSON:API relationships for plan/apply data |
| Schema | 1 | use `tfctl api schema search` |

All validators are deterministic (string contains/not-contains, behavioral
constraints). No LLM-as-judge graders yet. This keeps runs cheap and reproducible.

## Results (Claude Sonnet 4.6)

29/29 passing as of 2025-05-27.

## Adding evals

Create a new YAML file in `evals/tfctl/tasks/`:

```yaml
id: my-new-test
name: Short description
description: What this tests and why.
tags:
  - api-pattern

inputs:
  prompt: |
    The prompt to send to the model.

expected:
  output_contains:
    - "string that must appear"
  output_not_contains:
    - "string that must not appear"
  output_contains_any:
    - "at least one of these"
    - "must appear"
  behavior:
    max_tool_calls: 3
```

Validate with mock first (`executor: mock` in eval.yaml), then run against a
real model.

## Token usage

Each task uses ~130K input tokens (mostly the SKILL.md injected as system
prompt) across ~6-7 turns. Copilot caches aggressively. Second runs of the
same suite see ~80% cache hits. Full 29-task suite is roughly 3M input tokens
total, but most of that is cached reads.

## Structure

```
skills/
  SKILL.md                  <- the skill under test (copy from skills/tfctl/)
evals/tfctl/
  eval.yaml                 <- eval spec (graders, config, task glob)
  tasks/
    00-list-workspaces-pagination.yaml
    01-find-workspace-partial-name.yaml
    ...
    28-unknown-workspace-id.yaml
results/                    <- gitignored, local only
```
