// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package cmd provides the structures and functions for constructing commands.
package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/posener/complete"
	flag "github.com/spf13/pflag"

	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/version"
)

var (
	// ErrDisplayHelp can be returned from a run command to print the help text.
	ErrDisplayHelp = errors.New("help")

	// ErrDisplayUsage can be returned from a run command to print the usage text.
	ErrDisplayUsage = errors.New("usage")

	// ErrUnderlyingError can be returned when the command ran successfully but
	// detected an underlying problem (e.g. inspecting a resource that is in an
	// error state). Exits with code 6 and no additional output.
	ErrUnderlyingError = errors.New("underlying error")
)

// Command is used to construct a command.
//
// To create a command that should not be invoked itself but is only used to
// nest sub-commands, construct the Command without a Run function and call
// AddChild to add the child commands.
type Command struct {
	//  ____                                        _        _   _
	// |  _ \  ___   ___ _   _ _ __ ___   ___ _ __ | |_ __ _| |_(_) ___  _ __
	// | | | |/ _ \ / __| | | | '_ ` _ \ / _ \ '_ \| __/ _` | __| |/ _ \| '_ \
	// | |_| | (_) | (__| |_| | | | | | |  __/ | | | || (_| | |_| | (_) | | | |
	// |____/ \___/ \___|\__,_|_| |_| |_|\___|_| |_|\__\__,_|\__|_|\___/|_| |_|

	// Name is the name of the command.
	Name string

	// Aliases is an array of aliases that can be used instead of the first word
	// in Usage.
	Aliases []string

	// ShortHelp is the short description shown when listing subcommands or when
	// the command is incorrectly invoked.
	ShortHelp string

	// LongHelp is the long message shown in the '<this-command> --help' output.
	LongHelp string

	// Examples is a set of examples of how to use the command.
	Examples []Example

	// AdditionalDocs allows adding additional documentation sections.
	AdditionalDocs []DocSection

	//  ____                _____
	// |  _ \ _   _ _ __   |  ___|   _ _ __   ___ ___
	// | |_) | | | | '_ \  | |_ | | | | '_ \ / __/ __|
	// |  _ <| |_| | | | | |  _|| |_| | | | | (__\__ \
	// |_| \_\\__,_|_| |_| |_|   \__,_|_| |_|\___|___/

	// PersistentPreRun is the set of functions to run for this command and all
	// subcommands.
	PersistentPreRun func(c *Command, args []string) error

	// RunF is the function that will be run when the command is invoked. It may
	// be nil if the command contains children.
	RunF func(c *Command, args []string) error

	//   ____             __ _
	//  / ___|___  _ __  / _(_) __ _
	// | |   / _ \| '_ \| |_| |/ _` |
	// | |__| (_) | | | |  _| | (_| |
	//  \____\___/|_| |_|_| |_|\__, |
	//                         |___/

	// Hidden hides the command from help output and command listings while
	// still allowing it to be invoked directly.
	Hidden bool

	// NoAuthRequired allows a command to indicate that authentication is not
	// required to be invoked.
	NoAuthRequired bool

	// Args documents the expected positional arguments.
	Args PositionalArguments

	// Flags is the set of flags for this command.
	Flags Flags

	//  ___       _                        _
	// |_ _|_ __ | |_ ___ _ __ _ __   __ _| |
	//  | || '_ \| __/ _ \ '__| '_ \ / _` | |
	//  | || | | | ||  __/ |  | | | | (_| | |
	// |___|_| |_|\__\___|_|  |_| |_|\__,_|_|
	//
	// ASCII generated with `figlet`

	// parent stores the reference to the parent command
	parent *Command

	// children stores child commands.
	children []*Command

	// allCommandFlags is full set of flags that apply to this command. It should be
	// accessed via the allFlags() method.
	allCommandFlags *flag.FlagSet

	// pflags contains persistent flags. It should be accessed via the
	// persistentFlags() method.
	pflags *flag.FlagSet

	// parentPflags is all inherited persistent flags. It should be accessed via
	// the parentPersistentFlags() method.
	parentPflags *flag.FlagSet

	// io formats output
	io iostreams.IOStreams

	// logger is the logger for this command
	logger hclog.Logger
}

// Example is an example of how to use a given command.
type Example struct {
	// Preamble is plaintext displayed before the command. Must be set, start
	// with a captital letter, and end with a colon.
	Preamble string

	// Command is the command example and any output it may contain
	Command string
}

// PositionalArguments documents a positional argument in a command.
type PositionalArguments struct {
	// Preamble allows injecting documentation before individual positional
	// arguments. It is optional.
	Preamble string

	// Args in an inorder list of arguments.
	Args []PositionalArgument

	// Validate if set is invoked to validate the command has received an
	// expected set of arguments. If not set, it will be defaulted based on the
	// passed Args. If no args are set, NoArgs will be enforced. If all
	// arguments are not repeated, ExactArgs will be used, otherwise,
	// MinimumNArgs will be set. To bypass argument validation, set this to
	// ArbitraryArgs.
	Validate ValidateArgsFunc

	// Autocomplete allows configuring autocompletion of arguments.
	Autocomplete complete.Predictor
}

// PositionalArgument documents a positional argument.
type PositionalArgument struct {
	// Name is the name of the positional argument
	Name string

	// Preamble is plaintext displayed before the command
	Documentation string

	// Optional marks the argument as optional. If set, the argument must be the
	// last argument or all positional arguments following this must be optional
	// as well.
	Optional bool

	// Repeatable marks whether the positional argument can be repeated. Only
	// the last argument is repeatable.
	Repeatable bool
}

// Flags is the set of flags for a command. It includes both local flags (specific to the command)
// and persistent flags (inherited by child commands).
type Flags struct {
	// Local is the set of flags for this command.
	Local []*Flag

	// Persistent is the set of flags that exist for this command and all
	// its children.
	Persistent []*Flag
}

// Flag instantiates a flag. An example flag is:
//
//	Flag{
//	  Name: "project",
//	  Shorthand: "p",
//	  DisplayValue: "ID",
//	  Description: "project sets the project ID to target.",
//	  Value: flagvalue.Simple("", &projectID),
//	  Required: true,
//	}
type Flag struct {
	// Name is the name of the flag. The name must be lower case.
	Name string

	// Shorthand is an optional shorthand for the flag. Name must still be set
	// and shorthand can only be a single, lowercase character.
	Shorthand string

	// Description is the description of the flag.
	Description string

	// DisplayValue is an optional string that will be used when displaying
	// help for using the flag. If set, the displayed value will be
	// --Name=DISPLAY_NAME, otherwise it will just be --Name.
	//
	// As an example, a Flag with the name "project" and display value of "ID",
	// would be displayed as "--project=ID".
	//
	// DisplayValue must be upper case.
	DisplayValue string

	// Value is the value that will be set by the flag. The value should be set
	// using flagvalue package.
	//
	// Examples are:
	//
	//   flagvalue.Simple("", &destination)
	//   flagvalue.Enum[string]([]string{"ONE", "TWO"}, "", &myEnum)
	Value flagvalue.Value

	// IsBooleanFlag indicates that the flag is a boolean flag.
	IsBooleanFlag bool

	// InvertBooleanNoValue treats the boolean flag as being specified to equal
	// the value false. This should be set if the flag indicates the disabling
	// of a value (e.g. --no-replication).
	InvertBooleanNoValue bool

	// Repeatable marks whether the positional argument can be repeated.
	Repeatable bool

	// Required marks whether the flag is required.
	Required bool

	// Hidden hides the flag.
	Hidden bool

	// Autocomplete is the predictor for this flag.
	Autocomplete complete.Predictor

	// global marks the flag as global.
	global bool
}

// DocSection allows adding additional documentation sections.
type DocSection struct {
	// The title of the section.
	Title string

	// Section documentation. No additional formatting will be applied other
	// than indenting.
	Documentation string
}

// AddChild is used to add a child command.
func (c *Command) AddChild(cmd *Command) {
	cmd.parent = c
	c.children = append(c.children, cmd)
}

// CommandPath returns the full command path excluding the root command name.
// For example, if the command tree is "tfctl" -> "run" -> "start", this returns "run start".
func (c *Command) CommandPath() string {
	var parts []string
	for cmd := c; cmd != nil && cmd.parent != nil; cmd = cmd.parent {
		parts = append([]string{cmd.Name}, parts...)
	}
	return strings.Join(parts, " ")
}

// SetIO sets the commands IO for input and output.
func (c *Command) SetIO(io iostreams.IOStreams) {
	c.io = io
}

// Logger returns a logger named according to the command.
func (c *Command) Logger(ctx *Context) hclog.Logger {
	if c.logger != nil {
		return c.logger
	}

	if c.parent != nil {
		pl := c.parent.Logger(ctx)
		c.logger = pl.Named(c.Name)
		return c.logger
	}

	// Create the logger
	io := ctx.IO

	logOpt := &hclog.LoggerOptions{
		Name:       version.Name,
		Level:      ctx.ResolveLogLevel(),
		Output:     io.Err(),
		TimeFn:     time.Now,
		TimeFormat: "15:04:05.000",
		Color:      hclog.ColorOff, // Enabled next, maybe
	}

	if io.ColorEnabled() && io.IsErrorTTY() {
		logOpt.Color = hclog.ForceColor
		logOpt.ColorHeaderAndFields = true
	}

	c.logger = hclog.New(logOpt)
	return c.logger
}

// ExitCodeError is an error that includes an exit code. If returned by a
// command run, the command will exit using the specified exit code.
type ExitCodeError struct {
	Err  error
	Code int
}

// NewExitError returns an ExitCodeError. This can be returned to have the
// command exit code be set to a specific value.
func NewExitError(code int, wrapErr error) error {
	return &ExitCodeError{
		Err:  wrapErr,
		Code: code,
	}
}

func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("exit code %d: %v", e.Code, e.Err)
	}

	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitCodeError) Unwrap() error { return e.Err }
