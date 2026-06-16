# tfctl: The HCP Terraform CLI

[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)

Comprehensive, official CLI access to the HCP Terraform / Terraform Enterprise platform.

The `tfctl` CLI  provides high-level commands for common workflows, such as managing runs, variables, and workspaces, and direct API access for advanced automation. It supports multiple configuration profiles, allowing you to switch between different HCP Terraform organizations and Terraform Enterprise instances. It also integrates with AI coding agents to facilitate agent-assisted management of Terraform workflows.

![tfctl](assets/demo.gif "tfctl demo")

## Installation
You can install the CLI, command completion utility, and agent skill separately.  

### Homebrew

MacOS and Linux users can install tfctl from the HashiCorp brew tap:

```bash
$ brew install hashicorp/tap/tfctl
```

### Precompiled Binaries

You can download official release binaries at [releases.hashicorp.com](https://releases.hashicorp.com/tfctl/)

### Building from Source

See [Developers](#developers) below for details.

## Install shell completion

We recommend installing the shell completion module for command, argument, and API path completion.

```bash
$ tfctl --autocomplete-install
```

You can uninstall shell completion with the `tfctl --autocomplete-uninstall` command.

## Install AI agent skill

The `tfctl` CLI ships with an agent skill that gives AI coding agents access to HCP Terraform through the `tfctl` command, but discourages non-human delete operations. You can install it using the `tfctl harness install` command or NPX. Replace `<agent>` with one of the following supported AI agents:

- `antigravity`
- `bob` 
- `claude`
- `codex`
- `copilot`
- `opencode`
- `pi`

To install the skill with `tfctl harness install` command, run:

```bash
$ tfctl harness install <agent> --global
```

To install the skill for other agents using npx skills, run:

```bash
$ npx skills add hashicorp/tfctl-cli --skill 'tfctl'
```

This adds the skill to your user profile so that compatible agents can use `tfctl` commands on your behalf.

## Configure tfctl

The `tfctl` CLI uses a layered configuration system. You can configure settings in profiles, environment variables, or local Terraform configuration, in order of precedence.

### Set hostname

The default hostname is the HCP Terraform instance at `app.terraform.io`. To use a different HCP Terraform instance or your organization's Terraform Enterprise instance, use the `tfctl profile set hostname` command and specify the hostname you want the CLI to connect to.

```bash
$ tfctl profile set hostname <host>
```

### Set authentication token

Run the `tfctl auth login` command to create and install a token for `tfctl` to use to authenticate with HCP Terraform or Terraform Enterprise. If you already have a token, you can use `tfctl profile set token $TOKEN`.

```bash
$ tfctl auth login
```

The command opens HCP Terraform or your Terraform Enterprise instance in a browser window. Click the **Create an API token** button, then give your token a description and set its expiration. Click **Generate token**, then copy and paste the new token into your terminal window. The CLI does not print the pasted token to the screen. As with Terraform, `tfctl` stores these tokens in plain text. Refer to [Configuration reference](#configuration-reference) for more information.

Verify that the login is successful before leaving the token page in your browser, because HCP Terraform does not show the token once it's closed. If an issue occurs during the process or you close the dialog showing the token before copying it, you must create a new token.

If the CLI does not find a token configured for the active profile, it checks your Terraform configuration for a matching token. Refer to [Terraform tokens](#terraform-tokens) for more information.

### Set organization

Run the `tfctl profile set default_organization` command to set the organization. Replace `<name>` with your HCP Terraform or Terraform Enterprise organization name.

```bash
$ tfctl profile set default_organization <name>
```

## Example usage

**Command Syntax:** `tfctl <command> [subcommand] [flags] [arguments]`

```bash
# Scenario: Check the status of two workspaces across two instances of HCP Terraform. Start a run on a workspace.

# Configure profiles for EU and US organizations, and authenticate to both. You must already have users with access to the organizations in each HCP Terraform instance.
$ tfctl profile profiles create us --hostname=app.terraform.io
$ tfctl profile set default_organization my-us-org-name
$ tfctl auth login # Create API token in web browser, copy it, then paste it to terminal

$ tfctl profile profiles create eu --hostname=app.eu.terraform.io
$ tfctl profile set default_organization my-eu-org-name
$ tfctl auth login # Create API token in web browser, copy it, then paste it to terminal

# Get workspace configuration from EU org. Workspace must already exist in this org.
$ tfctl api /organizations/{organization}/workspaces/example-app-workspace

# Get status of last workspace run
$ tfctl run status example-app-workspace

# Switch to US profile
$ tfctl profile profiles activate us

# Start a run on the US workspace. Workspace must already exist in this org.
$ tfctl run start example-app-workspace --message="Run started with tfctl."

# Get detailed run status in JSON format
$ tfctl run status example-app-workspace --json
```

### Manage profiles

Use the `tfctl profile profiles` command group to create and manage profiles. You can use a different profile for each HCP Terraform organization and each instance of HCP Terraform and instance of Terraform Enterprise.

To create a profile, run the `tfctl profile profiles create` command and specify a name to create a profile. Replace `<name>` with a name for your new profile.

```bash
$ tfctl profile profiles create <name>
```

The CLI activates the new profile automatically.

### Run `tfctl` in Docker

If you have a `TFCTL_TOKEN` environment variable set, you can invoke `tfctl` using Docker. Refer to [Environment variables](#environment-variables) for more configuration environment variables.

For example, the following command will run `tfctl` in a docker container, passing the local `TFCTL_TOKEN` environment variable into the container:

```bash
$ docker run -e TFCTL_TOKEN hashicorp/tfctl
```

## Configuration reference

The `tfctl` CLI stores its configuration in the following directories, depending on your operating system:

- Linux/MacOS: `~/.config/tfctl`
- Windows: `%AppData%/tfctl`

The CLI stores configuration for individual profiles in the `profiles` subdirectory. For example, `profiles/default.hcl` stores the configuration for your default profile.

### Terraform tokens

If you have not configured a token for the active profile with `tfctl auth login`, the `tfctl` CLI checks your Terraform configuration for a matching token. These tokens may either be in your Terraform configuration directory, for example `~/.terraform.d/credentials.tfrc.json`, or the corresponding Terraform environment variables, such as `TF_TOKEN_app_terraform_io`.

### Environment variables

If you have not configured a particular option for the active profile, `tfctl` checks the following environment variables:

`TFCTL_ORGANIZATION`: The organization to use for commands that require an organization.

`TFCTL_HOSTNAME`: The Terraform Enterprise or HCP Terraform hostname to use. Defaults to `app.terraform.io`.

`TFCTL_TOKEN`: An HCP Terraform API token to use in conjunction with the default profile.

`TFCTL_TOKEN_<profile>`: An HCP Terraform API token to use in conjunction with the named profile.

`TF_TOKEN_<hostname>`: An HCP Terraform API token to present during authentication, with the specified hostname in Punycode formatting, for example `TF_TOKEN_app_terraform_io`. The CLI present the Terraform token only if it has not been configured in any other way.

## Command reference

![tfctl](assets/tfctl.png "tfctl")

The `tfctl` command can manage HCP Terraform runs and variables with the corresponding subcommands. It also provides direct access to the HCP Terraform API with the `api` subcommand.

Use the `--help` flag to print out detailed usage instructions. For example, `tfctl --help` to print out help for the tfctl CLI, and `tfctl run --help` for help with the `run` subcommand.

### Global flags

`tfctl` supports the following global flags.

- `--debug`: Enable debug output.
  - Data type: Boolean flag
  - Defaults to `false`.

- `--dry-run`: Creates a preview of the proposed changes.
  - Data type: Boolean flag
  - Defaults to `false`.

- `--json`: Sets the output format to JSON.
  - Data type: Boolean flag
  - Defaults to `false`.

- `--jq=<expression>`: A jq filter expression to apply to JSON output. Sets `--json` to `true`.
  - Data type: String
  - Optional parameter.

- `--markdown`: Sets the output format to Markdown.
  - Data type: Boolean flag
  - Defaults to `false`.

- `--no-color`: Disables color output.
  - Data type: Boolean flag
  - Defaults to `false`.

- `--profile=<name>`: The profile to use. If omitted, the CLI uses the current profile.
  - Data type: String
  - Optional parameter.

- `--quiet`: Minimizes output and disables interactive prompting.
  - Data type: Boolean flag
  - Defaults to `false`.

- `--version`: Print the version of `tfctl` CLI.
  - Data type: Boolean flag
  - Defaults to `false`.

### `tfctl auth login` reference

**Usage:** `tfctl auth login [options]`

#### Description

Authenticate the `tfctl` CLI with HCP Terraform or Terraform Enterprise. Opens a browser to the token creation page for the configured hostname and prompts you to paste the generated token.

#### Examples

Login interactively:

```bash
$ tfctl auth login
```

Login with a token from stdin. Replace `abcd.atlasv1.1234` with a valid HCP Terraform or Terraform Enterprise token.

```bash
$ echo "abcd.atlasv1.1234" | tfctl auth login --token
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

Supported agents are:

- `bob`
- `claude`
- `codex`
- `copilot`
- `gemini`
- `opencode`
- `pi`

#### Examples

Install `tfctl` skills for Bob in the current project directory.

```bash
$ tfctl harness install bob
```

Install `tfctl` skills for Claude in your user directory for use with all projects.

```bash
$ tfctl harness install --global claude
```

Print out agent context for `tfctl`, suitable for AGENTS.md.

```bash
$ tfctl harness context
```

### `tfctl api` reference

**Usage:** `tfctl api PATH [options]`

#### Description

Perform any HCP Terraform API v2 request with the given path or URL.

The HCP Terraform API typically requires a resource ID as part of the path for resource-specific requests. To support this, the `tfctl` CLI interpolates parameter values in the `<path>` argument denoted by `{NAME}`. Whenever possible, `tfctl` infers these values from the path, the active profile, or local Terraform configuration. You can provide values for named parameters with the `--pathparam` argument.

#### Arguments

- `PATH`: API path relative to configured host, or URL.
  - Required argument

#### Options

- `--all`: Fetch all records in multiple pages, up to 2000, and combine them into one page.

- `--attribute`, `-a`: Set attribute in request body. Sets `--method` to `POST`.
  - Repeatable
  - Format as `<name>=<value>`

- `--field`, `-f`: Add a query parameter to the request URL.
  - Repeatable
  - Format as `<name>=<value>`

- `--header`, `-H`: Add an HTTP header to the request.
  - Repeatable
  - Format as `<name>=<value>`

- `--input`, `-i`: Specify the entire JSON request body.
  - Repeatable
  - Format as JSON:API data envelope, or `-` to read from stdin.

- `--method`, `-X`: HTTP method to use (GET, POST, etc)

- `--page-number`, `-n`: Page number to return. Ignored if `--all` is set.
  - Default: 1

- `--page-size`, `-s`: Limit the number of records to return. Ignored if `--all` is set.
  - Default: Varies by resource

- `--pathparam`, `-p`: Provide a hint for path parameter resolution.
  - Repeatable
  - Format as `<name>=<value>`
  - Useful for resolving unique resource names, such as project, to an ID that can be used in the PATH.
  - Example: `tfctl api /projects/{name} -p 'name=Default Project'`

- `--type`, `-t`: Resource type for --attribute JSON:API request bodies.
    - This value is inferred from the path whenever possible.

### `tfctl api schema search` reference

**Usage:** `tfctl api schema search QUERY`

#### Description

Search all platform operations that can be invoked using `tfctl api`

#### Arguments

- `QUERY`: Any phrase
  - Required argument

#### Example

```bash
$ tfctl api schema search 'run tasks'
```

### `tfctl api schema get` reference

**Usage:** `tfctl api schema get OPERATION`

#### Description

Get the OpenAPI specification for any single operation, including any schema components in use.

#### Arguments

- `OPERATION`: The operation ID of any search result
  - Required argument

#### Example

```bash
$ tfctl api schema get getTask
```

### `tfctl get` reference

**Usage:** `tfctl get ID`

#### Description

Get or lists a resource. Acts as a simplified shortcut to using `tfctl api`.

#### Arguments

- `ID`: A flexible string argument that can be:
  - A resource ID, like `proj-9arLMbi8daoo3hhG` or `ws-sRMALNbEeg1aT93F`
  - A resource type prefix, like `proj`, `ws`, or `varset`
  - A resource type name, like `projects`, `organizations`, or `agent-pools`

### `tfctl create` reference

**Usage:** `tfctl create TYPE`

#### Description

Creates a resource. Acts as a simplified shortcut to using `tfctl api`.

#### Arguments

- `TYPE`: A resource type name to create, like `projects`, `organizations`, or `agent-pools`

#### Flags

`--attribute`, `-a`: Attribute for the JSON:API request body
  - Repeatable
  - Format as `<name>=<value>`

- `--input`, `-i`: Specify the entire JSON request body.
  - Repeatable
  - Format as JSON:API data envelope, or `-` to read from stdin.

- `--organization`, `-o`: Organization name
  - Default: Profile `default_organization` or terraform cloud config context

### `tfctl profile display` reference

**Usage:** `tfctl profile display [options]`

#### Description

Print out configuration for a profile.

#### Examples

Print out information about the active profile:

```bash
$ tfctl profile display
```

Print out information about a profile named `my-profile`:

```bash
$ tfctl profile display --profile="my-profile"
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile get` reference

**Usage:** `tfctl profile get PROPERTY [options]`

#### Description

Get the value of the given configuration property for a profile.

#### Arguments

- `PROPERTY`: The configuration property name to retrieve.
  - Required argument
  - Data type: String

#### Examples

Get the organization for the active profile:

```bash
$ tfctl profile get organization
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile set` reference

**Usage:** `tfctl profile set PROPERTY VALUE [options]`

#### Description

Set the value of the given configuration property for a profile.

#### Arguments

- `PROPERTY`: The configuration property name to set.
  - Required argument
  - Data type: String

- `VALUE`: The value to set for the property.
  - Required argument
  - Data type: String

#### Examples

Set the organization to `my-organization` for the active profile:

```bash
$ tfctl profile set default_organization my-organization
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile unset` reference

**Usage:** `tfctl profile unset PROPERTY [options]`

#### Description

Unset the value of the given configuration property for a profile.

#### Arguments

- `PROPERTY`: The configuration property name to unset.
  - Required argument
  - Data type: String

#### Examples

Unset the organization for the active profile:

```bash
$ tfctl profile unset default_organization
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

Switch to a profile named `my-profile`:

```bash
$ tfctl profile profiles activate my-profile
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
  - Default: `false`

- `--hostname`: Set the hostname for the new profile.
  - Optional
  - Data type: String

#### Examples

Create and switch to a new profile named `my-profile` configured for a Terraform Enterprise instance hosted at `my-tfe-instance.example.com`:

```bash
$ tfctl profile profiles create my-profile --hostname="my-tfe-instance.example.com"
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles list` reference

**Usage:** `tfctl profile profiles list [options]`

#### Description

List existing profiles.

#### Examples

List all profiles in JSON format:

```bash
$ tfctl profile profiles list --json
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles delete` reference

**Usage:** `tfctl profile profiles delete NAME [<name> ...] [options]`

#### Description

Delete an existing named profile.

#### Arguments

- `NAME`: One or more profile names to delete.
  - Required argument
  - Data type: String
  - Repeatable

#### Examples

Delete a profile named `old-profile`.

```bash
$ tfctl profile profiles delete old-profile
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl profile profiles rename` reference

**Usage:** `tfctl profile profiles rename NAME --new_name=<new_name> [options]`

#### Description

Rename an existing named profile.

#### Arguments

- `NAME`: The current profile name.
  - Required argument
  - Data type: String

- `--new_name`: Set the new profile name.
  - Required
  - Data type: String

#### Examples

Rename a profile named `old-name` to `new-name`:

```bash
$ tfctl profile profiles rename old-name --new_name="new-name"
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl run start` reference

**Usage:** `tfctl run start ID_OR_NAME [options]`

#### Description

Start a new run on the workspace specified by ID or name.

#### Arguments

- `ID_OR_NAME`: Workspace ID or name.
  - Required argument
  - Data type: String

#### Flags

- `--allow-empty-apply`: Allow the run to proceed even if the plan has no changes.
  - Optional
  - Data type: Boolean flag
  - Default: `false`

- `--debugging-mode`: Enables trace logging for this run by setting TF_LOG=trace in the Terraform environment for this run.
  - Optional
  - Data type: Boolean flag
  - Default: `false`

- `--message`: Attach a message to the run.
  - Optional
  - Data type: String

- `--organization`: Organization name. This value overrides the organization name configured for the profile.
  - Optional
  - Data type: String

#### Examples

Start a new run on an existing workspace named `my-workspace`:

```bash
$ tfctl run start my-workspace
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl run status` reference

**Usage:** `tfctl run status ID [options]`

#### Description

Print out the status of a run. You can specify a run ID, workspace ID, or workspace name for the `ID` argument.

- Run ID: Gets the specific run. Run IDs are prefixed with `run-`.
- Workspace ID: Gets the latest run on the workspace. Workspace IDs are prefixed with `ws-`. 
- Workspace name: Gets the most recent run on the named workspace.

#### Arguments

- `<id>`: Run ID, workspace ID, or workspace name.
  - Required argument
  - Data type: String

#### Flags

- `--organization`: Organization name.
  - Optional
  - Data type: String
  - Default: Defaults to the profile organization or HCP Terraform configuration context.

#### Examples

Print out the status of a run with an ID of `run-1234abcd`:

```bash
$ tfctl run status run-1234abcd
```

Print out the status of the latest run on a workspace named `my-workspace`:

```bash
$ tfctl run status my-workspace
```

Print out the status of the latest run on a workspace with an ID of `ws-abc123xyz`:

```bash
$ tfctl run status ws-abc123xyz
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### `tfctl variable import` reference

**Usage:** `tfctl variable import [TFVARS_FILE] [options]`

#### Description

Import Terraform variables from .tfvars files or environment variables from the `tfctl` process environment into a workspace or variable set.

You must provide either a variable set or a workspace by name. Otherwise, `tfctl` scans the current working directory for Terraform configuration to attempt to determine the workspace name.

#### Arguments

- `<tfvars_file>`: The .tfvars file to import variables from. You can specify the path to the file if it's in another directory. The CLI automatically configures variables whose names indicate they may be sensitive as `sensitive`, such as those containing `secret`, `token`, or `private`.
  - Optional argument
  - Data type: String

#### Options

- `--env`, `-e`: Environment variable to import. `tfctl` configures all imported environment variables as sensitive.
  - Repeatable
  - Data type: String

- `--organization`: Organization name.
  - Optional
  - Data type: String
  - Default: Defaults to the profile configuration or HCP Terraform configuration context.

- `--overwrite`: Update existing variables to match the imported variables. Otherwise, the CLI reports an error when imported variables don't match existing variables.
  - Optional
  - Data type: Boolean flag
  - Default: `false`

- `--variable-set-name`: Target variable set by name.
  - Optional
  - Data type: String
  - Default: If omitted, the CLI sets workspace variables

- `--workspace`: Workspace name override.
  - Optional
  - Data type: String
  - Default: Defaults to HCP Terraform or Terraform Enterprise configuration context

#### Examples

Import Terraform variables from `terraform.tfvars` file to a workspace named `my-workspace`:

```bash
$ tfctl variable import terraform.tfvars --workspace="my-workspace"
```

Import multiple environment variables into a variable set named `my-varset` and overwrite existing values:

```bash
$ tfctl variable import --overwrite --variable-set-name="my-varset" --env="AWS_ACCESS_KEY_ID" --env="AWS_SECRET_ACCESS_KEY"
```

#### Related

- [Configuration reference](#configuration-reference)
- [Global flags](#global-flags)

### Exit Codes

| Exit | Meaning                          | Solution                              |
|------|----------------------------------|---------------------------------------|
| 0    | OK                               | &mdash;                               |
| 1    | Usage error                      | Read `tfctl <cmd> --help`             |
| 2    | Not Found or Authorization Error | Verify URL/ID                         |
| 3    | Authentication Error             | `tfctl auth login`                    |
| 4    | Network error                    | Check connectivity                    |
| 5    | API Server Error Persists        | Try again later                       |
| 6    | Underlying error detected        | Varies depending on error condition   |
| 130  | Canceled (ctrl-c).               | &mdash;                               |


### Telemetry

By default, HashiCorp collects anonymous trace telemetry for each command invocation, including the command name, exit status, network metrics, and basic process information. You can disable telemetry using any of these methods: Set `TFCTL_TELEMETRY` to `disabled`, Set `DO_NOT_TRACK` to `true`, or set telemetry to disabled in your profile: 

```bash
$ tfctl profile set telemetry disabled
```

You can view the telemetry that we transmit by setting profile telemetry to `log`:

```bash
$ tfctl profile set telemetry log
```

### Uninstall/Clean up

**Uninstall Shell Completions**

```bash
$ tfctl --autocomplete-uninstall
```

## Developers

### Installing from Source

#### Prerequisites

- go v1.26.4 or later
- git
- make

#### Install `tfctl`

1. Clone the git [repository](https://github.com/hashicorp/tfctl-cli):
   - SSH: `git clone git@github.com:hashicorp/tfctl-cli.git`
   - HTTPS: `git clone https://github.com/hashicorp/tfctl-cli.git`
1. Change to the new directory: `cd tfctl-cli`
1. Run `make go/install`.

Verify the installation:

```bash
$ tfctl --version
```

### Preparing your Change

#### PR Preparation Checklist

1. Ensure you have [changie](https://changie.dev/guide/installation/) installed for release notes prep.
1. Ensure any command changes are sensitive to these global flags:
  - `--json` &mdash; Force machine readable output to stdout. Does not apply to stderr.
  - `--markdown` &mdash; Force markdown output to stdout. Does not apply to stderr.
  - `--dry-run` &mdash; Don't make any actual writes or other mutations. Describe what would have changed to stderr.
  - `--quiet` &mdash; Don't render output to stdout.
1. Get the logging interface from the context and add debug logging for interesting conditions and nonfatal situations.
1. Run `make gen/screenshot` if the root command output changes.
1. Add the `Autocomplete` field to positional arguments and flags to assist shell autocomplete.
1. Run `changie new` to prepare a new changelog entry for the next set of release notes.
