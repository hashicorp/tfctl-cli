---
name: tfctl
description: |
  Interact with HCP Terraform / Terraform Cloud / Terraform Enterprise using the tfctl CLI. Full API coverage.
  Use for ANY HCP Terraform or Terraform Cloud or Terraform Enterprise question or action — listing workspaces,
  starting/diagnosing runs, reading vars, modifying resources, calling API operations.
license: MPL-2.0
---

# tfctl — HCP Terraform CLI

Single binary, full v2 API coverage. Already authenticated.

## Hard rules

1. **Never pipe `tfctl` JSON to an external `jq`.** Use the built-in `--jq '<expr>'` flag — it implies `--json` and runs gojq on the response envelope.
2. **Never issue `-X DELETE`.** All deletes need a human. If asked to delete, print the exact command and ask the user to run it.
3. **Resolve names with `-p`, not separate lookup calls.** Paths with `{workspace}`/`{team}`/`{project}`/`{varset}` accept `-p workspace=NAME` etc. — tfctl resolves name→ID for you. Don't fetch the ID first.
4. **Trust the first answer.** `data: []`, `data: null`, `relationships.X.data: null`, or stderr "no current run"/"not found" ARE the answer. Don't re-query in another format. Don't walk relationships "to verify".
5. **When a named resource is not found, stop completely.** Exit code 2 or absence from a listing IS the full answer. Never:
   - Try a different resource ID "to verify the endpoint works"
   - Pivot to another org/workspace that appeared in the available list
   - Explore related resources to find "similar" information
   - Use Rule 4 to justify switching to a different resource: if you listed orgs and 'platform' isn't there, the first answer is "platform doesn't exist" — stop, don't use whatever org IS listed instead.
   
   Examples: `run-POLICY` returns exit 2 → stop, don't query other run IDs. Listing orgs shows no 'platform' → stop, don't use the org that IS listed.

### URL shape: per-workspace subpaths live at `/workspaces/{workspace}/...`

Most "things attached to a workspace" (vars, runs, varsets, remote-state-consumers, configuration-versions, notification-configurations, state-versions) are under `/workspaces/{workspace}/...`, NOT `/organizations/{org}/workspaces/{name}/...`. The org-nested form only exists for the workspace resource itself (`/organizations/{org}/workspaces/{name}`). For everything else, use `/workspaces/{workspace}/X -p workspace=NAME`.

**Exception: Policy checks and run-specific data** live under `/runs/{run-id}/...`, not under workspace paths. For example: `/runs/{run-id}/policy-checks`.

### Anti-patterns to avoid

These paths **do not exist**; don't try them:
- ❌ `/organizations/{org}/workspaces/{name}/vars` — use `/workspaces/{workspace}/vars -p workspace=NAME` instead
- ❌ `/organizations/{org}/workspaces/{name}/state-versions` — use `/workspaces/{workspace}/state-versions -p workspace=NAME`
- ❌ `/organizations/{org}/workspaces/{name}/configuration-versions` — use `/workspaces/{workspace}/configuration-versions -p workspace=NAME`
- ❌ `/workspaces/{workspace}/policy-checks` — use `/runs/{run-id}/policy-checks` instead
- ❌ `/organizations/{org}/varsets` (partially wrong path) — use `/organizations/{organization}/varsets` with correct org placeholder
- ❌ External `jq` pipes like `tfctl ... | jq '...'` — always use `tfctl api ... --jq '...'` with built-in flag

## Cookbook — one-line answers for common tasks

```bash
# Count workspaces in an org
tfctl api /organizations/{organization}/workspaces --page-size 1 --jq '.meta.pagination.["total-count"]'

# Find workspace by partial name (server-side search) — also returns current run state in one call
tfctl api /organizations/{organization}/workspaces -f 'search[name]=TERM' --jq '.data[] | {id, name: .attributes.name, current_run: .relationships.["current-run"].data}'

# Filter workspaces by attribute
tfctl api /organizations/{organization}/workspaces --all --jq '.data[] | select(.attributes.["terraform-version"] | startswith("1.8")) | .attributes.name'

# List variables on a workspace — path is /workspaces/{workspace}/vars (NOT /organizations/{org}/workspaces/{name}/vars; that endpoint does not exist)
tfctl api /workspaces/{workspace}/vars -p workspace=NAME --jq '.data[] | {key: .attributes.key, category: .attributes.category, sensitive: .attributes.sensitive}'

# Get current run status
tfctl run status NAME_OR_ID            # If it prints "no current run" or exits non-zero with that message, that IS the answer.

# Get current run ID for a workspace
tfctl api /organizations/{organization}/workspaces/NAME --jq '.data.relationships.["current-run"].data.id'

# List runs in a workspace with status filtering
tfctl api /workspaces/{workspace}/runs -p workspace=NAME --jq '.data[] | select(.attributes.status == "planned") | {id, status: .attributes.status}'

# Find workspace by VCS repo identifier (single call; no results = not connected to that repo)
tfctl api /organizations/{organization}/workspaces --all \
  --jq '.data[] | select(.attributes.["vcs-repo"] != null and .attributes.["vcs-repo"].identifier == "org/repo") | {id: .id, name: .attributes.name}'

# List workspaces accessible to a team (two-step: resolve team name → query team-workspaces)
# Note: /organizations/{org}/teams/{id}/workspaces does NOT exist — use /team-workspaces instead
tfctl api /organizations/{organization}/teams -f 'filter[names]=TEAM_NAME' --jq '.data[0].id'
tfctl api /team-workspaces -f 'filter[team][id]=TEAM_ID' --jq '.data[] | {workspace_id: .relationships.workspace.data.id, access: .attributes.access}'

# Get organization details and settings
tfctl api /organizations/{organization} --jq '.data | {id, name: .attributes.name, created_at: .attributes."created-at", terraform_version_default: .attributes."terraform-version"}'

# Get the current state version for a workspace
# (operationId: getCurrentStateVersion — single resource, not a list)
tfctl api /workspaces/{workspace}/current-state-version -p workspace=NAME --jq '.data | {serial: .attributes.serial, created_at: .attributes.["created-at"], status: .attributes.status}'

# List all variable sets in an org and their variable counts
tfctl api /organizations/{organization}/varsets --all --jq '.data[] | {name: .attributes.name, id: .id, var_count: (.relationships.vars.data | length)}'

# Get configuration version details
tfctl api /workspaces/{workspace}/configuration-versions -p workspace=NAME --jq '.data[] | {id, source: .attributes.source, created_at: .attributes.created-at}'

# List notification configurations
tfctl api /workspaces/{workspace}/notification-configurations -p workspace=NAME --jq '.data[] | {id, type: .attributes.destination-type, trigger: .attributes.triggers}'

# Filter workspaces by terraform version (1.7+)
tfctl api /organizations/{organization}/workspaces --all --jq '.data[] | select(.attributes.["terraform-version"] | ltrimstr("v") | split(".") | [.[0], .[1]] | join(".") | tonumber >= 1.7) | {name: .attributes.name, tf_version: .attributes.["terraform-version"]}'

# Filter workspaces excluding certain names
tfctl api /organizations/{organization}/workspaces --all --jq '.data[] | select(.attributes.name | test("^temp-|^old-") | not) | .attributes.name'

# Count resources by status (e.g., runs by status)
tfctl api /workspaces/{workspace}/runs -p workspace=NAME --all --jq '[.data[] | .attributes.status] | group_by(.) | map({status: .[0], count: length})'

# Get log URL for a completed run
# Note: run.log-read-url is null on completed runs — the URL lives on plan/apply.
tfctl api /runs/RUN_ID --jq '.data.relationships | {plan: .plan.data.id, apply: .apply.data.id}'
tfctl api /plans/PLAN_ID --jq '.data.attributes.["log-read-url"]'

# Start a run
tfctl run start NAME_OR_ID

# Add a remote state consumer to a workspace
# (operationId: addWorkspaceRemoteStateConsumers — POST returns 204)
tfctl api /workspaces/{workspace}/relationships/remote-state-consumers -p workspace=NAME \
  -i '{"data":[{"type":"workspaces","id":"ws-CONSUMER_ID"}]}'

# Apply a variable set to a workspace
# (operationId: updateWorkspaceRelationship for varsets)
tfctl api /workspaces/{workspace}/relationships/varsets -p workspace=NAME \
  -X POST -i '{"data":[{"type":"varsets","id":"varset-VARSET_ID"}]}'

# Get policy check results for a run
tfctl api /runs/{run-id}/policy-checks --jq '.data[] | {id: .id, status: .attributes.status, enforced: .attributes.enforcement-level}'

# Discover an API operation when you don't know it
tfctl api schema search "KEYWORD" --json     # returns operationIds
tfctl api schema get OPERATION_ID            # full OpenAPI schema (large response — only call when needed)
```

## Output flags

| Need              | Flag             |
|-------------------|------------------|
| Filter / extract  | `--jq '<expr>'`  |
| Full JSON         | `--json`         |
| Render for human  | `--markdown`     |
| Audit a mutation  | `--dry-run`      |

`--jq` implies `--json`. Don't pass both. Always pass one explicitly — don't rely on auto-detect.

## JSON:API conventions

Responses are JSON:API envelopes: `{data: {id, type, attributes, relationships}}` (or `data: [...]` for lists). To follow a link, read `data.relationships.<name>.data` which gives `{id, type}` or `null`. A `null` is final — there is no related resource.

Pagination: default page 1. Add `--all` for all pages (cap 2000), or `--page-size N` / `--page-number N` for explicit control. For counts, use `--page-size 1 --jq '.meta.pagination.["total-count"]'`.

## Mutations & batch updates

```bash
# Create or update a single variable
tfctl api /workspaces/{workspace}/vars -p workspace=NAME -a key=VARKEY -a value=VALUE -a category=env

# Batch updates: use separate calls (HCP TFC doesn't support bulk POST for vars)
# Instead of trying /workspaces/{ws}/vars with multiple items, do:
tfctl api /workspaces/{workspace}/vars -p workspace=NAME -a key=VAR1 -a value=VAL1 -a category=env
tfctl api /workspaces/{workspace}/vars -p workspace=NAME -a key=VAR2 -a value=VAL2 -a category=env
# (Multiple calls cost more, but it's the supported pattern)

# PATCH with a raw body
tfctl api /workspaces/{workspace}/X -X PATCH -p workspace=NAME -i '{"data":{"type":"X","attributes":{…}}}'
```
Add `--dry-run` to preview without sending.

## Exit codes (quick map)

| Code | Meaning | Action |
|------|---------|--------|
| 0 | Success | Done |
| 1 | Informational message or usage error | Read stderr; if it says "no current run" or similar, **that IS the answer — stop** |
| 2 | Not found (workspace/run doesn't exist) OR invalid auth | Verify the ID/name is correct, then check token if still failing |
| 3 | Auth token expired or invalid | Re-authenticate |
| 4 | Network error | Retry after brief delay |
| 5 | Rate limited (429) or server error (5xx) | Retry with backoff |
| 6 | Resource has an error state | The error is already diagnosed in output (e.g., plan failed); read it |

**Important**: When an API returns `data: []` (empty list) or `data: null`, that IS the answer. Don't retry with different flags or endpoints.

### Common troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `exit 2` when listing workspaces | Organization doesn't exist or auth token has no access | Verify org name and re-authenticate |
| `exit 1` with "no current run" | Workspace simply has no active run (informational) | **This is the answer** — stop, don't verify |
| `exit 3` when making any API call | Auth token expired | Re-authenticate with `tfctl login` |
| `exit 5` (429 rate limit) | Too many requests | Wait and retry; tfctl will backoff automatically |
| `exit 6` with "plan is errored" | Terraform plan had syntax errors (not CLI error) | Read the plan output for details |
| Empty list (`data: []`) when filtering | No resources match criteria | Verify criteria is correct; empty list is valid answer |

## Smart defaults

- `{organization}` resolves from active profile if set.
- `{workspace}` resolves from a local `cloud {}` block in CWD.
- Already-formed IDs (`ws-…`, `team-…`, `prj-…`, `varset-…`) are passed through as-is.
