---
name: tfctl
description: |
  Interact with HCP Terraform using the tfctl CLI. Full API coverage. Use for ANY HCP Terraform
  or Terraform Cloud question or action.
license: MPL-2.0
---

# tfctl - HCP Terraform (Terraform Cloud) Workflow CLI

Full CLI coverage for the entire HCP Terraform API.

## Agent Invariants

**MUST follow these rules:**

1. **Choose the right output mode** — `--jq` when you need to filter/extract data; `--json` for full JSON; `--markdown` when presenting results to a human (see Output Modes below). **Never pipe to external `jq` — use `--jq` instead.**
2. **Check context** using `tfctl profile display` before assuming configuration
3. **API Discovery** The API is too large to document every resource. You can perform a search using `tfctl api schema search <keyword>` to find relevant operations. When you are ready to make an API request, get the full request schema in OpenAPI format using `tfctl api schema get <operationId>`
4. **No Delete Operations** using the `tfctl api` command. All delete methods must be confimed interactively. Always prompt a human to perform delete operations themselves.

### Output Modes

**Choosing a mode:**

| Goal                     | Flag            | Format                                                                                     |
|--------------------------|-----------------|--------------------------------------------------------------------------------------------|
| Filter/extract JSON data | `--jq '<expr>'` | Built-in jq filter (no external jq needed). Implies `--json`; filter runs on the envelope. |
| Full JSON output         | `--json`        | JSON envelope: `{data, relationships, meta}`                                               |
| Show results to a user   | `--markdown`    | GFM tables, structured Markdown                                                            |
| Audit mutations          | `--dry-run`     | No changes, only a description of what would be modified rendered to stderr.               |

Always pass `--json` or `--markdown` explicitly — auto-detection depends on config and may not produce the format you expect. Use `--markdown` when composing reports, summarizing data, or displaying results inline. `--agent` is for headless integration scripts.

**Other modes:** `--quiet` (no output), `--debug` (verbose/debug logging enabled), `--jq '<expr>'` (built-in jq filter — see below),

### CLI Introspection

Navigate unfamiliar commands with `--help`

```bash
tfctl api --help
```

Walk the tree: start at `tfctl --help` for top-level commands, then drill into any subcommand. Commands include `EXAMPLES` with real invocation examples.

### Smart Defaults

- Some commands require an `--organization` argument, but it can be omitted if there is a profile default.
- Some commands require a `--workspace` argument, but it can be omitted if there is a terraform cloud block in the CWD.

## Quick Reference

| Task                  | Command                                                                 |
|-----------------------|-------------------------------------------------------------------------|
| Find an API operation | `tfctl api schema search <keyword> --json`                              |
| Get API schema        | `tfctl api schema get <operation>`                                      |
| List projects         | `tfctl api /organizations/{organization}/projects --json`               |
| Get Workspace state   | `tfctl api /organizations/{organization}/workspaces/{workspace} --json` |
| Run diagnostics       | `tfctl run status {run id or workspace}`                                |
| Start a run           | `tfctl run start {workspace}`                                           |

## API Conventions

The API follows a JSON:API standard convention, which all resources appearing within a JSON:API resource envelope with a `type` attribute.

Most related resources, such as the current run of a workspace, can be followed by consulting the `relationships/<key>` property of the resource envelope.

**URL patterns:**
- All resource collections are typically nested one resource deep. For example, `/organizations/{name}/workspaces` and `/workspaces/{id}/vars`.

**Pagination:**
When fetching lists of resources using the `api` command, the API returns paginated results. By default, only the first page of results is returned. You can use the following flags to control pagination:

- Use `--all` to fetch all pages of results (up to 1000 items). By default, only the first page is returned.
- Use `--page-size` to limit the number of items returned (default varies by resource).
- Use `--page-number` to specify the page number to fetch (default is 1).

## Decision Trees

### Finding Content

```
Need to find something?
├── Don't know the resource ID? → find by name using a list operation with a filter parameter
├── Workspace run logs? → Get the current-run ID from the workspace and `tfctl api /runs/{id} --jq '.data.attributes.["log-read-url"]'`
└── All else fail? → tfctl api schema search "query" --json
```

### Modifying Content

```
Want to change something?
├── Need PATCH schema? → `tfctl api schema get <operation>`
└── Have ID? → `tfctl /path/to/{id} -X PATCH -i'{ ...request body... }'`
```

## Common Workflows

### Diagnosing run errors

You can diagnose a particular run or the current run using:

```bash
$ tfctl run status ID
```

Where ID is either a run- ID, a ws- workspace ID, or a workspace name. You may need --organization flag unless there is a default organization set in the profile.

If a run is in an errored state due to a configuration issue make the necessary adjustments and then start a new run with:

```bash
$ tfctl run start WORKSPACE
```

## Common Errors and Debugging

**Rate limiting (429):** The CLI handles backoff automatically. If you see 429 errors (exit code 5), reduce request frequency.

**Missing argument errors (exit 1):**
When a required positional argument is missing, the CLI returns a structured error naming
the specific argument. Use this for elicitation:

```bash
$ tfctl <command> --help
```

**Not found/Authorization errors (exit 2):**
For security reasons, unauthorized access errors look identical to resource not found errors.
Verify you are signed in as the expected account.

```bash
tfctl api /account/details                      # Verify auth working
tfctl profile display                           # Check current configuration
```

**Authentication errors (exit 3):**
This could indicate a token misconfiguration.

```bash
tfctl api /account/details                      # Verify auth working
tfctl profile display                           # Check current configuration
```

**Network errors (exit 4):**
This could indicate a temporary network condition or a hostname misconfiguration.

```bash
tfctl profile display                           # Check current hostname configuration
```

**API errors (exit 5):**
This could indicate a bug in the platform API or an inability to process the command in a timely
manner. Try again or try a workaround.

**Underlying error detected (exit 6):**
The command ran successfully but the inspected resource is in an error state. For example,
`tfctl run status` returns exit 6 when the run has errored. The command will have already
printed diagnostic output.

## Built-in jq Filtering

The CLI has a built-in `--jq` flag powered by gojq — no external `jq` binary required. **Always prefer `--jq` over piping to external `jq`.**

```bash
# Extract fields from data array and filter by attribute
tfctl api /organizations/{organization}/workspaces --jq '.data[] | select(.attributes.["terraform-version"] != "1.15.1") | .relationships.["current-run"].data.id'

# Access envelope metadata
tfctl api /organizations/{organization}/projects --jq '.meta.pagination.["total-count"]'
```

`--jq` implies `--json` — no need to pass both. String results print as plain text; objects and arrays print as formatted JSON.

## Exit Codes

| Exit | Meaning                          | Solution                              |
|------|----------------------------------|---------------------------------------|
| 0    | OK                               | &mdash;                               |
| 1    | Usage error                      | Read `tfctl <cmd> --help`             |
| 2    | Not Found or Authorization Error | Verify URL/ID                         |
| 3    | Authentication Error             | `tfctl auth login`                    |
| 4    | Network error                    | Check connectivity                    |
| 5    | API Server Error Persists        | Try again later                       |
| 6    | Underlying error detected        | Command succeeded but found a problem |
| 130  | Canceled (ctrl-c).               | &mdash;                               |

## Learn More

- API overview: https://developer.hashicorp.com/terraform/cloud-docs/api-docs
