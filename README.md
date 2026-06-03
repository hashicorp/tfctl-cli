# tfctl: The HCP Terraform CLI

Comprehensive, official CLI access to the HCP Terraform / Terraform Enterprise platform.

![tfctl](assets/tfctl.png "tfctl")

## Installation

### Prerequisites

- The Go language, vFIXME or later.

### Install tfctl

1. Clone the git [repository](https://github.com/hashicorp/tfctl-cli): `git clone git@github.com:hashicorp/tfctl-cli.git`
1. Change to the new directory: `cd tfctl-cli`
1. Run `make go/install`.

Binary releases available soon!

### Install shell completion

Shell completion assists with command, argument, and API path completion and is highly recommended.

```bash
$ tfctl --autocomplete-install
```

You can uninstall shell completion with the `tfctl --autocomplete-uninstall` command.

### Install AI agent skill

tfctl ships with an agent skill that gives AI coding agents full access to HCP Terraform. You can install it using tfctl. Replace AGENT with the name of your supported AI agent: `bob`, `claude`, `codex`, `copilot`, `gemini`, `opencode`, or `pi`.

```bash
$ tfctl harness install AGENT --global
```

You can instead install tfctl skills with NPX.

```bash
$ npx skills add hashicorp/tfctl-cli --skill 'tfctl'
```

This adds the skill to your user profile so that compatible agents can use tfctl on your behalf.

## Configure tfctl

tfctl uses a host-centric, layered configuration with a logical precedence.

### Set hostname

tfctl defaults to the HCP Terraform instance at app.terraform.io. To use a different HCP Terraform instance or your organizations Terraform Enterprise instance, configure it now. Replace HOST with your HCP Terraform hostname (`app.terraform.io` or `app.eu.terraform.io`), or Terraform Enterprise hostname.

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

### Manage configuration profiles

tfctl supports multiple local configuration profiles, accessible via the `tfctl profile profiles` subcommand. Use profiles to switch between HCP Terraform organizations and instances of HCP Terraform and Terraform Enterprise. To start using profiles, create one.

```bash
$ tfctl profile profiles create NAME
```

tfctl will activate the new profile automatically.

## Example sage

tfctl provides access to runs, variables, and other HCP Terraform features through named subcommends. The tfctl also provides direct access to the [HCP Terraform API](https://developer.hashicorp.com/terraform/cloud-docs/api-docs) with the `tfctl api` subcommand.

```bash
# See status/diagnose Workspace current run
tfctl run status my-workspace

# Migrate a tfvars file to the current workspace
tfctl variable import bigsecret.tfvars

# Migrate a tfvars file to a new variable set
tfctl variable import bigsecret.tfvars --variable-set-name "production"

# Migrate ENV variables available to the current workspace
tfctl variable import -e AWS_REGION -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY

# Execute any API v2 GET query
tfctl api /account/details # Table format
tfctl api /organizations --json # JSON format

# Execute any POST query by specifying -a for request body attributes in key=value format or -i for raw request body input
tfctl api /organizations/acme/projects -a "name=my-project" -a "description=it\'s a very fine project"

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

`TFCTL_ORGANIZATION`: The default organization to use, where one might apply.

`TFCTL_HOSTNAME`: The Terraform Enterprise or HCP Terraform hostname to use. Defaults to `app.terraform.io`.

`TFCTL_TOKEN`: An HCP Terraform API token to use in conjunction with the default profile.

`TFCTL_TOKEN_<profile>`: An HCP Terraform API token to use in conjunction with the named profile.

`TF_TOKEN_<hostname>`: An HCP Terraform API token to use with the specified hostname with punycode formatting, e.g. `TF_TOKEN_app_terraform_io`. tfctl will use the Terraform token only if it has not bee configured in any other way.

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

### `tfctl auth` reference

Authenticate with HCP Terraform or Terraform Enterprise, or check authentication token status.

#### Subcommands

- `login`: Create and save an authentication token to login to HCP Terraform or Terraform Enterprise.
- `status`: Check the status of the currently configured authentication token.

#### Example usage

Create an authentication token for the configured host, either HCP Terraform or your Terraform Enterprise instance.

```bash
$ tfctl auth login
```

Check the status of the token for the configured host, including expiration date if available.

```bash
$ tfctl auth status
```

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

### `tfctl profile` reference

Configure and manage tfctl user profiles. Profiles allow you to use tfctl to manage multiple organizations and instances of HCP Terraform or Terraform Enterprise.

#### Subcommands

- `display`: Print out configuration for the active profile.
- `get PROPERTY`: Get the value of the given configuration property for the active profile.
- `set PROPERTY`: Set the value of the given configuration property for the active profile.
- `unset PROPERTY`: Unset the value of the given configuration property for the active profile.

#### Command groups

- [`profiles`](#tfctl-profile-profiles-reference): Manage profiles

#### Example usage

Display configuration for the active profile.

```bash
$ tfctl profile display
```

Set the organization to "my-organization" for the active profile.

```bash
$ tfctl profile set organization my-organization
```

### `tfctl profile profiles` reference

#### Description

Manage tfctl user profiles.

#### Subcommands

- `activate NAME`: Activate an existing named profile.
- `create NAME`: Create a new configuration profile, and activate it.
  - `--no-activate`: Don't automatically activate the new profile.
  - `--hostname=HOST`: Set the hostname for the new profile.
- `delete NAME`: Delete an existing named profile.
- `list`: List existing configuration profiles.
- `rename NAME`: Rename an existing named profile.
  - `--new_name=NAME`: Set the profile name. Required.

#### Example usage

Create and switch to a new profile.

```bash
$ tfctl profile profiles create NAME --hostname=HOST
```

List configured profiles.

```bash
$ tfctl profile profiles list
```

Switch to a profile by name.

```bash
$ tfctl profile profiles activate NAME
```

### `tfctl run` reference

#### Description

Print out the status of Terraform runs, or start a new run.

#### Subcommands

- `start WORKSPACE`: Start a new run on the workspace specified by ID or name.
  - `--allow-empty-apply`: Allow the run to proceed even if the plan has no changes.
  - `--debugging-mode`: Enables trace logging for this run by setting TF_LOG=trace in the terraform environment for this run.
  - `--message MESSAGE`: Attach a message to the run.
  - `--organization NAME`: Organization name, overrides profile's configured organization name.
- `status ID`: Print out the status of a run by run ID, or the latest run on a workspace by workspace ID or name.
  - `--organization NAME`: Organization name, overrides profile's configured organization name.

#### Example usage

Start a new run on an existing workspace named "my-workspace".

```bash
$ tfctl run start my-workspace
```

Print out the status of a run with an ID of "run-1234abcd".

```bash
$ tfctl run status run-1234abcd
```

Print out the status of the latest run on an existing workspace named "my-workspace".

```bash
$ tfctl run status my-workspace
```

### `tfctl variable` reference

#### Description

Manage Terraform and environment variables for workspaces and variable sets.

#### Subcommands

- `import`: Start a new run on the workspace specified by ID or name. Will error if the variable already exists in the target variable set or workspacem unless --overwrite is set.
  - `TFVARS_FILE`: Import Terraform variables from a .tfvars file. Optional
  - `--env=NAME`, `-e NAME`: Import environment variable by name. Repeatable.
  - `--overwrite`: Overwrite existing variable instead of erroring.
  - `--variable-set-name=NAME`: Import variables to variable set by name.
  - `--workspace=NAME`: Import variables to workspace by name.

#### Example usage

Import Terraform variables from "terraform.tfvars" file to a workspace named "my-workspace".

```bash
$ tfctl import terraform.tfvars --worksapce=my-workspace
```

Import multiple environment variables into a variable set named "my-varset", overwriting existing values if any.

```bash
$ tfctl import --overwrite --variable-set-name=my-varset --env=AWS_ACCESS_KEY_ID --env=AWS_SECRET_ACCESS_KEY
```

### `tfctl api` reference

Perform any HCP Terraform API v2 request with the given path or URL, or inspect the API schema.

#### Arguments

- `PATH`: API path relative to configured host, or URL. 
- `--all`: Disable pagination and fetch all records, to a maxiumum of 2000.
- `--attribute=NAME=VALUE`, `-a NAME=VALUE`: Set attribute in request body. Implies `--method=POST`. Repeatable.
- `--field=KEY=VALUE`, `-f KEY=VALUE`: Set query parameter in request URL. Repeatable.
- `--header='name: value'`, `-H 'name: value'`: Set request header. Repeatable
- `--input=BODY`, `-i BODY`: Set raw JSON request body, use `-` to read from stdin.
- `--method=METHOD`, `-X METHOD`: HTTP method to use (e.g. GET, POST, etc.)
- `--page-number=NUMBER`: Page number to return. Ignored if --all is set. Default is 1.
- `--page-size=SIZE`: Limit the number of records to return. Default varies by resource. Ignored if --all is set.
- `--pathparam=NAME=VALUE`, `-p NAME=VALUE`: Provide a hint for path parameter resolution. Repeatable.
- `--type=JSON:API TYPE`, `-t JSON:API TYPE`: Resource type for --attribute JSON:API request bodies. 

The HCP Terraform API typically requires a resource ID as part of the path for resource-specific requests. To support this, tfctl interpolates parameter values in the `PATH` argument denoted by `{NAME}`. Whenever possible, tfctl will infer these values from the path, the active profile, or local Terraform configuration. You can provide values for named parameters with the `--pathparam` argument.

#### Command groups

- [`schema`](#tfctl-api-schema-reference): Inspect API schema.

#### Example usage

Print out details about the account associated with the configured token in JSON format.

```bash
$ tfctl api /account/details --json
```

Create a project named "my-project" for the currently configured organization.

```bash
$ tfctl api /organizations/{organization}/projects --attribute="name=my-project" --attribute="description=A very fine project indeed."
```

Print a list of all the workspaces (up to a limit of 2000) for the currently configured organization, sorted by the time the last run was started.

```
$ tfctl api /organizations/{organization}/workspaces --all --field="sort=-current-run.created-at"
```

### `tfctl api schema` reference

Search for API operations from the OpenAPI spec and inspect a single operation schema.

#### Subcommands

- `get`: Show a trimmed OpenAPI document for a single operationId or all operations on an exact API path.
  - `OPERATION_ID_OR_PATH`: An exact OpenAPI operationId or an API path (starting with /) to inspect.
- `search`: Search API operations by keywords.
  - `QUERY`: The search query to match against API operations. Repeatable.

#### Example usage

Inspect the getWorkspace operation.

```bash
$ tfctl api schema get getWorkspace
```

Print out all operations available for workspaces.

```bash 
$ tfctl api schema get /organizations/{organization}/workspaces
```

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
