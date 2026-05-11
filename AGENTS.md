# tfctl CLI

A CLI for interacting with HCP Terraform and Terraform Enterprise in several ways, featuring a low-level API helper, high level commands, and interactive commands.

## Contributing
**MUST follow these rules:**

- Use TDD.
- Respect the global --dry-run flag - when dry-run is enabled, don't change any data or execute any mutations.
- For stdout rendering, create a displayer that can render --json, --markdown, and default (pretty) output.
- Use ColorScheme formatting for stderr rendering.
- When authoring a new command, pass an XXXOpts type value to a private runXXX function. Don't share the entire command context. Test functions should test the behavior of the runXXX function by varying the options passed to it.
- Use the command Logger() to produce appropriate debug output.

## Testing instructions
- Test: `go test ./... -run "<MyTestFunc>"`
- Lint: `golangci-lint run`
- Test for regressions: `go test ./... -race`
