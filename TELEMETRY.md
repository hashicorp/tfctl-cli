# Telemetry

By default, HashiCorp collects anonymous trace telemetry for each command invocation, including the command name, exit status, network metrics, and basic process information. You can disable telemetry using any of these methods: Set `TFCTL_TELEMETRY` to `disabled`, Set `DO_NOT_TRACK` to `true`, or set telemetry to disabled in your profile: 

```bash
$ tfctl profile set telemetry disabled
```

You can view the telemetry that we transmit by setting profile telemetry to `log`:

```bash
$ tfctl profile set telemetry log
```

## Telemetric Schema

HashiCorp does not collect or transmit your data and takes steps to redact and obscure data that could contain your data. The following schema reflects exactly which telemetric data is collected.

### Resource Schema (Common to all spans)
| Key             | Example Value  | Source                                    |
|-----------------|----------------|-------------------------------------------|
| device_id       | (string uuid)  | Randomly generated once per install       |
| service.name    | "tfctl"        | Constant                                  |
| service.version | "v0.3.0"       | Build version                             |
| session_id      | (SHA-256 hash) | Hash of the parent process ID + device_id |

### Command Span Schema (One span per execution)

| Key              | Example Value       | Source                                                               |
|------------------|---------------------|----------------------------------------------------------------------|
| command          | "api schema search" | Which subcommand was invoked (not the full command arguments)        |
| dry_run_flag     | false               | Global flag state                                                    |
| debug_flag       | false               | Global flag state                                                    |
| json_flag        | false               | Global flag state                                                    |
| os               | “darwin”            | GOOS value                                                           |
| arch             | “arm64”             | GOARCH value                                                         |
| is_ci            | false               | “CI” env detection                                                   |
| is_tty           | false               | Terminal environment detection                                       |
| is_named_profile | false               | Config detection- default profile was loaded, or some named profile? |
| hostname         | (SHA-256 hash)      | Hashed hostname from config                                          |
| exit_status      | 0                   | Process exit code                                                    |
| agent            | "claude"            | Process environment detection (CLAUDECODE == “1” ?)                  |

### Network Request Span (One span per network request)

| Key                 | Example Value            | Source                              |
|---------------------|--------------------------|-------------------------------------|
| http.path           | “/workspaces/<redacted>” | API request path, with IDs redacted |
| http.method         | "GET"                    | API request verb                    |
| http.status_code    | 200                      | HTTP response status                |
| http.content_length | 121070                   | Response byte size                  |
| http.duration_ms    | 855                      | Request latency                     |

