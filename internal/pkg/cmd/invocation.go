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

// Invocation passes global objects for constructing and invoking a command.
type Invocation struct {
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
	// helpers exported in the Invocation.
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
func (i *Invocation) GetGlobalFlags() GlobalFlags {
	if !i.flags.parsed {
		panic("This is a programmer error. Only access global flags from within a run command. Otherwise flags haven't been parsed yet.")
	}

	return i.flags
}

// IsDryRun returns true when commands should avoid making mutating changes.
func (i *Invocation) IsDryRun() bool {
	return i.GetGlobalFlags().dryRun
}

// ResolveLogLevel returns the resolved verbosity level, with the --debug
// flag taking precedence over the profile setting.
func (i *Invocation) ResolveLogLevel() hclog.Level {
	if !i.flags.parsed {
		return hclog.Error
	}

	switch {
	case i.GetGlobalFlags().debug >= 2:
		return hclog.Trace
	case i.GetGlobalFlags().debug == 1:
		return hclog.Debug
	default:
		return hclog.Error
	}
}

// ConfigureRootCommand should be only called on the root command. It configures
// global flags and ensures that the invocation is configured based on any flags
// set during a command invocation.
func ConfigureRootCommand(i *Invocation, cmd *Command) {
	// Store the IO on the command, making it available to the entire tree.
	cmd.io = i.IO

	cmd.Flags.Persistent = append(cmd.Flags.Persistent, &Flag{
		Name:         "profile",
		DisplayValue: "NAME",
		Description:  "The profile to use. If omitted, the currently selected profile will be used.",
		Value:        flagvalue.Simple("", &i.flags.profile),
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
		Value:        flagvalue.Simple("", &i.flags.jq),
		global:       true,
	}, &Flag{
		Name:          "json",
		Description:   "Sets the output format.",
		Value:         flagvalue.Simple(false, &i.flags.json),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "markdown",
		Description:   "Sets the output format to markdown.",
		Value:         flagvalue.Simple(false, &i.flags.markdown),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "dry-run",
		Description:   "Shows what would happen without actually changing anything.",
		Value:         flagvalue.Simple(false, &i.flags.dryRun),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "quiet",
		Description:   "Minimizes output and disables interactive prompting.",
		Value:         flagvalue.Simple(false, &i.flags.Quiet),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "no-color",
		Description:   "Disables color output.",
		Value:         flagvalue.Simple(false, &i.flags.noColor),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "debug",
		Description:   "Enable debug output.",
		Value:         flagvalue.Counter(0, &i.flags.debug),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "version",
		Description:   fmt.Sprintf("Print the version of %s CLI.", version.Name),
		Value:         flagvalue.Simple(false, &i.flags.Version),
		IsBooleanFlag: true,
		global:        true,
	})

	// Setup the pre-run command
	cmd.PersistentPreRun = func(c *Command, args []string) error {
		if err := i.applyGlobalFlags(c); err != nil {
			return err
		}

		c.io = i.IO
		logger := logging.FromContext(i.ShutdownCtx)
		logger.SetLevel(i.ResolveLogLevel())
		logger.Debug("Log level set", "level", logger.GetLevel())

		// Replace the context with a context containing a newly named logger for this command.
		commandName := strings.TrimPrefix(c.commandPath(), fmt.Sprintf("%s ", version.Name))
		i.ShutdownCtx = logging.WithLogger(i.ShutdownCtx, logger.Named(commandName))

		tel := telemetry.FromContext(i.ShutdownCtx)

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

			logger.Debug("Error", "error", firstLine)
		})

		// Start the telemetry span now that we know the command and flags.
		if tel != nil {
			i.ShutdownCtx = tel.StartCommand(i.ShutdownCtx, telemetry.CommandInfo{
				Command: c.CommandPath(),
				Profile: i.Profile,
				Debug:   i.flags.debug > 0,
				JSON:    i.flags.json || i.flags.jq != "",
				DryRun:  i.flags.dryRun,
			})
		}

		err := isAuthenticated(i, c, args)
		if err != nil {
			return err
		}

		return nil
	}
}

// applyGlobalFlags applies the global flags.
func (i *Invocation) applyGlobalFlags(_ *Command) error {
	// Mark that we have parsed flags
	i.flags.parsed = true

	// Parse the profile first
	if p := i.flags.profile; p != "" {
		l, err := profile.NewLoader()
		if err != nil {
			return err
		}

		p, err := l.LoadProfile(i.ShutdownCtx, i.flags.profile)
		if err != nil {
			return err
		}

		*i.Profile = *p
	}

	// Set the output format if the flag is set.
	f := format.Unset
	if i.flags.json {
		f = format.JSON
	}
	if i.flags.markdown {
		if f == format.Unset {
			f = format.Markdown
		} else {
			return fmt.Errorf("cannot set multiple output formats")
		}
	}

	// --jq implies --json and is only compatible with --json
	if i.flags.jq != "" {
		if f != format.Unset && f != format.JSON {
			return fmt.Errorf("--jq cannot be used with --markdown; only --json output is supported")
		}
		if f == format.Unset {
			f = format.JSON
		}
		i.Output.SetJQFilter(i.flags.jq)
	}

	if f != format.Unset {
		i.Output.SetFormat(f)
	}

	// Disable color if set
	if i.flags.noColor || (i.Profile != nil && i.Profile.NoColor != nil && *i.Profile.NoColor) {
		i.IO.ForceNoColor()
	}

	// Set quiet on the IOStream if enabled by the flag
	if i.flags.Quiet {
		i.IO.SetQuiet(true)
	}

	return nil
}

// NewAPIClient returns a new API Client configured using the invocation Profile.
// When debug output is enabled and a non-nil logger is provided, the client's
// HTTP transport is wrapped to log requests and responses.
func (i *Invocation) NewAPIClient() (*client.Client, error) {
	address := i.Profile.GetHostname()
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "https://" + address
	}
	apiClient, err := client.New(i.ShutdownCtx, address, i.Profile.GetToken(), http.Header{
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
func (i *Invocation) ParseFlags(c *Command, args []string) ([]string, error) {
	if err := c.parseFlags(args); err != nil {
		return nil, err
	}

	if err := i.applyGlobalFlags(c); err != nil {
		return nil, err
	}

	return c.allCommandFlags.Args(), nil
}

func isAuthenticated(i *Invocation, c *Command, args []string) error {
	logger := logging.FromContext(i.ShutdownCtx)

	if isTopLevelCmd(args) || c.NoAuthRequired {
		return nil
	}

	if i.Profile.GetToken() == "" {
		return authHelp(c.io)
	}

	if i.Profile.Token == "" {
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
