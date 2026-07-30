# tfctl Skill Evaluations

The evaluation runner sends each YAML task to a Responses API model with
`skills/tfctl/SKILL.md` as its instructions. It executes model-requested `tfctl`
function calls and returns their stdout, stderr, and exit code to the model.
Results save an ordered trace of model responses, visible messages, response
item metadata, tool calls, tool results, and provider errors. Response events
include per-turn token usage; tool results include call IDs, exit codes, stdout,
and stderr. API-provided reasoning summaries are included, but hidden reasoning
content is not persisted. The runner grades a rendered list of tool invocations
with case-insensitive Go regular expressions.

The evaluator is an independent Go module. Run repository-level Make targets
from the repository root; they build the current `tfctl` source before running.
For direct `go -C evals` commands, install `tfctl` on `PATH` first.

## Run

Amazon Bedrock is the default provider. Configure the standard AWS credential
chain and region (`AWS_REGION` or `AWS_DEFAULT_REGION`), or use
`AWS_BEARER_TOKEN_BEDROCK`.

```sh
make eval
make eval/save
make eval/baseline
make eval/compare
```

For OpenAI or an OpenAI-compatible endpoint, set `OPENAI_API_KEY`, optionally
set `OPENAI_BASE_URL`, and select the provider:

```sh
EVAL_PROVIDER=openai EVAL_MODEL=gpt-5.6 make eval
```

The runner accepts `--provider`, `--model`, `--output`, `--tags`, `--json`,
`--compare`, and `--task`. Flags override the corresponding `EVAL_*`
environment variables. `--tags` accepts comma-separated tags; `--task` accepts
a filename glob or substring.

## Tasks

Tasks live in `evals/tasks/` and use this strict schema:

```yaml
task: |
  List all workspaces and show their names.
tags: [api-pattern, pagination]
accept:
  - '--all'
  - '(?:/plans/)|(?:/applies/)'
reject: ['\|\s*jq']
turns: 10
```

Every `accept` expression must match the rendered tool invocations and every
`reject` expression must not match. Expressions use Go's RE2-compatible syntax
and are automatically case-insensitive. Use alternation such as
`(?:first)|(?:second)` when any accepted tool form is sufficient. Prefer the
smallest patterns that express the essential flags, paths, request data, or
outcomes; require a complete command shape only when the task specifically
depends on it. Invalid regular expressions are task validation errors. At least
one check is required.
The stable task ID comes from the filename with its numeric prefix and `.yaml`
suffix removed.

## Baselines

`make eval/baseline` writes `evals/baseline.json`. Review and commit that file.
Do not hand-author a baseline. Comparison runs fail only when a previously
passing task fails; provider errors and malformed inputs always fail.

Generated files under `evals/results/` are ignored. Override paths and runner
arguments when needed:

```sh
make eval/compare EVAL_BASELINE=path/to/baseline.json \
  EVAL_OUTPUT=evals/results/model.json EVAL_ARGS='--tags pagination'
```

Run evaluator checks independently with `make eval/test` and `make eval/lint`.

## CI

The `Skill Evals` workflow uses the Go version in `go.mod`, passes runner
configuration through `EVAL_*`, and uploads the current JSON result even on a
failure. Configure AWS credentials using repository secrets or workload
identity. For OpenAI, configure `OPENAI_API_KEY` as a repository secret.
