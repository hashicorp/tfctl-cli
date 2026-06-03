# tfctl: The HCP Terraform CLI

[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.25.10+-00ADD8?logo=go)](https://go.dev/)

Comprehensive, official CLI access to the HCP Terraform / Terraform Enterprise platform.

tfctl provides both high-level commands for common workflows (managing runs, variables, and workspaces) and direct API access for advanced automation. It supports multiple configuration profiles, allowing you to switch between different HCP Terraform organizations and Terraform Enterprise instances. It also integrates with AI coding agents to facilitate agent-assisted management of Terraform workflows.

![tfctl](assets/tfctl.png "tfctl")

## Installation

### Prerequisites

- The Go language, v1.25.10 or later
- Git
- Make

### Install tfctl

1. Clone the git [repository](https://github.com/hashicorp/tfctl-cli):
   - SSH: `git clone git@github.com:hashicorp/tfctl-cli.git`
   - HTTPS: `git clone https://github.com/hashicorp/tfctl-cli.git`
1. Change to the new directory: `cd tfctl-cli`
1. Run `make go/install`.

Binary releases available soon!

Verify the installation:

```bash
$ tfctl --version
```

### Install shell completion

Shell completion assists with command, argument, and API path completion and is highly recommended.

```bash
$ tfctl --autocomplete-install
```

You can uninstall shell completion with the `tfctl --autocomplete-uninstall` command.

### Install AI agent skill

tfctl ships with an agent skill that gives AI coding agents full access to HCP Terraform. You can install it using tfctl or NPX. Replace AGENT with one of the supported AI agents: `bob`, `claude`, `codex`, `copilot`, `gemini`, `opencode`, or `pi`.

To install the skill with tfctl, run:

```bash
$ tfctl harness install AGENT --global
```

To install the skill with NPX, run:

```bash
$ npx skills add hashicorp/tfctl-cli --skill 'tfctl'
```

This adds the skill to your user profile so that compatible agents can use tfctl on your behalf.

## Configure tfctl

tfctl uses a layered configuration system. Settings can be specified in profiles, environment variables, or local Terraform configuration, with a clear order of precedence.

### Set hostname

tfctl defaults to the HCP Terraform instance at app.terraform.io. To use a different HCP Terraform instance or your organization's Terraform Enterprise instance, configure it now. Replace HOST with your HCP Terraform hostname (`app.terraform.io` or `app.eu.terraform.io`), or Terraform Enterprise hostname.

```bash
$ tfctl profile set hostname HOST
```

### Set authentication token

Create and install a token for tfctl to use to authenticate with HCP Terraform or Terraform Enterprise.

```bash
$ tfctl auth login
```

tfctl will open a browser window to HCP Terraform or your Terraform Enterprise instance. Click the **Create an API token** button, give your token a description and set its expiration, then click the **Generate token** button. Copy and paste the new token into your terminal window. tfctl will not print the pasted token to the screen.

If you have not configured a tfctl token for the current profile, tfctl will check your Terraform configuration for a matching token. Refer to [Terraform tokens](#terraform-tokens) for more information.

### Set default organization

Configure the organization for tfctl to use. Replace NAME with your HCP Terraform or Terraform Enterprise organization name.

```bash
$ tfctl profile set organization NAME
```

### Manage profiles

tfctl supports multiple local profiles, accessible via the `tfctl profile profiles` subcommand. Use profiles to switch between HCP Terraform organizations and instances of HCP Terraform and Terraform Enterprise. To start using profiles, create one.

```bash
$ tfctl profile profiles create NAME
```

tfctl will activate the new profile automatically.

## Example usage

**Command Syntax:** `tfctl <command> [subcommand] [flags] [arguments]`

tfctl provides access to runs, variables, and other HCP Terraform features through named subcommands. tfctl also provides direct access to the [HCP Terraform API](https://developer.hashicorp.com/terraform/cloud-docs/api-docs) with the `tfctl api` subcommand.

```bash
# See status/diagnose Workspace current run
tfctl run status my-workspace

# Import variables from a tfvars file to the Terraform workspace configured in the current directory
tfctl variable import bigsecret.tfvars

# Import variables from a tfvars file to a new variable set
tfctl variable import bigsecret.tfvars --variable-set-name="production"

# Import environment variables available to the Terraform workspace configured in the current directory
tfctl variable import -e AWS_REGION -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY

# Execute any API v2 GET query
tfctl api /account/details # Table format
tfctl api /organizations --json # JSON format

# Execute any POST query by specifying -a for request body attributes in key=value format or -i for raw request body input
tfctl api /organizations/acme/projects -a "name=my-project" -a "description=A very fine project"

# ...or use a JSON input file as the body
tfctl api /organizations/acme/projects --input my-project.json

# ...or use stdin as the request body
./generate_hcptf_run.sh | tfctl api /runs --input -

# Fetch all workspaces (up to 1000 pages of data) and sorts by latest runs
tfctl api /organizations/acme/workspaces --all -f "sort=-current-run.created-at"
```

## Configuration reference

tfctl stores its configuration in the following directories, depending on your operating system:

- Linux/MacOS: `~/.config/tfctl`
- Windows: `%AppData%/tfctl`

tfctl stores configuration for individual profiles in the `profiles` subdirectory. For example, `profiles/default.hcl` stores the configuration for your default profile.

### Terraform tokens

If you have not configured a token for the current profile with `tfctl auth login`, tfctl will check your Terraform configuration for a matching token. This configuration is found in your Terraform configuration directory, for example `~/.terraform.d/credentials.tfrc.json`, or the corresponding Terraform environment variables, such as `TF_TOKEN_app_terraform_io`.

### Environment variables

If you have not configured a particular option for the current profile, tfctl will check the following environment variables:

`TFCTL_ORGANIZATION`: The default organization to use for commands that require an organization.

`TFCTL_HOSTNAME`: The Terraform Enterprise or HCP Terraform hostname to use. Defaults to `app.terraform.io`.

`TFCTL_TOKEN`: An HCP Terraform API token to use in conjunction with the default profile.

`TFCTL_TOKEN_<profile>`: An HCP Terraform API token to use in conjunction with the named profile.

`TF_TOKEN_<hostname>`: An HCP Terraform API token to use with the specified hostname with punycode formatting, e.g. `TF_TOKEN_app_terraform_io`. tfctl will use the Terraform token only if it has not been configured in any other way.

## Command reference

tfctl supports managing HCP Terraform runs and variables with the corresponding subcommands. It also provides direct access to the HCP Terraform API with the `api` subcommand.

Use the `--help` flag to print out detailed usage instructions. For example, `tfctl --help` to print out help for the tfctl CLI, and `tfctl run --help` for help with the `run` subcommand.

### Global flags

tfctl supports the following global flags.

- `--agent`, `--json`: Sets the output format to JSON.
  - Data type: Boolean flag
  - Defaults to false.

- `--debug`: Enable debug output.
  - Data type: Boolean flag
  - Defaults to false.

- `--dry-run`: Shows what would happen without actually changing anything.
  - Data type: Boolean flag
  - Defaults to false.

- `--jq=EXPRESSION`: A jq filter expression to apply to JSON output. Implies --json.
  - Data type: String
  - Optional parameter.

- `--markdown`: Sets the output format to markdown.
  - Data type: Boolean flag
  - Defaults to false.

- `--no-color`: Disables color output.
  - Data type: Boolean flag
  - Defaults to false.

- `--profile=NAME`: The profile to use. If omitted, the currently selected profile will be used.
  - Data type: String
  - Optional parameter.

- `--quiet`: Minimizes output and disables interactive prompting.
  - Data type: Boolean flag
  - Defaults to false.

- `--version`: Print the version of tfctl CLI.
  - Data type: Boolean flag
  - Defaults to false.

### `tfctl auth login` reference

**Usage:** `tfctl auth login [options]`

#### Description

Authenticate the tfctl CLI with HCP Terraform or Terraform Enterprise. Opens a browser to the token creation page for the configured hostname and prompts you to paste the generated token.

#### Examples

Login interactively:

```bash
$ tfctl auth login
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl auth status` reference

**Usage:** `tfctl auth status [options]`

#### Description

Check the status of the currently configured authentication token, including expiration date if available.

#### Examples

Check the status of the token for the configured host:

```bash
$ tfctl auth status
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl harness` reference

Install coding agent skills or print out coding agent context document.

#### Subcommands

- `context`: Print coding agent context for tfctl, suitable for AGENTS.md.
- `install AGENT`: Install coding agent skills for tfctl in your project directory.
  - `--global`: Install skills in the global user directory instead of the current project directory.

Available agents are: `bob`, `claude`, `codex`, `copilot`, `gemini`, `opencode`, and `pi`.

#### Example usage

Install tfctl skills for Bob in the current project directory.

```bash
$ tfctl harness install bob
```

Install tfctl skills for Claude in your user directory, for use with all projects.

```bash
$ tfctl harness install --global claude
```

Print out agent context for tfctl, suitable for AGENTS.md.

```bash
$ tfctl harness context
```

### `tfctl profile display` reference

**Usage:** `tfctl profile display [options]`

#### Description

Print out configuration for the active profile.

#### Examples

```bash
$ tfctl profile display
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile get` reference

**Usage:** `tfctl profile get PROPERTY [options]`

#### Description

Get the value of the given configuration property for the active profile.

#### Arguments

- `PROPERTY`: The configuration property name to retrieve.
  - Required argument
  - Data type: String

#### Examples

```bash
$ tfctl profile get organization
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile set` reference

**Usage:** `tfctl profile set PROPERTY VALUE [options]`

#### Description

Set the value of the given configuration property for the active profile.

#### Arguments

- `PROPERTY`: The configuration property name to set.
  - Required argument
  - Data type: String

- `VALUE`: The value to set for the property.
  - Required argument
  - Data type: String

#### Examples

Set the organization to "my-organization" for the active profile:

```bash
$ tfctl profile set organization my-organization
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile unset` reference

**Usage:** `tfctl profile unset PROPERTY [options]`

#### Description

Unset the value of the given configuration property for the active profile.

#### Arguments

- `PROPERTY`: The configuration property name to unset.
  - Required argument
  - Data type: String

#### Examples

```bash
$ tfctl profile unset organization
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles activate` reference

**Usage:** `tfctl profile profiles activate NAME [options]`

#### Description

Activate an existing named profile.

#### Arguments

- `NAME`: The profile name to activate.
  - Required argument
  - Data type: String

#### Examples

Switch to a profile by name:

```bash
$ tfctl profile profiles activate NAME
```

Switch back to the default profile:

```bash
$ tfctl profile profiles activate default
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles create` reference

**Usage:** `tfctl profile profiles create NAME [options]`

#### Description

Create a new profile, and activate it automatically unless `--no-activate` is specified.

#### Arguments

- `NAME`: The profile name to create.
  - Required argument
  - Data type: String

#### Flags

- `--no-activate`: Don't automatically activate the new profile.
  - Optional
  - Data type: Boolean flag
  - Default: false

- `--hostname`: Set the hostname for the new profile.
  - Optional
  - Data type: String

#### Examples

Create and switch to a new profile:

```bash
$ tfctl profile profiles create NAME --hostname=HOST
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles list` reference

**Usage:** `tfctl profile profiles list [options]`

#### Description

List existing profiles.

#### Examples

```bash
$ tfctl profile profiles list
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles delete` reference

**Usage:** `tfctl profile profiles delete PROFILE_NAMES [PROFILE_NAMES ...] [options]`

#### Description

Delete an existing named profile.

#### Arguments

- `PROFILE_NAMES`: One or more profile names to delete.
  - Required argument
  - Data type: String (repeatable)

#### Examples

```bash
$ tfctl profile profiles delete old-profile
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles rename` reference

**Usage:** `tfctl profile profiles rename NAME --new_name=NEW_NAME [options]`

#### Description

Rename an existing named profile.

#### Arguments

- `NAME`: The current profile name.
  - Required argument
  - Data type: String

#### Flags

- `--new_name`: Set the new profile name.
  - Required
  - Data type: String

#### Examples

```bash
$ tfctl profile profiles rename old-name --new_name=new-name
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl run start` reference

**Usage:** `tfctl run start WORKSPACE [options]`

#### Description

Start a new run on the workspace specified by ID or name.

#### Arguments

- `WORKSPACE`: Workspace ID or name.
  - Required argument
  - Data type: String

#### Flags

- `--allow-empty-apply`: Allow the run to proceed even if the plan has no changes.
  - Optional
  - Data type: Boolean flag
  - Default: false

- `--debugging-mode`: Enables trace logging for this run by setting TF_LOG=trace in the terraform environment for this run.
  - Optional
  - Data type: Boolean flag
  - Default: false

- `--message`: Attach a message to the run.
  - Optional
  - Data type: String

- `--organization`: Organization name, overrides profile's configured organization name.
  - Optional
  - Data type: String

#### Examples

Start a new run on an existing workspace named "my-workspace":

```bash
$ tfctl run start my-workspace
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl run status` reference

**Usage:** `tfctl run status ID [options]`

#### Description

Print out the status of a run by run ID, or the latest run on a workspace by workspace ID or name.

The ID argument can be:
- A run ID (run-...)
- A workspace ID (ws-...) to get the latest run
- A workspace name to get the latest run (may require --organization)

#### Arguments

- `ID`: Run ID, workspace ID, or workspace name.
  - Required argument
  - Data type: String

#### Flags

- `--organization`: Organization name.
  - Optional
  - Data type: String
  - Default: Defaults to profile or terraform cloud config context

#### Examples

Print out the status of a run with an ID of "run-1234abcd":

```bash
$ tfctl run status run-1234abcd
```

Print out the status of the latest run on a workspace using the workspace name:

```bash
$ tfctl run status my-workspace
```

Print out the status of the latest run on a workspace using the workspace ID:

```bash
$ tfctl run status ws-abc123xyz
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl variable import` reference

**Usage:** `tfctl variable import [TFVARS_FILE] [options]`

#### Description

Import Terraform variables from .tfvars files or environment variables from the tfctl process environment into Workspaces or Variable Sets.

Provide either a variable set or a workspace by name, or tfctl will scan the current working directory for Terraform configuration to attempt to determine the workspace name.

#### Arguments

- `TFVARS_FILE`: The .tfvars file to import variables from.
  - Optional argument
  - Data type: File path (string)

#### Flags

- `--env`, `-e`: Environment variable to import.
  - Repeatable
  - Data type: String

- `--organization`: Organization name.
  - Optional
  - Data type: String
  - Default: Defaults to config or terraform cloud config context

- `--overwrite`: Update matching existing variables instead of erroring.
  - Optional
  - Data type: Boolean flag
  - Default: false

- `--variable-set-name`: Target Variable Set by name.
  - Optional
  - Data type: String
  - Default: Defaults to workspace if not set

- `--workspace`: Workspace name override.
  - Optional
  - Data type: String
  - Default: Defaults to terraform cloud config context

#### Examples

Import Terraform variables from "terraform.tfvars" file to a workspace named "my-workspace":

```bash
$ tfctl variable import terraform.tfvars --workspace=my-workspace
```

Import multiple environment variables into a variable set named "my-varset", overwriting existing values if any:

```bash
$ tfctl variable import --overwrite --variable-set-name=my-varset --env=AWS_ACCESS_KEY_ID --env=AWS_SECRET_ACCESS_KEY
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl api` reference

**Usage:** `tfctl api PATH [options]`

#### Description

Perform any HCP Terraform API v2 request with the given path or URL.

The HCP Terraform API typically requires a resource ID as part of the path for resource-specific requests. To support this, tfctl interpolates parameter values in the `PATH` argument denoted by `{NAME}`. Whenever possible, tfctl will infer these values from the path, the active profile, or local Terraform configuration. You can provide values for named parameters with the `--pathparam` argument.

#### Arguments

- `PATH`: API path relative to configured host, or URL.
  - Required argument
  - Data type: String

#### Flags

- `--all`: Disable pagination and fetch all records, to a maximum of 2000.
  - Optional
  - Data type: Boolean flag
  - Default: false

- `--attribute`, `-a`: Set attribute in request body. Implies `--method=POST`.
  - Repeatable
  - Data type: String (NAME=VALUE format)

- `--field`, `-f`: Set query parameter in request URL.
  - Repeatable
  - Data type: String (KEY=VALUE format)

- `--header`, `-H`: Set request header.
  - Repeatable
  - Data type: String ('name: value' format)

- `--input`, `-i`: Set raw JSON request body, use `-` to read from stdin.
  - Optional
  - Data type: String or file path

- `--method`, `-X`: HTTP method to use (e.g. GET, POST, etc.)
  - Optional
  - Data type: String

- `--page-number`: Page number to return. Ignored if --all is set.
  - Optional
  - Data type: Number
  - Default: 1

- `--page-size`: Limit the number of records to return. Default varies by resource. Ignored if --all is set.
  - Optional
  - Data type: Number

- `--pathparam`, `-p`: Provide a hint for path parameter resolution.
  - Repeatable
  - Data type: String (NAME=VALUE format)

- `--type`, `-t`: Resource type for --attribute JSON:API request bodies.
  - Optional
  - Data type: String

#### Examples

Print out details about the account associated with the configured token in JSON format:

```bash
$ tfctl api /account/details --json
```

Create a project named "my-project" for the currently configured organization:

```bash
$ tfctl api /organizations/{organization}/projects --attribute="name=my-project" --attribute="description=A very fine project indeed."
```

Print a list of all the workspaces (up to a limit of 2000) for the currently configured organization, sorted by the time the last run was started:

```bash
$ tfctl api /organizations/{organization}/workspaces --all --field="sort=-current-run.created-at"
```

#### Related

- [`tfctl api schema`](#tfctl-api-schema-get-reference): Inspect API schema
- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl api schema get` reference

**Usage:** `tfctl api schema get OPERATION_ID_OR_PATH [options]`

#### Description

Show a trimmed OpenAPI document for a single operationId or all operations on an exact API path.

#### Arguments

- `OPERATION_ID_OR_PATH`: An exact OpenAPI operationId or an API path (starting with /) to inspect.
  - Required argument
  - Data type: String

#### Examples

Inspect the getWorkspace operation:

```bash
$ tfctl api schema get getWorkspace
```

Print out all operations available for workspaces:

```bash 
$ tfctl api schema get /organizations/{organization}/workspaces
```

#### Related

- [`tfctl api`](#tfctl-api-reference): Perform API requests
- [Global flags](#global-flags)

### `tfctl api schema search` reference

**Usage:** `tfctl api schema search QUERY [QUERY ...] [options]`

#### Description

Search API operations by keywords.

#### Arguments

- `QUERY`: The search query to match against API operations.
  - Required argument (repeatable)
  - Data type: String

#### Examples

Search for workspace-related operations:

```bash
$ tfctl api schema search workspace
```

#### Related

- [`tfctl api`](#tfctl-api-reference): Perform API requests
- [Global flags](#global-flags)

## Exit codes

tfctl will return the following shell exit codes, depending on whether the command exited successfully or with an error.

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
