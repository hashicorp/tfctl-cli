// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/go-openapi/runtime"
	"github.com/hashicorp/cli"
	"github.com/muesli/reflow/indent"
	"github.com/muesli/reflow/wordwrap"
	"github.com/posener/complete"
	"github.com/spf13/pflag"

	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/ld"
)

// Run runs the given command.
func (c *Command) Run(args []string) int {
	if c.RunF == nil {
		if len(c.children) != 0 {
			return cli.RunResultHelp
		}

		fmt.Println("Command has no run function or children. This is an invalid command")
		return 1
	}

	// Get the colorscheme
	io := c.getIO()
	cs := c.getIO().ColorScheme()

	// Parse the flags
	if err := c.parseFlags(args); err != nil {
		fmt.Fprintf(io.Err(), "%s %s\n", cs.ErrorLabel(), err)
		fmt.Fprintln(io.Err())
		fmt.Fprint(io.Err(), c.usageHelp())
		return 1
	}

	// Ensure all required flags have been set.
	var requiredFlags []string
	c.allFlags().VisitAll(func(f *pflag.Flag) {
		required := isFlagRequired(f.Annotations)
		if required && !f.Changed {
			requiredFlags = append(requiredFlags, flagString(f))
		}
	})

	if len(requiredFlags) != 0 {
		plural := ""
		if len(requiredFlags) > 1 {
			plural = "s"
		}
		requiredErr := wordWrap(fmt.Sprintf("missing required flag%s: %s\n",
			plural, strings.Join(requiredFlags, ", ")), 80)
		requiredErr = strings.TrimSpace(indent.String(requiredErr, 2))

		fmt.Fprintf(io.Err(), "%s %s\n", cs.ErrorLabel(), requiredErr)
		fmt.Fprintln(io.Err())
		fmt.Fprint(io.Err(), c.usageHelp())
		return 1
	}

	// Capture the args after parsing flags
	parsedArgs := c.allFlags().Args()

	// Run the prerun functions starting from the root parent and working down.
	prerunFuncs := []func(c *Command, args []string) error{}
	for cc := c; cc != nil; cc = cc.parent {
		if f := cc.PersistentPreRun; f != nil {
			prerunFuncs = append(prerunFuncs, f)
		}
	}
	slices.Reverse(prerunFuncs)
	for _, f := range prerunFuncs {
		if err := f(c, parsedArgs); err != nil {
			fmt.Fprintln(io.Err(), err)
			return 1
		}
	}

	// Validate our arguments.
	if err := c.Args.validateFunc()(c, parsedArgs); err != nil {
		fmt.Fprintf(io.Err(), "%s %s\n", cs.ErrorLabel(), err)
		fmt.Fprintln(io.Err())
		fmt.Fprint(io.Err(), c.usageHelp())
		return 1
	}

	// Run the command
	if err := c.RunF(c, parsedArgs); err != nil {
		exitCode := 1
		var runtimeErr runtime.ClientResponseStatus
		var exitCodeErr *ExitCodeError
		if errors.Is(err, ErrDisplayHelp) {
			return cli.RunResultHelp
		} else if errors.Is(err, ErrDisplayUsage) {
			fmt.Fprint(io.Err(), c.usageHelp())
			return 1
		} else if errors.As(err, &runtimeErr) && runtimeErr.IsCode(http.StatusUnauthorized) {
			// TODO: This runtimeErr is inaccurate for HCPTF
			// Request failed because of authentication issues.
			fmt.Fprintf(io.Err(), "%s %s\n\n", cs.ErrorLabel(), authErrorHelp(io, c.commandPath(), args))
			return 1
		} else if errors.As(err, &exitCodeErr) {
			exitCode = exitCodeErr.Code
		}

		fmt.Fprintf(io.Err(), "%s %s\n", cs.ErrorLabel(), wordWrap(err.Error(), 120))
		return exitCode
	}

	return 0
}

// authErrorHelp returns a help message for recovering from authentication errors.
func authErrorHelp(io iostreams.IOStreams, commandPath string, args []string) string {
	// Build the original command
	command := "$ " + commandPath
	for _, a := range args {
		// If there are spaces in the argument, quote it.
		if strings.Contains(a, " ") {
			command += fmt.Sprintf(" %s", strconv.Quote(a))
		} else {
			command += fmt.Sprintf(" %s", a)
		}
	}

	// Quote the entire command so we can inject it into the template as a
	// variable.
	command = strconv.Quote(command)

	// Render the help message to logout, login, and re-run the command.
	return heredoc.New(io, heredoc.WithPreserveNewlines(), heredoc.WithWidth(0)).Mustf(`
		Unauthorized request. Re-attempt by first logging out and back in, and then re-run the command.

		  {{ Bold "$ tfcloud auth logout" }}
		  {{ Bold "$ tfcloud auth login" }}
		  {{ with $cmd := %s }}{{ Bold $cmd }}{{ end }}
	`, command)
}

// helpEntry is used to structure help output with titles.
type helpEntry struct {
	Title string
	Body  string
}

// help prints the long help output for the command. If the command is invoked
// because no valid child command could be matched, a command suggestion is
// made.
func (c *Command) help() string {
	// If we have an invalid command, print a suggestion and the usage help.
	var buf bytes.Buffer
	if help, _ := c.allFlags().GetBool("help"); !help && c.RunF == nil {
		invalid := ""
		commands := strings.Split(c.commandPath(), " ")
		for i, arg := range os.Args {
			// Have to check the raw argument as well to handle `tfcloud -h` since
			// the flags will not have been parsed.
			if arg == "-h" || arg == "--help" {
				break
			}

			if i >= len(commands) {
				invalid = arg
				break
			}
		}

		if invalid != "" {
			c.nestedSuggestFunc(&buf, invalid)
			_, _ = buf.WriteString(c.usageHelp())
			return buf.String()
		}
	}

	// Add the command name
	cs := c.getIO().ColorScheme()
	helpEntries := []helpEntry{}

	// Add the command usage
	helpEntries = append(helpEntries, helpEntry{"USAGE", c.useLine()})

	// Add the description
	helpEntries = append(helpEntries, helpEntry{"DESCRIPTION", wordWrap(c.LongHelp, 80)})

	// Print any available aliases
	if len(c.Aliases) > 0 {
		usages := c.aliasUsages()
		var aliases []string
		for a, u := range usages {
			aliases = append(aliases, fmt.Sprintf("%s - %s", a, u))
		}

		helpEntries = append(helpEntries, helpEntry{"ALIASES", strings.Join(aliases, "\n")})
	}

	commandHelp := func(group bool) {
		// Determine the minimum padding
		maxLength := 0
		for _, c := range c.children {
			if (group && c.RunF != nil) || (!group && c.RunF == nil) {
				continue
			}

			maxLength = max(maxLength, len(c.Name))
		}

		namePadding := maxLength + 2
		var names []string
		for _, c := range c.children {
			if (group && c.RunF != nil) || (!group && c.RunF == nil) {
				continue
			}

			names = append(names, rpad(c.Name+":", namePadding)+c.ShortHelp)
		}

		slices.Sort(names)
		if len(names) == 0 {
			return
		}

		title := "COMMANDS"
		if group {
			title = "COMMAND GROUPS"
		}

		helpEntries = append(helpEntries, helpEntry{
			Title: title,
			Body:  strings.Join(names, "\n"),
		})
	}

	// If we have children, display the available commands
	if len(c.children) != 0 {
		commandHelp(true)
		commandHelp(false)
	}

	// Print the examples
	if len(c.Examples) != 0 {
		var buf bytes.Buffer
		for _, e := range c.Examples {
			fmt.Fprintln(&buf, e.text(cs))
		}

		helpEntries = append(helpEntries, helpEntry{"EXAMPLES", buf.String()})
	}

	if args := c.Args.text(cs); args != "" {
		helpEntries = append(helpEntries, helpEntry{"POSITIONAL ARGUMENTS", args})
	}

	// Print flags only if the command is runnable
	helpEntries = append(helpEntries, c.flagsHelpEntry()...)

	// Add any additional documentation provided
	for _, d := range c.AdditionalDocs {
		helpEntries = append(helpEntries, helpEntry{strings.ToUpper(d.Title), d.Documentation})
	}

	for i, e := range helpEntries {
		if e.Title != "" {
			// If there is a title, add indentation to each line in the body
			fmt.Fprintln(&buf, cs.String(e.Title).Bold())
			fmt.Fprintln(&buf, indent.String(strings.Trim(e.Body, "\r\n"), 2))
		} else {
			// If there is no title print the body as is
			fmt.Fprintln(&buf, e.Body)
		}

		if i != len(helpEntries)-1 {
			fmt.Fprintln(&buf)
		}
	}

	return buf.String()
}

// flagsHelpEntry returns help entries for the command's flags.
func (c *Command) flagsHelpEntry() []helpEntry {
	var helpEntries []helpEntry

	// If we are the root command, just print global flags.
	if c.parent == nil && c.RunF == nil {
		helpEntries = append(helpEntries, helpEntry{
			Title: "GLOBAL FLAGS",
			Body:  flagsetUsage(c.globalFlags()),
		})
		return helpEntries
	}

	// Print flags only if the command is runnable
	if c.RunF == nil {
		return nil
	}

	flagSets := []struct {
		flags *pflag.FlagSet
		name  string
	}{
		{
			flags: c.localFlags(),
			name:  "",
		},
		{
			flags: c.inheritedFlags(),
			name:  "INHERITED ",
		},
	}

	for _, set := range flagSets {
		required, optional := splitRequiredFlags(set.flags)
		if required.HasFlags() {
			flagUsages := flagsetUsage(required)
			helpEntries = append(helpEntries, helpEntry{
				Title: fmt.Sprintf("REQUIRED %sFLAGS", set.name),
				Body:  flagUsages,
			})

			if optional.HasFlags() {
				flagUsages := flagsetUsage(optional)
				helpEntries = append(helpEntries, helpEntry{
					Title: fmt.Sprintf("OPTIONAL %sFLAGS", set.name),
					Body:  flagUsages,
				})
			}
		} else if optional.HasFlags() {
			flagUsages := flagsetUsage(optional)
			helpEntries = append(helpEntries, helpEntry{
				Title: fmt.Sprintf("%sFLAGS", set.name),
				Body:  flagUsages,
			})
		}
	}

	globalFlagUsages := flagsetUsageShort(c.globalFlags(), "For more global flag details, run $ tfcloud --help")
	if globalFlagUsages != "" {
		helpEntries = append(helpEntries, helpEntry{"GLOBAL FLAGS", globalFlagUsages})
	}

	return helpEntries
}

// text returns the help text for a given example.
func (e *Example) text(cs *iostreams.ColorScheme) string {
	var buf bytes.Buffer

	if e.Preamble != "" {
		fmt.Fprintln(&buf, wordWrap(e.Preamble, 80))
		fmt.Fprintln(&buf)
	}
	if e.Command != "" {
		// Use a higher limit for command wrapping since they may include
		// potentially long identifiers.
		fmt.Fprintln(&buf, cs.String(wordWrap(e.Command, 120)).Color(cs.Green()))
	}

	return buf.String()
}

// text returns the help text for the positional arguments.
func (p PositionalArguments) text(cs *iostreams.ColorScheme) string {
	var buf bytes.Buffer
	if p.Preamble != "" {
		fmt.Fprintln(&buf, p.Preamble)
	}

	for _, a := range p.Args {
		fmt.Fprintln(&buf, a.text(cs))
	}

	return buf.String()
}

// text returns the help text for a positional argument.
func (a PositionalArgument) text(cs *iostreams.ColorScheme) string {
	var buf bytes.Buffer

	nameUpper := strings.ToUpper(a.Name)
	repeatable := ""
	if a.Repeatable {
		repeatable = fmt.Sprintf(" [%s ...]", nameUpper)
	}
	fmt.Fprintf(&buf, "%s%s\n", cs.String(nameUpper).Underline(), repeatable)
	if a.Optional {
		fmt.Fprintln(&buf, indent.String(cs.String("Optional Argument\n").Italic().String(), 2))
	}
	fmt.Fprintln(&buf, indent.String(wordWrap(a.Documentation, 80), 2))

	return buf.String()
}

// aliasUsages returns a map from the alias to its usage.
func (c *Command) aliasUsages() map[string]string {
	aliases := make(map[string]string)
	for _, a := range c.Aliases {
		var useline string
		if c.hasParent() {
			useline = c.parent.commandPath() + " " + a
		} else {
			useline = a
		}
		if c.RunF == nil {
			useline += " <command>"
		}

		aliases[a] = useline
	}

	return aliases
}

// usageHelp returns the short usage help that displays the commands usage and
// flags.
func (c *Command) usageHelp() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Usage: %s\n", c.useLine())
	fmt.Fprintln(&buf)

	if len(c.children) != 0 {
		// Determine the minimum padding
		maxLength := 0
		for _, c := range c.children {
			maxLength = max(maxLength, len(c.Name))
		}

		namePadding := maxLength + 2
		var names []string
		for _, c := range c.children {
			names = append(names, rpad(c.Name+":", namePadding)+c.ShortHelp)
		}

		// Sort the names
		slices.Sort(names)

		fmt.Fprintln(&buf, "Commands:")
		fmt.Fprint(&buf, indent.String(strings.Join(names, "\n"), 2))
		return buf.String()
	}

	required, optional := splitRequiredFlags(c.nonGlobalFlags())
	if required.HasFlags() {
		fmt.Fprintln(&buf, "Required Flags:")
		fmt.Fprint(&buf, indent.String(flagsetUsage(required), 2))

		if optional.HasFlags() {
			fmt.Fprintln(&buf)
			fmt.Fprintln(&buf, "Optional Flags:")
			fmt.Fprint(&buf, indent.String(flagsetUsage(optional), 2))
		}
	} else if optional.HasFlags() {
		// If all the flags are optional, group them together.
		fmt.Fprintln(&buf, "Flags:")
		fmt.Fprint(&buf, indent.String(flagsetUsage(optional), 2))
	}

	// Print a smaller help output for global flags.
	global := c.globalFlags()
	if global.HasFlags() {
		fmt.Fprintln(&buf, "Global Flags:")
		fmt.Fprint(&buf, indent.String(flagsetUsageShort(global, "For more global flag details, run $ tfcloud --help"), 2))
	}

	return buf.String()
}

// Display helpful error message in case subcommand name was mistyped.
func (c *Command) nestedSuggestFunc(w io.Writer, arg string) {
	fmt.Fprintf(w, "unknown command %q for %q\n", arg, c.commandPath())

	var candidates []string
	if arg == "help" {
		candidates = []string{"--help"}
	} else {
		candidates = c.suggestionsFor(arg)
	}

	if len(candidates) > 0 {
		fmt.Fprint(w, "\nDid you mean this?\n")
		for _, c := range candidates {
			fmt.Fprintf(w, "  %s\n", c)
		}
	}

	fmt.Fprintln(w)
}

// suggestionsFor provides suggestions for the typedName.
func (c *Command) suggestionsFor(typedName string) []string {
	options := make([]string, len(c.children))
	for i, c := range c.children {
		options[i] = c.Name
	}

	typedNameLower := strings.ToLower(typedName)
	return ld.SuggestionsWithOverride(typedName, options, 2, true, func(_, option string) bool {
		return strings.HasPrefix(strings.ToLower(option), typedNameLower)
	})
}

// useLine puts out the full usage for a given command (including parents).
func (c *Command) useLine() string {
	var useline string
	if c.hasParent() {
		useline = c.parent.commandPath() + " " + c.Name
	} else {
		useline = c.Name
	}
	if c.RunF == nil {
		useline += " <command>"
	}

	// Add any positional arguments
	cs := c.getIO().ColorScheme()
	for _, a := range c.Args.Args {
		name := cs.String(strings.ToUpper(a.Name)).Underline()
		if !a.Optional {
			if a.Repeatable {
				useline += fmt.Sprintf(" %s [%s ...]", name, name)
			} else {
				useline += fmt.Sprintf(" %s", name)
			}
		} else {
			if a.Repeatable {
				useline += fmt.Sprintf(" [%s ...]", name)
			} else {
				useline += fmt.Sprintf(" [%s]", name)
			}
		}
	}

	// Add the flags
	if c.hasAvailableFlags() {
		required, _ := splitRequiredFlags(c.allFlags())
		if required.HasFlags() {
			required.VisitAll(func(f *pflag.Flag) {
				useline += fmt.Sprintf(" %s", flagString(f))
			})

		}
		useline += " [Optional Flags]"
	}

	wrapped := wordWrap(useline, 80)
	indented := indent.String(wrapped, 2)
	return strings.TrimSpace(indented)
}

// commandPath returns the full path to this command.
func (c *Command) commandPath() string {
	if c.hasParent() {
		return c.getParent().commandPath() + " " + c.Name
	}
	return c.Name
}

// buildFlags converts the flags from the cmd.Flag format to a pflag.FlagSet.
func (c *Command) buildFlags() {
	// We have already built the flags.
	if c.allCommandFlags != nil {
		return
	}

	// Instantiate the various flag sets
	c.allCommandFlags = pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	c.allCommandFlags.SetOutput(c.getIO().Err())
	c.pflags = pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	c.pflags.SetOutput(c.getIO().Err())
	c.parentPflags = pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	c.parentPflags.SetOutput(c.getIO().Err())

	// Convert the flags to a pflag and add them to the correct set
	for _, f := range c.Flags.Local {
		c.allCommandFlags.AddFlag(f.pflag())
	}
	for _, f := range c.Flags.Persistent {
		p := f.pflag()
		c.allCommandFlags.AddFlag(p)
		c.pflags.AddFlag(p)
	}

	// Add all parent persistent flags
	for parent := c.parent; parent != nil; parent = parent.parent {
		parentPFlags := parent.persistentFlags()
		c.allCommandFlags.AddFlagSet(parentPFlags)
		c.parentPflags.AddFlagSet(parentPFlags)
	}
}

// pflag returns the pflag.Flag representation of the Flag.
func (f *Flag) pflag() *pflag.Flag {
	a := newFlagAnnotations()
	p := &pflag.Flag{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Usage:       f.Description,
		Value:       f.Value,
		Hidden:      f.Hidden,
		Annotations: a,
	}

	if f.IsBooleanFlag {
		p.NoOptDefVal = "true"
		if f.InvertBooleanNoValue {
			p.NoOptDefVal = "false"
		}
	}

	if f.Required {
		a.Required()
	}

	if f.DisplayValue != "" {
		a.DisplayValue(f.DisplayValue)
	}

	if f.global {
		a.Global()
	}

	if f.Repeatable {
		a.Repeatable()
	}

	return p
}

// allFlags returns the complete FlagSet that applies to this command. The flagset
// will include any persistent flag defined by parents of the given command.
func (c *Command) allFlags() *pflag.FlagSet {
	c.buildFlags()
	return c.allCommandFlags
}

// persistentFlags is a flagset for defining flags that should apply to this
// command and all its children. When accessing flags defined here, prefer using
// Flags() as it will contain both flags from this flagset and local flags.
func (c *Command) persistentFlags() *pflag.FlagSet {
	c.buildFlags()
	return c.pflags
}

// parentPersistentFlags returns the persistent FlagSet set by parent commands.
func (c *Command) parentPersistentFlags() *pflag.FlagSet {
	c.buildFlags()
	return c.parentPflags
}

// localFlags returns all flags defined by this command.
func (c *Command) localFlags() *pflag.FlagSet {
	c.buildFlags()
	local := pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	local.SetOutput(c.getIO().Err())

	addToLocal := func(f *pflag.Flag) {
		// Add the flag if it is not a parent PFlag, or it shadows a parent PFlag
		if local.Lookup(f.Name) == nil && f != c.parentPersistentFlags().Lookup(f.Name) {
			local.AddFlag(f)
		}
	}
	c.allFlags().VisitAll(addToLocal)
	c.persistentFlags().VisitAll(addToLocal)
	return local
}

// inheritedFlags returns all inherited, non-global flags.
func (c *Command) inheritedFlags() *pflag.FlagSet {
	c.buildFlags()
	inherited := pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	inherited.SetOutput(c.getIO().Err())

	addToInherited := func(f *pflag.Flag) {
		if _, ok := f.Annotations[flagAnnotationGlobal]; !ok {
			inherited.AddFlag(f)
		}
	}
	c.parentPersistentFlags().VisitAll(addToInherited)
	return inherited
}

// globalFlags returns all flags marked as global.
func (c *Command) globalFlags() *pflag.FlagSet {
	c.buildFlags()
	global := pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	global.SetOutput(c.getIO().Err())

	addToGlobal := func(f *pflag.Flag) {
		if isFlagGlobal(f.Annotations) {
			global.AddFlag(f)
		}
	}
	c.allFlags().VisitAll(addToGlobal)
	return global
}

// nonGlobal returns all flags that apply to this command that aren't global.
func (c *Command) nonGlobalFlags() *pflag.FlagSet {
	c.buildFlags()
	nonglobal := pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	nonglobal.SetOutput(c.getIO().Err())

	addToNonGlobal := func(f *pflag.Flag) {
		if !isFlagGlobal(f.Annotations) {
			nonglobal.AddFlag(f)
		}
	}
	c.allFlags().VisitAll(addToNonGlobal)
	return nonglobal
}

// parseFlags parses the flags from the arguments.
func (c *Command) parseFlags(args []string) error {
	c.buildFlags()
	if err := c.allFlags().Parse(args); err != nil {
		return err
	}

	return nil
}

// hasAvailableFlags checks if the command contains any flags (local plus persistent from the entire
// structure) which are not hidden or deprecated.
func (c *Command) hasAvailableFlags() bool {
	c.buildFlags()
	return c.allFlags().HasAvailableFlags()
}

// splitRequiredFlags returns two flagset, one that contains the required flags
// and the other that contains optional flags.
func splitRequiredFlags(flagset *pflag.FlagSet) (required, optional *pflag.FlagSet) {
	required = pflag.NewFlagSet("tfcloud", pflag.ContinueOnError)
	optional = pflag.NewFlagSet("tfcloud", pflag.ContinueOnError)
	flagset.VisitAll(func(f *pflag.Flag) {
		if _, ok := f.Annotations[flagAnnotationRequired]; ok {
			required.AddFlag(f)
		} else {
			optional.AddFlag(f)
		}
	})

	return required, optional
}

// flagsetUsage returns the usage string for the given flagset. Each flag is
// described on its own line with its description below. For a more compact
// representation, use flagsetUsageShort.
func flagsetUsage(flags *pflag.FlagSet) string {
	var buf bytes.Buffer
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}

		longDisplay := flagString(flag)
		if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
			fmt.Fprintf(&buf, "-%s, %s\n", flag.Shorthand, longDisplay)
		} else {
			fmt.Fprintf(&buf, "%s\n", longDisplay)
		}

		// Add the usage
		fmt.Fprintf(&buf, "%s\n\n", indent.String(wordWrap(flag.Usage, 80), 2))
	})

	return buf.String()
}

// flagsetUsageShort returns the usage string for the given flagset in a compact
// form where the description is omitted and optional suffix can be provided
// which will be printed on its own line below the flag usage.
func flagsetUsageShort(flags *pflag.FlagSet, suffix string) string {
	var names []string
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}

		names = append(names, flagString(flag))
	})

	usage := fmt.Sprintf("%s\n", strings.Join(names, ", "))
	if suffix != "" {
		suffix = strings.TrimSpace(suffix)
		usage = fmt.Sprintf("%s\n%s\n", usage, suffix)
	}

	return wordWrap(usage, 80)
}

// flagString returns a string representation for the flag.
func flagString(f *pflag.Flag) string {
	v := getFlagDisplayValue(f.Annotations)
	repeatable := isFlagRepeatable(f.Annotations)
	if v != "" {
		if repeatable {
			return fmt.Sprintf("--%s=%s [Repeatable]", f.Name, v)
		}
		return fmt.Sprintf("--%s=%s", f.Name, v)
	}

	if repeatable {
		return fmt.Sprintf("--%s [Repeatable]", f.Name)
	}

	return fmt.Sprintf("--%s", f.Name)
}

// getIO retrieves the IO configured for the command and configures it to output
// to Err even if quiet is specified. To access the raw IOStreams, use
// getRawIO.
func (c *Command) getIO() iostreams.IOStreams {
	return iostreams.UseLoud(c.getRawIO())
}

// getRawIO gets the configured IO for the command.
func (c *Command) getRawIO() iostreams.IOStreams {
	if c.io != nil {
		return c.io
	}

	return c.parent.getRawIO()
}

// hasParent determines if the command is a child command.
func (c *Command) hasParent() bool {
	return c.parent != nil
}

// getParent returns a commands parent command.
func (c *Command) getParent() *Command {
	return c.parent
}

// getAutocompleteFlags builds the complete Flags, supporting both the long and
// short declerations.
func (c *Command) getAutocompleteFlags() complete.Flags {
	// Get all flag predictors from this command to the root
	allPredictors := make(map[string]complete.Predictor)
	for c := c; c != nil; c = c.parent {
		for _, flag := range c.Flags.Local {
			allPredictors[flag.Name] = flag.Autocomplete
		}

		for _, flag := range c.Flags.Persistent {
			allPredictors[flag.Name] = flag.Autocomplete
		}
	}

	flagPredictors := make(map[string]complete.Predictor)
	c.allFlags().VisitAll(func(f *pflag.Flag) {
		p, ok := allPredictors[f.Name]
		if !ok {
			return
		}

		flagPredictors["--"+f.Name] = p
		if f.Shorthand != "" {
			flagPredictors["-"+f.Shorthand] = p
		}
	})

	return flagPredictors
}

// validateFunc returns the set validation function or a default argument
// validation function based on the documented arguments.
func (p PositionalArguments) validateFunc() ValidateArgsFunc {
	if p.Validate != nil {
		return p.Validate
	}

	numArgs := len(p.Args)
	if numArgs == 0 {
		return NoArgs
	}

	optional := 0
	for _, a := range p.Args {
		if a.Optional {
			optional++
		}

		if a.Repeatable {
			return MinimumNArgs(numArgs - optional)
		}
	}

	if optional > 0 {
		return RangeArgs(numArgs-optional, numArgs)
	}

	return ExactArgs(numArgs)
}

// rpad adds padding to the right of a string.
func rpad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds ", padding)
	return fmt.Sprintf(template, s)
}

// wordWrap wraps an input at the given wrap length. It uses a customized
// wordwrap.Writer that is more appropriate for splitting command line flags.
func wordWrap(input string, wrap int) string {
	w := wordwrap.NewWriter(wrap)
	w.Breakpoints = []rune{}
	_, _ = w.Write([]byte(input))
	_ = w.Close()
	return w.String()
}
