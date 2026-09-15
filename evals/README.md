# tfctl Skill Evaluations

The evaluation runner sends each YAML task to a simple, custom, Google ADK agent with
`skills/tfctl/SKILL.md` as its instructions. The agent has one function tool,
`tfctl(args)`.

The runner executes ordinary `tfctl` requests with a temporary configuration
directory. A request that starts with `tfctl api`, `tfctl get`, or `tfctl create`
is not executed. The runner records that request, stops the agent, and grades
the recorded isolated request. This prevents a platform request and lets each
task run independently. If the agent makes no isolated request, the runner
grades its visible text output instead. The saved output includes model
reasoning, but task checks do not grade it.

The evaluator is an independent Go module. Run repository-level Make targets
from the repository root; they build the current `tfctl` source before running.
For direct `go -C evals` commands, install `tfctl` on `PATH` first.

## Providers

ADK Go's only OpenAI-shaped model connector
(`google.golang.org/adk/v2/model/openaimodel`) speaks the newer Responses
API, which neither a local OpenAI-compatible server nor AWS Bedrock speaks
natively. Rather than run a translating proxy (e.g. LiteLLM) in front of
either one, the runner talks to both directly through two small adapters
in `evals/internal/model`:

- `openaichat`: calls a Chat Completions endpoint (the format local model
  servers such as llama.cpp, vLLM, and Ollama actually speak).
- `bedrockconverse`: calls the AWS Bedrock Converse API using the standard
  AWS SDK credential chain.

Select a provider with `--provider` or `EVAL_PROVIDER`:

```sh
# A local OpenAI-compatible server
EVAL_PROVIDER=openai EVAL_MODEL=qwen3.8-Q8 make eval

# AWS Bedrock
EVAL_PROVIDER=bedrock EVAL_MODEL=us.openai.gpt-5.6-luna make eval/save
```

For the `openai` provider, `--base-url`/`EVAL_BASE_URL` sets the Chat
Completions base URL (default `http://127.0.0.1:8000/v1`) and
`--api-key`/`EVAL_API_KEY` sets an optional API key. `--model`/`EVAL_MODEL`
is the model name the server expects.

For the `bedrock` provider, `--model`/`EVAL_MODEL` is a Bedrock model ID or
cross-region inference profile ID (e.g. `us.anthropic.claude-...`).
Credentials and region come from the standard AWS SDK default chain:
`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, optional `AWS_SESSION_TOKEN`,
`AWS_REGION`, or an `AWS_PROFILE`.

`EVAL_PROVIDER` and `EVAL_MODEL` are required when `--provider`/`--model`
are not set. Do not set `GOOGLE_API_KEY`; the runner does not call Gemini.

## Run

```sh
EVAL_PROVIDER=openai EVAL_MODEL=qwen3.8-Q8 make eval
EVAL_PROVIDER=bedrock EVAL_MODEL=us.openai.gpt-5.6-luna make eval/save
```

The runner accepts `--provider`, `--model`, `--base-url`, `--api-key`,
`--output`, `--tags`, `--json`, and `--task`. Flags override the corresponding
`EVAL_*` environment variables. `--tags` accepts comma-separated tags; `--task`
accepts a filename glob or substring.

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

Every `accept` expression must match the isolated invocation, or the visible
assistant output when there is no isolated invocation. Every `reject`
expression must not match. Expressions use Go's RE2-compatible syntax and are
automatically case-insensitive. Use alternation such as
`(?:first)|(?:second)` when any accepted form is sufficient. Prefer the
smallest patterns that express required flags, paths, or request data. Invalid
regular expressions are task validation errors. At least one check is required.
The stable task ID comes from the filename with its numeric prefix and `.yaml`

Generated files under `evals/results/` are ignored. Override the output path
and runner arguments when needed:

```sh
make eval/save EVAL_OUTPUT=evals/results/pagination.json EVAL_ARGS='--tags pagination'
```

Run evaluator checks independently with `make eval/test` and `make eval/lint`.

## CI

The `Skill Evals` workflow runs against the `bedrock` provider, passes
runner configuration through `EVAL_*`, and uploads the current JSON result
even on a failure. AWS credentials (as provisioned by doormat) and the
Bedrock model ID are supplied as `workflow_dispatch` inputs.
