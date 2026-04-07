package command

import (
	"fmt"
	"io"
	"log"

	cli "github.com/hashicorp/cli"
)

// Meta carries UI and stdio details that commands use while running.
type Meta struct {
	// UI is the command-line UI implementation.
	UI cli.Ui
	// Stdin is the command input stream.
	Stdin io.Reader
	// Stdout is the command output stream.
	Stdout io.Writer
	// Stderr is the command error stream.
	Stderr io.Writer
	// StdoutIsTTY reports whether Stdout is attached to a terminal.
	StdoutIsTTY bool
	// StderrIsTTY reports whether Stderr is attached to a terminal.
	StderrIsTTY bool
	// HumanOutput reports whether commands should prefer human-oriented rendering.
	HumanOutput bool
}

// ExitError represents a process exit code from the CLI.
type ExitError struct {
	// Code is the process exit status.
	Code int
}

// Error returns a message for the exit code.
func (e ExitError) Error() string {
	switch e.Code {
	case 0:
		return ""
	case 1:
		return "invalid command"
	case 2:
		return "request error"
	case 3:
		return "server error"
	default:
		log.Printf("Exit code %d should be added to the ExitError description", e.Code)
		return fmt.Sprintf("command failed (code %d)", e.Code)
	}
}

// Commands returns the CLI command registry.
func Commands(meta *Meta) map[string]cli.CommandFactory {
	return map[string]cli.CommandFactory{
		"api": func() (cli.Command, error) {
			return &APICommand{Meta: meta}, nil
		},
		"api schema": func() (cli.Command, error) {
			return &APISchemaCommand{Meta: meta}, nil
		},
		"workspace": func() (cli.Command, error) {
			return &NamespaceCommand{Meta: meta, Name: "workspace"}, nil
		},
		"variable": func() (cli.Command, error) {
			return &NamespaceCommand{Meta: meta, Name: "variable"}, nil
		},
		"variable import": func() (cli.Command, error) {
			return &VariableImportCommand{Meta: meta}, nil
		},
	}
}

// NamespaceCommand groups related subcommands under a shared namespace.
type NamespaceCommand struct {
	// Meta provides UI and stream access for command execution.
	Meta *Meta
	// Name is the namespace name shown in help and synopsis output.
	Name string
}

// Help returns the command help text.
func (c *NamespaceCommand) Help() string {
	if c.Name == "workspace" {
		return "Usage: tfcloud workspace <subcommand>\n\n  vcs    Create or update workspace VCS settings"
	}

	return "Usage: tfcloud variable <subcommand>\n\n  import    Import tfvars or environment variables"
}

// Run executes the namespace command.
func (c *NamespaceCommand) Run(args []string) int {
	if len(args) > 0 {
		c.Meta.UI.Error("unknown subcommand: " + args[0])
	}
	return cli.RunResultHelp
}

// Synopsis returns a short summary of the command.
func (c *NamespaceCommand) Synopsis() string {
	if c.Name == "workspace" {
		return "Workspace workflows"
	}
	return "Variable workflows"
}

func parseSingleArg(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("accepts 1 argument, but got %d. Try using -help", len(args))
	}
	return args[0], nil
}
