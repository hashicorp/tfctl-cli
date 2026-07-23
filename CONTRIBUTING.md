# tfctl CLI — Contributing

# Part 1: How to Contribute

## 1. Development Setup

**Prerequisites**: Go 1.26, git, bash, make

```bash
git clone https://github.com/hashicorp/tfctl-cli
cd tfctl-cli
scripts/setup.sh      # Install toolchain and dev tools
make bin              # Build binary for your os/arch
make check            # Verify everything passes before creating PR
```

## 2. Essential Requirements

- [ ] Run `changie new` to prepare a new changelog entry for the next set of release notes.
- [ ] Ensure any command changes are sensitive to these global flags:
  - `--json` &mdash; Force machine readable output to stdout. Does not apply to stderr.
  - `--markdown` &mdash; Force markdown output to stdout. Does not apply to stderr.
  - `--dry-run` &mdash; Don't make any actual writes or other mutations. Describe what would have changed to stderr.
  - `--quiet` &mdash; Only render essential content.
- [ ] Get the logging interface from the context and add debug logging for interesting conditions and nonfatal situations.
- [ ] Run `make gen/screenshot` if the root command output changes.
- [ ] Add the `Autocomplete` field to **positional arguments** and **flags** to assist shell autocomplete.

## 3. Questions?

Open an issue for questions about contributing.

# Part 2: Architecture

`tfctl` is a Go command-line tool for interacting with **HCP Terraform** and
**Terraform Enterprise (TFE)**. It provides three tiers of interaction:

1. A **low-level API helper** (`tfctl api`) for raw HTTP calls against the
   Terraform API.
2. **High-level commands** (runs, variables, workspaces, auth, profiles) that
   wrap common workflows.
3. **Interactive / agent-oriented commands** (`tfctl harness`, prompt-based
   auth) designed to support coding agents and humans at a terminal.

- **Module:** `github.com/hashicorp/tfctl-cli`
- **Language / toolchain:** Go 1.26
- **CLI framework:** `github.com/hashicorp/cli` (not cobra/urfave), wrapped by a
  custom command abstraction.

---

## 1. Design Principles

These principles (codified in `AGENTS.md`) shape the architecture:

- **Testability first (TDD).** Every command separates wiring (`NewCmdXxx`) from
  behavior (`runXxx`), so behavior is tested by varying an options struct rather
  than driving the whole CLI.
- **Uniform output.** All stdout rendering goes through a single `Displayer` /
  `Outputter` system that supports `--json`, `--markdown`, and default pretty
  output (plus table and agent formats).
- **Consistent styling.** All stderr/human styling goes through `ColorScheme`.
- **Safe by default.** A global `--dry-run` flag must suppress all mutations.
- **Observable.** Commands emit structured debug output through a
  context-carried logger, and OpenTelemetry traces command/HTTP activity.

---

## 2. High-Level Structure

```
cmd/tfctl/            Binary entry point (main.go only)
internal/
  commands/           Command implementations (one package per group)
    root/             Root command wiring
    api/  auth/  run/  variable/  create/  get/
    profile/          Profile management (+ profiles/ subcommands)
    harness/          Coding-agent skills, context, session-scoped exec
    cmdtest/ cmdutil/ Shared test + command helpers
  pkg/                Reusable infrastructure
    cmd/              Home-grown Command / Invocation framework + hashicorp/cli bridge
    client/           HCP Terraform / TFE API client (wraps go-tfe/v2)
    format/           Displayer / Outputter rendering (json/markdown/pretty/table/agent)
    iostreams/        Terminal I/O abstraction + ColorScheme
    logging/          hclog-based logging carried on context
    telemetry/        OpenTelemetry tracing
    profile/          HCL-backed profile + host cache config
    openapi/          Embedded OpenAPI spec loading
    execsession/      Session-scoped permission store (agent safety rail)
    terraform/        Reads local Terraform Cloud config
    flagvalue/ heredoc/ table/ ld/ git/ resource/   Utilities
version/              Name + Version constants
skills/               Embedded coding-agent skills
scripts/              Developer setup scripts (setup.sh)
Makefile              Build/test/lint/tooling targets (make help)
```

**Layering rule:** `internal/commands/*` (what the CLI *does*) depends on
`internal/pkg/*` (the *infrastructure*). Cross-cutting concerns — I/O, color,
logging, telemetry, profile config — are threaded through a single `Invocation`
value and the shutdown `context.Context`.

---

## 3. Lifecycle & Bootstrap

The entry point is `cmd/tfctl/main.go` (`realMain`), which wires the process in
a fixed order:

1. **Signal handling.** A root context is created with
   `context.WithCancelCause`; a goroutine cancels it on `SIGINT`/`SIGTERM` so
   long-running commands can abort cleanly.
2. **I/O.** `iostreams.System(ctx)` builds the terminal abstraction (stdin/out/
   err, TTY detection, color capability). Console state is restored on exit.
3. **Logging.** An initial log level is derived by scanning args for `--debug`,
   a logger is created, and it is stored **on the context**
   (`logging.WithLogger`) so every layer can retrieve it.
4. **Profile.** `profile.NewLoader()` loads the active profile, writing sane
   defaults on first run (`loadActiveProfile`). Profile `no_color` is honored
   here.
5. **Telemetry.** `telemetry.Init` starts OpenTelemetry (device ID, hostname,
   version) and is stored on the context.
6. **Invocation.** A `cmd.Invocation` is assembled bundling `IO`, `Profile`,
   `Output: format.New(io)`, and `ShutdownCtx`.
7. **Command tree.** `root.NewCmdRoot(inv)` builds the command tree;
   `cmd.ToCommandMap` flattens it into the `map[string]cli.CommandFactory` that
   `hashicorp/cli` expects.
8. **Run.** `cli.CLI.Run()` dispatches to the matched command; afterward
   telemetry is shut down with the resulting exit status.

```
main → realMain
  ├─ context + signal handling
  ├─ iostreams.System
  ├─ logging.NewLogger → WithLogger(ctx)
  ├─ profile.Loader → active profile
  ├─ telemetry.Init → WithTelemetry(ctx)
  ├─ cmd.Invocation{IO, Profile, Output, ShutdownCtx}
  ├─ root.NewCmdRoot(inv) → cmd.ToCommandMap
  └─ cli.CLI.Run() → exit status
```

---

## 4. The Command Framework

Rather than use `hashicorp/cli` directly, the project defines its own richer
command model in `internal/pkg/cmd/`.

### `Command` (`command.go`)

A declarative struct describing a single command or a nesting group:

- Identity/help: `Name`, `Aliases`, `ShortHelp`/`LongHelp`, `Examples`.
- Arguments: `Args` (`PositionalArguments`).
- Flags: `Flags.Local` + `Flags.Persistent` (persistent flags flow to
  descendants).
- Hooks: `PersistentPreRun`, and the command body `RunF`.
- Behavior flags: `Hidden`, `NoAuthRequired`.
- Tree building: `AddChild` composes groups; a group without a `RunF` acts as a
  container.

Control flow uses **sentinel errors** — `ErrDisplayHelp`, `ErrDisplayUsage`,
`ErrUnderlyingError` — and `ExitCodeError`/`NewExitError` to set specific exit
codes (e.g. auth failure → 3, not-found → 2).

### `Invocation` (`invocation.go`)

The shared context object passed into every `NewCmdXxx` constructor. It holds
`IO`, `Output`, `Profile`, `ShutdownCtx`, and parsed `GlobalFlags`. Key
responsibilities:

- **`ConfigureRootCommand`** installs the global flags (`--profile`, `--json`,
  `--markdown`, `--jq`, `--dry-run`, `--quiet`, `--no-color`, `--debug`,
  `--version`) and registers the persistent pre-run hook.
- **`applyGlobalFlags`** (run in pre-run) reconciles flags: reloads the profile
  if `--profile` was passed, resolves the output format (`--json`/`--markdown`/
  `--jq` with conflict checks), disables color, and sets quiet mode.
- **`NewAPIClient`** constructs the API client from the active profile's
  hostname + token, injecting a `User-Agent` header.
- **`ResolveLogLevel` / `IsDryRun`** expose global state to commands.

### `hashicorp/cli` Bridge (`compat.go`)

`ToCommandMap` recursively flattens the `Command` tree into path-keyed factories
(e.g. `"run status"`), expanding aliases. `CompatibleCommand` implements the
`cli.Command`, `cli.CommandAutocomplete`, and `cli.CommandHelpTemplate`
interfaces, adapting the custom model to the framework.

### Pre-Run Pipeline

On every command, the persistent pre-run hook (in `ConfigureRootCommand`):

1. Applies global flags.
2. Sets the log level and creates a per-command **named logger** on the context.
3. Starts a telemetry span describing the command.
4. Enforces authentication (`isAuthenticated`) unless the command is top-level
   or marked `NoAuthRequired`.

---

## 5. The `runXxx` / `XxxOpts` Command Pattern

Every command follows the same two-part shape, which is the backbone of the
project's testing strategy:

```
NewCmdRunStart(inv) *cmd.Command   // wiring: declares flags, args, help
   └─ RunF: build StartOpts from parsed flags + invocation
             → runStart(ctx, startOpts, runOpts)   // behavior
```

- **`XxxOpts`** carries exactly what the behavior needs — typically `IO`,
  `Output`, an API client, `Profile`, `DryRun`, and command-specific fields —
  **not** the whole command context.
- **`runXxx(ctx, opts)`** contains all real logic and is unit-tested directly by
  varying the opts.
- Test seams (e.g. injectable `Store`, `PID`, `Run` in `harness exec`) are
  passed via opts so behavior can be exercised deterministically.

Examples: `run/run_start.go` (`StartOpts`/`CreateOpts` → `runStart`),
`run/run_status.go` (`StatusOpts` → `runStatus`), `auth/login.go` (`LoginOpts` →
`loginRun`), `api/api.go` (`Opts`), `harness/harness_exec.go` (`ExecOpts`).

---

## 6. Output Rendering (`internal/pkg/format`)

All stdout output is unified through the `Displayer` / `Outputter` system, so a
single command definition produces pretty, table, JSON, markdown, or agent
output.

- **`Format`** enum: `Unset`, `Pretty`, `Table`, `JSON`, `Markdown`, `Agent`.
- **`Displayer` interface:** `DefaultFormat()`, `Payload() any`,
  `FieldTemplates() []Field`. A `Field` pairs a name with a `text/template`
  value expression (e.g. `{{ .CloudProvider }}/{{ .Region }}`).
- **`Outputter`** (`format.New(io)`, stored on `Invocation.Output`):
  `Display(d)` selects the format (the displayer's default unless a global flag
  forces one) and dispatches to `outputPretty` / `outputTable` / `outputJSON` /
  `outputMarkdown` / `outputAgent`. `--jq` runs a `gojq` filter over JSON.
- **Extension interfaces:** `TemplatedPayload` (alternate payload for templated
  output) and `StringPayload` (pre-formatted pretty/markdown, e.g. the run
  status summary emits ANSI for pretty and markdown syntax for `--markdown`).
- **Helpers:** `NewDisplayer[T]` and `DisplayFields`/`Show` infer fields from a
  struct via reflection for simple cases.

This is the mechanism that satisfies the "one displayer, three formats" rule in
`AGENTS.md`.

---

## 7. Terminal I/O and Styling

### `IOStreams` (`internal/pkg/iostreams`)

Abstracts stdin/stdout/stderr, TTY detection, quiet mode, and color capability.
Provides interactive primitives: `PromptConfirm`, `ReadSecret` (no-echo),
`CanPrompt`. Tests use `iostreams.Test()` to capture buffers.

### `ColorScheme` (`colorscheme.go`)

Used for **stderr / human-facing** styling (per `AGENTS.md`). Built on
`muesli/termenv` with automatic degradation to terminal capability:

- Chainable strings: `.Bold()`, `.Italic()`, `.Underline()`, `.Faint()`,
  `.Color()`, `.CodeBlock(ext)`, etc.
- HashiCorp-branded named colors and `RGB(hex)`.
- Semantic helpers: `SuccessIcon()`, `FailureIcon()`, `WarningLabel()`,
  `DryRunLabel()`, `ErrorLabel()`.
- A markdown mode emits markdown syntax instead of ANSI from the same API.

Color is disabled by `--no-color`, profile `no_color`, or a non-TTY stream.

---

## 8. API Client Layer (`internal/pkg/client`)

Wraps **`github.com/hashicorp/go-tfe/v2`**, which is built on **Microsoft
Kiota**-generated clients against the Terraform OpenAPI spec.

- **`Client`** wraps `*tfe.Client`, the Kiota request adapter, `BaseURL`, and
  default headers. `client.New(ctx, address, token, headers)` configures retry
  policy (server errors, rate limiting, max 5 retries, retry-hook logging).
- **Typed calls** use the generated fluent API, e.g.
  `client.TFE.API.Workspaces().ByWorkspace_id(id).Get(ctx, nil)`.
- **Raw calls** (`Client.Do`) build a Kiota `RequestInformation`, convert to a
  native `*http.Request`, and send it — this powers `tfctl api`. `ResolveURL`
  handles relative/absolute paths while preserving encoded slashes.
- **`Resolver`** (`resolver.go`) resolves resources by name (workspace, variable
  set, current run, etc.), honoring `createIfNotFound` and `dryRun`.
- **Transports:** HTTP is wrapped by a `loggingTransport` (debug request/
  response logging with timing) and a `telemetryTransport` (OTel spans with
  status/size/duration), installed via `SetLogger`/`SetTelemetry`.

---

## 9. Configuration & Profiles (`internal/pkg/profile`)

Configuration is **HCL-backed** (`hashicorp/hcl/v2`).

- A `Profile` holds `Name`, `Hostname`, `Token`, `DefaultOrganization`,
  `NoColor`, `Telemetry`.
- On-disk: per-profile `<name>.hcl`, an `active_profile.hcl` pointer, a
  `device_id` (telemetry UUID), and a per-host cache.
- `Loader` provides `GetActiveProfile`, `LoadProfile`, `ListProfiles`,
  `DefaultProfile`, `GetDeviceID`, and autocomplete prediction.
- Hostname helpers default to `app.terraform.io`, normalize via IDNA, and detect
  HCP Terraform vs. TFE.
- The global `--profile` flag overrides the active profile at runtime.
- Local Terraform Cloud config is read via `terraform.FindCloudConfig` to
  default the organization.

---

## 10. Authentication (`internal/commands/auth`)

- Tokens live on the profile (`Profile.Token`) or are resolved from the
  environment / Terraform credentials via `Profile.GetToken()`.
- `tfctl auth login` (marked `NoAuthRequired`) reads a token from `--token`,
  stdin, or interactively (opens the browser to the token settings page and
  reads a secret with no echo), verifies it against the account endpoint, then
  persists it — respecting `--dry-run`.
- Enforcement happens in the pre-run pipeline: non-top-level commands without
  `NoAuthRequired` require a token, otherwise a helpful "run `tfctl auth login`"
  error is returned. `ErrUnauthorized` maps to exit code 3, `ErrNotFound` to 2.

---

## 11. Logging (`internal/pkg/logging`)

- Built on `github.com/hashicorp/go-hclog`, writing to stderr with timestamps;
  color only on a color-capable TTY.
- The logger is **carried on the context** (`WithLogger`/`FromContext`),
  returning a null logger when absent.
- Verbosity is controlled by the counting `--debug` flag:
  `>=2 → Trace`, `1 → Debug`, else `Error`.
- The pre-run hook names the logger per command path so log lines are
  attributable; the API transports log at debug. Commands should use
  `logging.FromContext(ctx)` for debug output (per `AGENTS.md`).

---

## 12. Telemetry (`internal/pkg/telemetry`)

OpenTelemetry tracing (`go.opentelemetry.io/otel`, OTLP + stdout exporters).
Initialized at startup and carried on the context. The pre-run hook opens a span
per command capturing command path, profile, and flag state; the HTTP transport
adds per-request spans. Telemetry can be disabled per profile and is shut down
with the final exit status. Errors are intentionally non-fatal.

---

## 13. Interactive & Agent Support

There is no full-screen TUI; interactivity is prompt-based and agent-oriented:

- **Prompts:** `PromptConfirm` / `ReadSecret` guard interactive auth and
  destructive `api` deletes.
- **`tfctl harness`:** installs coding-agent skills (`harness_install.go`) and
  prints agent context (`harness_context.go`).
- **Session-scoped permissions** (`internal/pkg/execsession` +
  `harness exec`): `tfctl harness exec --allow-delete=… -- <child>` grants a
  temporary, auto-reverting permission so nested `tfctl` invocations can perform
  noninteractive mutations — a safety rail for automated agents.

---

## 14. Testing Strategy

- **TDD** is mandated. Commands are tested by calling `runXxx` directly with
  constructed `XxxOpts` (no full CLI dispatch), asserting on captured I/O.
- **HTTP harness** (`internal/commands/cmdtest`): `RouteMap` maps method+path to
  handlers, `NewServer` spins up an `httptest.Server` returning JSON:API
  responses, and `NewInvocation` builds a test `Invocation` pointed at it.
- **Table-driven subtests** with `t.Parallel()`; assertions via
  `stretchr/testify`.
- **Golden-style tests** in `format/` verify pretty/markdown/json/table output.

Commands:

```
go test ./... -run "<TestFunc>"    # focused test
go test ./... -race                # regression / race check
golangci-lint run                  # lint
```

---

## 15. Key Dependencies

| Area                 | Library                                                  |
|----------------------|----------------------------------------------------------|
| CLI framework        | `github.com/hashicorp/cli`                               |
| Flags / autocomplete | `spf13/pflag`, `posener/complete`                        |
| Terminal styling     | `muesli/termenv`, `muesli/reflow`                        |
| Terraform API        | `hashicorp/go-tfe/v2` (Kiota-generated)                  |
| Kiota runtime        | `microsoft/kiota-abstractions-go` (+ http/serialization) |
| OpenAPI              | `getkin/kin-openapi`, `blugelabs/bluge` (schema search)  |
| Config               | `hashicorp/hcl/v2`, `zclconf/go-cty`                     |
| JSON query           | `itchyny/gojq` (`--jq`)                                  |
| Logging              | `hashicorp/go-hclog`                                     |
| Telemetry            | `go.opentelemetry.io/otel` (+ OTLP/stdout exporters)     |
| Browser              | `cli/browser` (login)                                    |
| Testing              | `stretchr/testify`                                       |

---

## 16. Request Flow Summary

A typical mutating command (e.g. `tfctl run start`) flows as:

```
CLI args
  → hashicorp/cli dispatch (ToCommandMap)
  → Command.PersistentPreRun
       applyGlobalFlags · set log level · start telemetry span · auth check
  → Command.RunF
       build StartOpts (IO, Output, APIClient, Profile, DryRun)
       → runStart(ctx, opts)
            Resolver resolves workspace/run
            client.TFE.API... performs API call (skipped/echoed under --dry-run)
            Output.Display(displayer) → pretty | json | markdown | table
            ColorScheme-styled status to stderr
  → exit status → telemetry.Shutdown
```

This consistent path — global flags, context-carried logger/telemetry, an
`Invocation`-built client, an `XxxOpts` → `runXxx` split, and unified `Displayer`
output — is repeated across every command, which is what makes the CLI uniform
and heavily testable.
