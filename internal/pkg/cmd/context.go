// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/internal/pkg/telemetry"
	"github.com/hashicorp/tfctl-cli/version"
)

// Context passes global objects for constructing and invoking a command.
type Context struct {
	// IO is used to interact directly with IO or the terminal.
	IO iostreams.IOStreams

	// Output is used to print structured output.
	Output *format.Outputter

	// ShutdownCtx is a context that is canceled if the user requests the
	// command to be shutdown. If a command can block for an extended amount of
	// time, the context should be used to exit early.
	ShutdownCtx context.Context

	// flags stores our global flags. Access must go through GetGlobalFlags()
	// which ensures flags are only accessed after the flags have been parsed
	// from the arguments.
	flags GlobalFlags

	Profile *profile.Profile
}

// GlobalFlags contains the global flags.
type GlobalFlags struct {
	// parsed stores if the flags have been parsed yet
	parsed bool

	// Unexported global flags. These should generally be access via other
	// helpers exported in the Context.
	profile  string
	json     bool
	markdown bool
	noColor  bool
	jq       string
	debug    int
	dryRun   bool

	// Version indicates the user has requested the version of the CLI
	Version bool

	// Quiet indicates the user has requested minimal output
	Quiet bool
}

// GetGlobalFlags returns the global flags. It panics if the flags have not been
// parsed yet, which should only be the case if they are accessed outside of a run command.
func (ctx *Context) GetGlobalFlags() GlobalFlags {
	if !ctx.flags.parsed {
		panic("This is a programmer error. Only access global flags from within a run command. Otherwise flags haven't been parsed yet.")
	}

	return ctx.flags
}

// IsDryRun returns true when commands should avoid making mutating changes.
func (ctx *Context) IsDryRun() bool {
	return ctx.GetGlobalFlags().dryRun
}

// ResolveLogLevel returns the resolved verbosity level, with the --debug
// flag taking precedence over the profile setting.
func (ctx *Context) ResolveLogLevel() hclog.Level {
	if !ctx.flags.parsed {
		return hclog.Warn
	}

	switch {
	case ctx.GetGlobalFlags().debug >= 2:
		return hclog.Trace
	case ctx.GetGlobalFlags().debug == 1:
		return hclog.Debug
	default:
		return hclog.LevelFromString(ctx.Profile.GetVerbosity())
	}
}

// ConfigureRootCommand should be only called on the root command. It configures
// global flags and ensures that the context is configured based on any flags
// set during a command invocation.
func ConfigureRootCommand(ctx *Context, cmd *Command) {
	// Store the IO on the command, making it available to the entire tree.
	cmd.io = ctx.IO

	cmd.Flags.Persistent = append(cmd.Flags.Persistent, &Flag{
		Name:         "profile",
		DisplayValue: "NAME",
		Description:  "The profile to use. If omitted, the currently selected profile will be used.",
		Value:        flagvalue.Simple("", &ctx.flags.profile),
		global:       true,
		Autocomplete: complete.PredictFunc(func(_ complete.Args) []string {
			l, err := profile.NewLoader()
			if err != nil {
				return nil
			}

			profiles, err := l.ListProfiles()
			if err != nil {
				return nil
			}

			return profiles
		}),
	}, &Flag{
		Name:         "jq",
		DisplayValue: "EXPRESSION",
		Description:  "A jq filter expression to apply to JSON output. Implies --json.",
		Value:        flagvalue.Simple("", &ctx.flags.jq),
		global:       true,
	}, &Flag{
		Name:          "json",
		Description:   "Sets the output format.",
		Value:         flagvalue.Simple(false, &ctx.flags.json),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "markdown",
		Description:   "Sets the output format to markdown.",
		Value:         flagvalue.Simple(false, &ctx.flags.markdown),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "dry-run",
		Description:   "Shows what would happen without actually changing anything.",
		Value:         flagvalue.Simple(false, &ctx.flags.dryRun),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "quiet",
		Description:   "Minimizes output and disables interactive prompting.",
		Value:         flagvalue.Simple(false, &ctx.flags.Quiet),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "no-color",
		Description:   "Disables color output.",
		Value:         flagvalue.Simple(false, &ctx.flags.noColor),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "debug",
		Description:   "Enable debug output.",
		Value:         flagvalue.Counter(0, &ctx.flags.debug),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "version",
		Description:   fmt.Sprintf("Print the version of %s CLI.", version.Name),
		Value:         flagvalue.Simple(false, &ctx.flags.Version),
		IsBooleanFlag: true,
		global:        true,
	})

	// Setup the pre-run command
	cmd.PersistentPreRun = func(c *Command, args []string) error {
		if err := ctx.applyGlobalFlags(c); err != nil {
			return err
		}

		c.io = ctx.IO
		logger := logging.FromContext(ctx.ShutdownCtx)
		logger.SetLevel(ctx.ResolveLogLevel())
		logger.Debug("Log level set", "level", logger.GetLevel())

		ctx.ShutdownCtx = logging.WithLogger(ctx.ShutdownCtx, logger.ResetNamed(c.commandPath()))
		tel := telemetry.FromContext(ctx.ShutdownCtx)

		logger.Debug("Got telemetry context", "telemetry nil", tel == nil)

		telemetry.SetErrorHandler(func(err error) {
			logger = logger.ResetNamed(version.Name).Named("telemetry")

			errorReader := strings.NewReader(err.Error())
			scanner := bufio.NewScanner(errorReader)
			scanner.Split(bufio.ScanLines)

			firstLine := ""
			additionalLines := 0
			for scanner.Scan() {
				if firstLine == "" {
					firstLine = scanner.Text()
				} else {
					additionalLines++
				}
			}

			if additionalLines > 0 {
				firstLine = fmt.Sprintf("%s (and %d more lines)", firstLine, additionalLines)
			}

			logger.Debug("Telemetry Error", "error", firstLine)
		})

		// Start the telemetry span now that we know the command and flags.
		if tel != nil {
			ctx.ShutdownCtx = tel.StartCommand(ctx.ShutdownCtx, telemetry.CommandInfo{
				Command: c.CommandPath(),
				Profile: ctx.Profile,
				Debug:   ctx.flags.debug > 0 || ctx.Profile.GetVerbosity() == "debug",
				JSON:    ctx.flags.json || ctx.flags.jq != "",
				DryRun:  ctx.flags.dryRun,
			})
		}

		err := isAuthenticated(ctx, c, args)
		if err != nil {
			return err
		}

		return nil
	}
}

// applyGlobalFlags applies the global flags.
func (ctx *Context) applyGlobalFlags(_ *Command) error {
	// Mark that we have parsed flags
	ctx.flags.parsed = true

	// Parse the profile first
	if p := ctx.flags.profile; p != "" {
		l, err := profile.NewLoader()
		if err != nil {
			return err
		}

		p, err := l.LoadProfile(ctx.flags.profile)
		if err != nil {
			return err
		}

		*ctx.Profile = *p
	}

	// Set the output format if the flag is set.
	f := format.Unset
	if ctx.flags.json {
		f = format.JSON
	}
	if ctx.flags.markdown {
		if f == format.Unset {
			f = format.Markdown
		} else {
			return fmt.Errorf("cannot set multiple output formats")
		}
	}

	// --jq implies --json and is only compatible with --json
	if ctx.flags.jq != "" {
		if f != format.Unset && f != format.JSON {
			return fmt.Errorf("--jq cannot be used with --markdown; only --json output is supported")
		}
		if f == format.Unset {
			f = format.JSON
		}
		ctx.Output.SetJQFilter(ctx.flags.jq)
	}

	if f != format.Unset {
		ctx.Output.SetFormat(f)
	}

	// Disable color if set
	if ctx.flags.noColor || (ctx.Profile != nil && ctx.Profile.NoColor != nil && *ctx.Profile.NoColor) {
		ctx.IO.ForceNoColor()
	}

	// Set quiet on the IOStream if enabled by the flag or profile
	if ctx.flags.Quiet || ctx.Profile.IsQuiet() {
		ctx.IO.SetQuiet(true)
	}

	return nil
}

// NewAPIClient returns a new API Client configured using the context Profile.
// When debug output is enabled and a non-nil logger is provided, the client's
// HTTP transport is wrapped to log requests and responses.
func (ctx *Context) NewAPIClient() (*client.Client, error) {
	address := ctx.Profile.GetHostname()
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "https://" + address
	}
	apiClient, err := client.New(ctx.ShutdownCtx, address, ctx.Profile.GetToken(), http.Header{
		"User-Agent": []string{fmt.Sprintf("%s-cli/%s", version.Name, version.Version)},
	})
	if err != nil {
		return nil, err
	}

	return apiClient, nil
}

// ParseFlags can be used to parse the flags for a given command before it is
// run. This can be helpful in very specific cases such as accessing flags
// during autocompletion. The return args are the non-flag arguments.
func (ctx *Context) ParseFlags(c *Command, args []string) ([]string, error) {
	if err := c.parseFlags(args); err != nil {
		return nil, err
	}

	if err := ctx.applyGlobalFlags(c); err != nil {
		return nil, err
	}

	return c.allCommandFlags.Args(), nil
}

func isAuthenticated(ctx *Context, c *Command, args []string) error {
	logger := logging.FromContext(ctx.ShutdownCtx)

	if isTopLevelCmd(args) || c.NoAuthRequired {
		return nil
	}

	if ctx.Profile.GetToken() == "" {
		return authHelp(c.io)
	}

	if ctx.Profile.Token == "" {
		logger.Debug("Token missing from profile; using token configured by environment")
	}

	return nil
}

func authHelp(io iostreams.IOStreams) error {
	cs := io.ColorScheme()
	help := heredoc.Docf(`
No authentication detected. To get started with %s CLI, please run: %s`,
		version.Name,
		cs.String(fmt.Sprintf("%s auth login", version.Name)).Bold().String())

	return errors.New(help)
}

// Used to parse commands and skip loading profile.
func isTopLevelCmd(args []string) bool {
	if len(args) != 1 {
		return false
	}

	switch args[0] {
	case "version":
		return true
	case "-v":
		return true
	case "--version":
		return true
	case "-version":
		return true
	case "-h":
		return true
	case "--help":
		return true
	}
	return false
}
