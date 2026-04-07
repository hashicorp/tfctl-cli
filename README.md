## tfcloud: The HCP Terraform CLI

Effectively interact with the HCP Terraform platform.

#### Quick Start

tfcloud uses a host-centric, layered configuration with a logical precedence. Configuration commands
do not yet exist in the CLI, so start by writing this file to `$HOME/.config/tfcloud/tfcloud.hcl`
(or `%AppData%/tfcloud/tfcloud.hcl` on Windows) substituting your own hostname, token, and organization.

```hcl
profile "default" "app.terraform.io" {
  token        = "your-token"
  organization = "user-org"
}
```

```
# Migrate a tfvars file to the current workspace
tfcloud variable import bigsecret.tfvars

# Migrate a tfvars file to a new variable set
tfcloud variable import bigsecret.tfvars -variable-set-name "production"

# Migrate ENV variables available to the current workspace
tfcloud variable import -e AWS_REGION -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY

# Execute any API v2 GET query
tfcloud api /account/details # Table format
tfcloud api /organizations -json # JSON format

# Execute any POST query by specifying -a for request body attributes in key=value format or -i for raw request body input
tfcloud api /organizations/acme/projects -a "name=my-project" -a "description=it\'s a very fine project"

# ...or use a JSON input file as the body
tfcloud api /organizations/acme/projects -input my-project.json

# ...or use stdin as the request body
./generate_hcptf_run.sh | tfcloud api /runs -input -

# If using parameters in a GET request, set the method to GET.
# This example fetches all pages of data (up to 1000 items) and sorts by created-at descending
tfcloud api /organizations/acme/workspaces -paginate -method GET -f "sort=-created-at"
```

#### Configuration Reference

**Profile-level Configuration**

Linux/MacOS: `~/.config/tfcloud/tfcloud.hcl`
Windows: `%AppData%/tfcloud/tfcloud.hcl`

**Working Directory Configuration**

Working directory config overwrites profile-level config, when available.

`.tfcloud.hcl`

**Token created by `terraform login`**

`~/.terraform.d/credentials.tfrc.json` is checked for the configured hostname if the token is not set by configuration file.

**Token in Environment Variables**

`TFCLOUD_TOKEN`: An API token to use in conjunction with the default profile, only used if token is not set by any other configuration file.

`TFCLOUD_TOKEN_<profile>`: Reserved for future use with multiple profiles.

`TF_TOKEN_<hostname>`: An API token to use with the specified hostname with punycode formatting, e.g. `TF_TOKEN_app_terraform_io`, only used if the token is not specified in any other way.


#### Usage

You can use `tfcloud -help` for detailed usage instructions.

**`tfcloud api <path> [flags]`**

Perform an API request.

`-H, -header <key:value>`
  Add a HTTP request header in key:value format

`-i, -input <file>`
  The file to use as body for the HTTP request (use "-" to read from standard input)

`-X, -method <string>`
  The HTTP method for the request (default "GET", unless using -a attributes)

`-t, -type <string>`
  When used with a JSON:API request body for POST/PATCH, the resource type (default to the resource implied by the path)

`-paginate`
  Make additional HTTP requests to fetch all pages of results but emit in a streamable manner

`-a, -attribute <key=value>`
  Add a typed resource attribute to the request body in key=value format

`-f, -field <key=value>`
  Add a query string parameter to the request URL in key=value format

`-agent`
  Print the raw response body.

`-json`
  Print the raw response body, colorized, if a terminal is attached.

`-v, -verbose`
  Log HTTP request and response details to stderr

**`tfcloud variable import [tfvars-file] [flags]`**

Import variables from a tfvars file or the process environment into the current workspace or a variable set.

`-e <name>`
  Import an environment variable by name. Repeat to import multiple values.

`-variable-set-name <name>`
  Target a variable set by name instead of the current workspace.

`-organization <string>`
  Organization name. Optional when it can be resolved from the default organization in `tfcloud.hcl` or local Terraform configuration.

`-workspace <string>`
  Override the target workspace name.

`-overwrite`
  Update matching existing variables instead of failing when duplicates are found.
