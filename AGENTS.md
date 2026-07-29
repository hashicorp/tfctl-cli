# tfctl CLI

`tfctl` is a Go CLI for HCP Terraform and Terraform Enterprise. It provides raw API access, high-level workflows, and commands for humans and coding agents.

## Planning and Communication

- Use ASD-STE100 Simplified Technical English in plans and documentation.
- Make the smallest correct change. Follow the established package boundaries and command patterns.
- Treat the rules in this file as requirements for new and substantially changed code. The known deviations below are technical debt, not examples to copy.

## Development Setup

- Use Go 1.26.4, git, bash, and make.
- Run `scripts/setup.sh` to install development tools and the `tfctl` binary. The script does not install Go.
- Run `make bin` to build the binary.
- Run `make check` for formatting checks, lint, and standard tests. This target does not run the race detector.

## Repository Architecture

- `cmd/tfctl/main.go` is the process entry point. It creates I/O, logging, profiles, telemetry, the shared invocation, and the command tree.
- `internal/commands/` contains command behavior. The top-level groups are `api`, `get`, `create`, `run`, `auth`, `variable`, `profile`, and `harness`.
- `internal/pkg/` contains reusable infrastructure. Important packages include `cmd`, `client`, `format`, `iostreams`, `logging`, `telemetry`, `profile`, `openapi`, and `execsession`.
- `internal/commands/*` can depend on `internal/pkg/*`. Do not add dependencies from infrastructure packages to command packages.
- `skills/` contains embedded coding-agent skills.

The CLI uses a custom command model in `internal/pkg/cmd` and adapts it to `github.com/hashicorp/cli`. `cmd.Invocation` carries shared I/O, output, profile, shutdown context, and parsed global state to command constructors.

For runnable leaf commands, persistent pre-run applies global flags, configures the context logger, starts a telemetry span, and checks authentication. Group help, flag parse errors, and required-argument errors can return before persistent pre-run.

## Command Design

- Use TDD.
- Keep command declaration and flag wiring in `NewCmdXxx`.
- Put command behavior in a private `runXxx` function. Pass an `XxxOpts` value that contains only the required dependencies and values.
- Do not pass `*cmd.Invocation` to `runXxx`. Resolve invocation state and construct clients in command wiring, then pass explicit dependencies in the options value.
- Test `runXxx` directly by varying its options. Add `Command.Run` tests when flag parsing, argument validation, autocomplete, or exit behavior needs coverage.
- Group commands with no `RunF` do not need an options value or behavior function.
- Keep shared behavior private unless another command package has a concrete need to call it.

## Global Flags

Command changes must account for these global flags:

- `--dry-run` must prevent remote mutations and command-specific state changes. Report the skipped action to stderr.
- `--quiet` suppresses `IOStreams.ErrUnessential()` and disables prompts. It does not automatically suppress stdout or `IOStreams.Err()`.
- `--no-color` disables command-facing color and styling.
- `--debug` controls the context logger level.
- `--profile` replaces the active profile for the invocation.

## Mutation Safety

- Check dry-run state before every write, mutation request, browser launch, child process, or other command-specific side effect.
- In dry-run mode, do not mutate shared in-memory values as a substitute for avoiding a persisted write. Use a copy when validation needs a proposed value.
- Render dry-run details to `IOStreams.Err()` with `ColorScheme.DryRunLabel()`.
- Do not rely on `client.Resolver.dryRun` to block creation. The field is not enforced. Callers must set `createIfNotFound` to false or guard the mutation before calling the resolver.
- Keep destructive API operations behind the existing confirmation and exec-session checks. Harness exec-session permission applies only to selected API deletes and is not a general mutation permission.

## Output and Diagnostics

- Send structured stdout through `format.Outputter` with a `format.Displayer`.
- A displayer must provide a default format, a payload, and field templates that work with forced JSON and Markdown output. The outputter also supports pretty and table output. `format.Agent` currently renders as JSON.
- Use direct stdout only for an intentional raw byte stream or child-process pass-through. Document why global format conversion does not apply.
- Never include credentials or sensitive values in a displayer payload. JSON output serializes the full payload, not only the displayed field templates.
- Use `IOStreams.Err()` for essential diagnostics that must remain visible with `--quiet`.
- Use `IOStreams.ErrUnessential()` for progress, guidance, and routine success messages that `--quiet` can suppress.
- Use `IOStreams.ColorScheme()` for command-facing stderr styling. Logging has separate hclog color handling.
- A command that must suppress stdout in quiet mode must implement that behavior explicitly.

## Logging and Telemetry

- Get the logger with `logging.FromContext(ctx)`.
- Add debug logs for useful decisions, fallback behavior, ignored nonfatal errors, and external operations.
- Do not log tokens, credentials, sensitive variable values, or request bodies that can contain secrets.
- Pass the command context to API and other blocking calls so cancellation and telemetry propagate.
- Telemetry command spans exist only for runnable commands that reach persistent pre-run. Do not assume that help and parse-error paths have a command span.

## Arguments, Flags, and Help

- Set `Command.Args.Autocomplete` for positional-argument completion. `PositionalArgument` does not have an `Autocomplete` field.
- Set `Flag.Autocomplete` for flags that accept values. Use an appropriate `complete.Predictor`.
- If autocomplete would be incorrect, omit it and add a short comment. `harness exec` is an example because its trailing arguments belong to another executable.
- Add examples and clear help for user-facing behavior.
- Run `make gen/screenshot` when root command output changes.

## Profiles and Configuration

- Profiles are HCL files under the `profiles/` configuration directory. The configuration root also contains `active_profile.hcl`, `device_id`, and host caches.
- `Profile.Predict` completes profile property names. Profile-name completion uses `Loader.ListProfiles`.
- Hostname helpers default, normalize, and validate hostnames. They do not classify HCP Terraform and Terraform Enterprise.
- Selected commands can use local Terraform configuration as an organization or workspace fallback.
- `auth login --token` reads a token from stdin. It does not accept the token as the flag value.

## Testing and Release Checks

- Write a failing test before the implementation change.
- Run a focused test with `go test ./... -run '<TestFunc>'`.
- Run lint with `golangci-lint run`.
- Run regression and race tests with `go test ./... -race`.
- Use `cmdtest.NewServer` for routed HTTP test servers and `cmdtest.WriteJSONAPI` when a handler needs a JSON:API response.
- Format tests use inline expected output rather than golden files.
- Run `changie new` to prepare a changelog entry for a user-visible change.

## Known Architecture Deviations

Do not reproduce these patterns in new code. Fix a deviation when it is in the direct scope of the change.

- `internal/commands/profile/set.go` decodes proposed values directly into the shared profile before the dry-run check.
- `client.Resolver` stores `dryRun` but does not read it. Creation safety depends on each caller.
- Some ignored nonfatal errors have no debug log. For example, `auth status` suppresses token-expiration lookup failures.
- Telemetry shutdown runs after normal CLI dispatch, but early returns such as the root banner path bypass it.
