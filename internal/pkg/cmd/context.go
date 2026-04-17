// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/posener/complete"

	"github.com/hashicorp/tfcloud/internal/config"
	"github.com/hashicorp/tfcloud/internal/pkg/client"
	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
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
	agent    bool
	markdown bool
	noColor  bool
	debug    int

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
		Name:          "json",
		Description:   "Sets the output format.",
		Value:         flagvalue.Simple(false, &ctx.flags.json),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "agent",
		Description:   "Sets the output format.",
		Value:         flagvalue.Simple(false, &ctx.flags.agent),
		IsBooleanFlag: true,
		global:        true,
	}, &Flag{
		Name:          "markdown",
		Description:   "Sets the output format to markdown.",
		Value:         flagvalue.Simple(false, &ctx.flags.markdown),
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
		Description:   "Print the version of tfcloud CLI.",
		Value:         flagvalue.Simple(false, &ctx.flags.Version),
		IsBooleanFlag: true,
		global:        true,
	})

	// Setup the pre-run command
	cmd.PersistentPreRun = func(c *Command, args []string) error {
		// Setup the HTTP logger. We retrieve the commands logger so the API
		// logger is named with the subcommand.
		// ctx.HCP.SetLogger(newAPILogger(c.Logger()))
		// ctx.HCP.Debug = true

		if err := ctx.applyGlobalFlags(c); err != nil {
			return err
		}

		c.io = ctx.IO

		err := isAuthenticated(ctx, c, args)
		if err != nil {
			return err
		}

		return nil
	}
}

// applyGlobalFlags applies the global flags.
func (ctx *Context) applyGlobalFlags(c *Command) error {
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

	// Set the verbosity if the flag is set.
	verbosity := ctx.Profile.GetVerbosity()
	switch ctx.flags.debug {
	case 0:
		// nothing
	case 1:
		verbosity = "debug"
	default:
		verbosity = "trace"
	}

	if verbosity != "" {
		l := hclog.LevelFromString(verbosity)
		if l == hclog.NoLevel {
			return fmt.Errorf("invalid log level: %q", verbosity)
		}

		c.Logger().SetLevel(l)
	}

	// Set the output format if the flag is set.
	f := format.Unset
	if ctx.flags.json {
		f = format.JSON
	}
	if ctx.flags.agent {
		f = format.Agent
	}
	if ctx.flags.markdown {
		if f == format.Unset {
			f = format.Markdown
		} else {
			return fmt.Errorf("cannot set multiple output formats")
		}
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
func (ctx *Context) NewAPIClient() (*client.Client, error) {
	apiClient, err := client.New(ctx.Profile, http.Header{
		"User-Agent": []string{fmt.Sprintf("tfcloud-cli/%s", config.Version)},
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
	if isTopLevelCmd(args) || c.NoAuthRequired {
		return nil
	}

	if ctx.Profile.Token == "" {
		return authHelp(c.io)
	}

	return nil
}

func authHelp(io iostreams.IOStreams) error {
	cs := io.ColorScheme()
	help := heredoc.Docf(`
No authentication detected. To get started with tfcloud CLI, please run:  %s`,
		cs.String("tfcloud auth login").Bold().String())

	return errors.New(help)
}

// Used to parse commands and skip loading tfcloud profile.
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
