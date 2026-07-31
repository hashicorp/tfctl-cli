## v0.4.0 (July 31, 2026)

NEW FEATURES:

* This release adds `harness exec`. The command lets a user give noninteractive `tfctl` delete permission to a wrapped command. Use `--allow-delete=resources,other-resources` to give permission to delete those resource types for one session. The permission applies to all subprocesses.

* tfctl automatically updates outdated tfctl skills that it installed. tfctl does not update modified skills.

ENHANCEMENTS:

* tfctl now checks for a newer release when you run `tfctl`, `tfctl version`, or `tfctl --version`. These commands also tell you when you must run `auth login`.

* `run start` now has a `--plan-only` flag. The flag creates a speculative plan-only run. You cannot apply this run, regardless of the workspace auto-apply setting.

* tfctl now checks positional arguments and API path parameters for control characters and invalid UTF-8. It rejects malformed values before requests, terminal output, and audit output. These checks are not a security boundary. Your API token still controls authorization.

* The `TFCTL_CONFIG_DIR` environment variable can now set the configuration directory. Profiles and exec sessions use the specified directory. Use this variable to isolate tfctl state in continuous integration or test harnesses.

BUG FIXES:

* `auth login --dry-run` no longer opens a web browser. Argument completion now includes `--dry-run`.

* `profile display --markdown` no longer returns an error.

* The `profile profiles list --json` output no longer includes the Token property. This change prevents accidental token exposure.

* tfctl no longer tries to send telemetry again after responses such as the 429 rate-limit response.

* Authentication now detects Terraform `TF_TOKEN_<hostname>` environment variables, such as `TF_TOKEN_app_terraform_io`. This behavior matches Terraform CLI token resolution. Detection supports punycode hostnames and both Terraform dash encodings. You can encode each dash as `-` or `__`. tfctl did not previously detect these tokens.

* The not-found (404) API error now tells you to verify the request path and resource IDs. It no longer suggests an authentication problem.

* `api --all` now returns output when a result has only one page. Previously, the pagination check read the response body but did not restore it. Thus, the command returned no output.

* `api --json` and `api --jq` no longer put one-to-one relationship IDs in `attributes`. Table and pretty output still show these IDs. Raw JSON output remains unchanged.

* Telemetry payloads now redact customer data in HTTP paths, such as organization and workspace names.

* `--quiet` no longer suppresses output from `api` GET requests.

NOTES:

* This release adds `CONTRIBUTING.md`, `AGENTS.md`, developer setup automation, and `make help`. These resources make the initial development setup easier.
