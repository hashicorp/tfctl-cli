// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/pflag"

	"github.com/hashicorp/tfcloud/internal/iostreams"
)

const (
	// shortHelpMaxLength is the maximum length of the short help text.
	shortHelpMaxLength = 60
)

var (
	// commandNameRegex is used to validate the command names. It enforces that
	// the command name is lower case and contains only letters and hyphens.
	commandNameRegex = regexp.MustCompile(`^[a-z]+([-][a-z]+)*$`)

	// errCommandNameInvalid is returned when the command name is invalid.
	errCommandNameInvalid = fmt.Errorf("only lower case names with hyphens are allowed")

	// shortHelpRegex is used to validate the short help text. It enforces that
	// the short help text starts with a capital letter, ends with a period, and
	// contains only letters, apostrophes, hyphens, and spaces.
	shortHelpRegex = regexp.MustCompile(`^[A-Z][a-zA-Z-\s']+\.$`)

	// errShortHelpInvalid is returned when the short help text is invalid.
	errShortHelpInvalid = fmt.Errorf("short help text must start with a capital letter, end with a period, and contain only letters, apostrophes, hyphens, and spaces")

	// flagNameRegex is used to validate the flag names. It enforces that the flag
	// name is lower case and contains only letters and hyphens.
	flagNameRegex = regexp.MustCompile(`^[a-z0-9]+([-][a-z0-9]+)*$`)

	// errFlagNameInvalid is returned when the flag name is invalid.
	errFlagNameInvalid = fmt.Errorf("only lower case letters, numbers, and hyphens are allowed")

	// flagDescriptionRegex is used to validate the flag descriptions. It enforces that the
	// flag description starts with a capital letter and ends with a period.
	flagDescriptionRegex = regexp.MustCompile(`(?s)^[A-Z].+\.$`)

	// errFlagDescriptionInvalid is returned when the flag description is invalid.
	errFlagDescriptionInvalid = fmt.Errorf("description must start with a capital letter and end with a period")

	// argsPreambleRegex is used to validate the preamble of the positional
	// arguments. It enforces that the preamble starts with a capital letter and
	// ends with a period.
	argsPreambleRegex = regexp.MustCompile(`(?s)^[A-Z].+\.$`)

	// errArgsPreambleInvalid is returned when the preamble of the positional
	// arguments is invalid.
	errArgsPreambleInvalid = fmt.Errorf("preable must start with a capital letter and end with a period")

	// examplePreambleRegex is used to validate the preamble of the examples. It
	// enforces that the preamble starts with a capital letter and ends with a
	// colon.
	examplePreambleRegex = regexp.MustCompile(`(?s)^[A-Z].+:$`)

	// eaxmplePreambleInvalidError is returned when the preamble of the example
	// is invalid.
	errExamplePreambleInvalid = fmt.Errorf("preamble must start with a capital letter and end with a colon")

	// errCommandLongHelpPrefixInvalid is returned when the long help prefix is
	// invalid for a command.
	errCommandLongHelpPrefixInvalid = func(c *Command) error {
		template, plaintext := expectedLongHelpPrefix(c)

		got := c.LongHelp
		if len(got) > 100 {
			got = got[:100] + "..."
		}

		return fmt.Errorf("invalid command long help prefix.\n\nWANT: %q\nGOT: %q\nREPLACE WITH: %q", plaintext, got, template)
	}
)

// Validate validates the command and all of its children.
func (c *Command) Validate() error {
	var validationErr error

	// Validate ourselves and then the children.
	if err := c.validate(); err != nil {
		validationErr = errors.Join(validationErr, err)
	}

	namesAndAliases := make(map[string]struct{}, len(c.children))
	for _, child := range c.children {
		if err := child.Validate(); err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("error validating command %s: %w", child.Name, err))
			continue
		}

		// Ensure the child name and its aliases are unique.
		if _, ok := namesAndAliases[child.Name]; ok {
			validationErr = errors.Join(validationErr, fmt.Errorf("child command name %q used by a sibling name or alias", child.Name))
		} else {
			namesAndAliases[child.Name] = struct{}{}
		}

		for _, alias := range child.Aliases {
			if _, ok := namesAndAliases[alias]; ok {
				validationErr = errors.Join(validationErr, fmt.Errorf("child command %q has alias %q already used by a sibling name or alias", child.Name, alias))
			} else {
				namesAndAliases[alias] = struct{}{}
			}
		}
	}

	return validationErr
}

func (c *Command) validate() error {
	var validationErr error

	// Validate the name
	if c.Name == "" {
		validationErr = errors.Join(validationErr, fmt.Errorf("command name cannot be empty"))
	} else if !commandNameRegex.MatchString(c.Name) {
		validationErr = errors.Join(validationErr, errCommandNameInvalid)
	}

	// Ensure the aliases are valid and there are no duplicates in the aliases
	aliases := make(map[string]struct{}, len(c.Aliases))
	for _, alias := range c.Aliases {
		// Ensure the alias is not the name
		if alias == c.Name {
			validationErr = errors.Join(validationErr, fmt.Errorf("command name cannot be an alias"))
			continue
		}

		// Check for duplicates
		if _, ok := aliases[alias]; ok {
			validationErr = errors.Join(validationErr, fmt.Errorf("duplicate alias %q found", alias))
		}
		aliases[alias] = struct{}{}

		// Validate the alias
		if alias == "" {
			validationErr = errors.Join(validationErr, fmt.Errorf("alias name is empty"))
		} else if !commandNameRegex.MatchString(alias) {
			validationErr = errors.Join(validationErr, fmt.Errorf("alias %q: %w", alias, errCommandNameInvalid))
		}
	}

	// Validate that the help text is set
	if c.ShortHelp == "" || c.LongHelp == "" {
		validationErr = errors.Join(validationErr, fmt.Errorf("short and long help text must be set"))
	}

	// Validate the short help.
	if len(c.ShortHelp) > shortHelpMaxLength {
		validationErr = errors.Join(validationErr,
			fmt.Errorf("short help text is too long. Max length is %d; got %q (%d)",
				shortHelpMaxLength, c.ShortHelp, len(c.ShortHelp)))
	} else if !shortHelpRegex.MatchString(c.ShortHelp) {
		validationErr = errors.Join(validationErr, fmt.Errorf("%w; got %q", errShortHelpInvalid, c.ShortHelp))
	}

	// Validate the long help. Since the LongHelp is rendered, we can't fully
	// validate that the template is correct. Instead, we validate that the
	// plaintext output is correct.
	_, longHelpPrefix := expectedLongHelpPrefix(c)
	if c.parent != nil && !strings.HasPrefix(strings.TrimSpace(c.LongHelp), longHelpPrefix) {
		validationErr = errors.Join(validationErr, errCommandLongHelpPrefixInvalid(c))
	}

	// Validate the additional documentation sections
	for i, d := range c.AdditionalDocs {
		if err := d.validate(); err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("error validating documentation section %d: %w", i, err))
		}
	}

	// Validate the examples
	for i, e := range c.Examples {
		if err := e.validate(); err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("error validating example %d: %w", i, err))
		}
	}

	// Validate IO is set
	if err := c.validateIO(); err != nil {
		validationErr = errors.Join(validationErr, err)

		// Inject a test io so we can continue validation
		c.io = iostreams.Test()
	}

	// Validate the Flags
	if err := c.validateFlags(); err != nil {
		validationErr = errors.Join(validationErr, err)
	}

	// validate the positional arguments
	if err := c.Args.validate(); err != nil {
		validationErr = errors.Join(validationErr, fmt.Errorf("error validating positional arguments: %w", err))
	}

	// Validate that either RunF or Children are set, but not both.
	if c.RunF == nil && len(c.children) == 0 {
		validationErr = errors.Join(validationErr, fmt.Errorf("either RunF or Children must be set"))
	} else if c.RunF != nil && len(c.children) > 0 {
		validationErr = errors.Join(validationErr, fmt.Errorf("both RunF and Children cannot be set"))
	}

	return validationErr
}

// expectedLongHelpPrefix returns the expected long help prefix for the command
// that should be present in the template, and the plaintext version to test
// against.
func expectedLongHelpPrefix(c *Command) (templated, plaintext string) {
	group := " group"
	if c.RunF != nil {
		group = ""
	}

	templated = fmt.Sprintf(`The {{ template "mdCodeOrBold" %q }} command%s`, c.commandPath(), group)
	plaintext = fmt.Sprintf("The %s command%s", c.commandPath(), group)
	return
}

// validateIO checks that the io is set on the command or any parent command.
func (c *Command) validateIO() error {
	for c := c; c != nil; c = c.parent {
		if c.io != nil {
			return nil
		}
	}

	return fmt.Errorf("io not set on command or any parent command")
}

func (c *Command) validateFlags() error {
	var validationErr error
	defer func() {
		if err := recover(); err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("panic validating flags: %v", err))
		}
	}()

	for _, flag := range c.Flags.Local {
		if err := flag.validate(); err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("error validating local flag %q: %w", flag.Name, err))
		}
	}

	for _, flag := range c.Flags.Persistent {
		if err := flag.validate(); err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("error validating persistent flag %q: %w", flag.Name, err))
		}
	}

	// Return early since visiting invalid flags can cause a panic.
	if validationErr != nil {
		return validationErr
	}

	// Ensure local flags do not override parent persistent flags
	var flagErr error
	localFlags, inheritedFlags := c.localFlags(), c.parentPersistentFlags()
	localFlags.VisitAll(func(f *pflag.Flag) {
		if flagErr != nil {
			return
		}

		if inheritedFlags.Lookup(f.Name) != nil {
			flagErr = fmt.Errorf("local flag %q overrides inherited persistent flag", f.Name)
		}
	})
	if flagErr != nil {
		validationErr = errors.Join(validationErr, flagErr)
	}

	return validationErr
}

func (f *Flag) validate() error {
	var validationErr error

	if f.Name == "" {
		validationErr = errors.Join(validationErr, fmt.Errorf("name cannot be empty"))
	} else if !flagNameRegex.MatchString(f.Name) {
		validationErr = errors.Join(validationErr, fmt.Errorf("%w; got %q", errFlagNameInvalid, f.Name))
	}
	if f.Shorthand != strings.ToLower(f.Shorthand) {
		validationErr = errors.Join(validationErr, fmt.Errorf("shorthand %q is not lowercase", f.Shorthand))
	} else if len(f.Shorthand) > 1 {
		validationErr = errors.Join(validationErr, fmt.Errorf("shorthand %q must be a single character", f.Shorthand))
	}
	if f.DisplayValue != strings.ToUpper(f.DisplayValue) {
		validationErr = errors.Join(validationErr, fmt.Errorf("display value %q is not uppercase", f.DisplayValue))
	}
	if !flagDescriptionRegex.MatchString(f.Description) {
		validationErr = errors.Join(validationErr, fmt.Errorf("%w; got %q", errFlagDescriptionInvalid, f.Description))
	}
	if f.Value == nil {
		validationErr = errors.Join(validationErr, fmt.Errorf("value cannot be nil"))
	}

	return validationErr
}

// validate validates the documentation section.
func (d *DocSection) validate() error {
	var validationErr error

	if d.Title == "" {
		return fmt.Errorf("title cannot be empty")
	} else if strings.HasSuffix(d.Title, ".") {
		return fmt.Errorf("title cannot end with a period")
	}
	if d.Documentation == "" {
		return fmt.Errorf("documentation cannot be empty")
	}

	return validationErr
}

// validate validates the positional arguments.
func (p *PositionalArguments) validate() error {
	var validationErr error

	// Start capital and end with a period if set.
	if p.Preamble != "" && !argsPreambleRegex.MatchString(p.Preamble) {
		return errArgsPreambleInvalid
	}

	l := len(p.Args)
	for i, p := range p.Args {
		if err := p.validate(i == l-1); err != nil {
			return fmt.Errorf("error validating positional argument %d: %w", i, err)
		}
	}

	return validationErr
}

// validate validates the positional argument. isLast indicates if the positional
// argument is the last argument.
func (a *PositionalArgument) validate(isLast bool) error {
	var validationErr error

	if a.Name == "" {
		validationErr = errors.Join(validationErr, fmt.Errorf("name cannot be empty"))
	} else if a.Name != strings.ToUpper(a.Name) {
		validationErr = errors.Join(validationErr, fmt.Errorf("name %q is not uppercase", a.Name))
	}

	if a.Documentation == "" {
		validationErr = errors.Join(validationErr, fmt.Errorf("documentation cannot be empty"))
	} else if !strings.HasSuffix(a.Documentation, ".") {
		validationErr = errors.Join(validationErr, fmt.Errorf("documentation must end with a period"))
	}

	if a.Optional && !isLast {
		validationErr = errors.Join(validationErr, fmt.Errorf("optional positional argument %q must be the last argument", a.Name))
	}
	if a.Repeatable && !isLast {
		validationErr = errors.Join(validationErr, fmt.Errorf("repeatable positional argument %q must be the last argument", a.Name))
	}

	return validationErr
}

// validate validates the example.
func (e *Example) validate() error {
	var validationErr error

	if e.Preamble == "" {
		validationErr = errors.Join(validationErr, fmt.Errorf("preamble cannot be empty"))
	} else if !examplePreambleRegex.MatchString(e.Preamble) {
		validationErr = errors.Join(validationErr, errExamplePreambleInvalid)
	}

	if e.Command == "" {
		validationErr = errors.Join(validationErr, fmt.Errorf("command cannot be empty"))
	} else if !strings.HasPrefix(e.Command, "$ ") || !strings.HasPrefix(e.Command, "#") {
		validationErr = errors.Join(validationErr, fmt.Errorf("example command must start with $ or #"))
	}

	return validationErr
}
