// Package main provides the tfcloud CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/brandonc/tfcloud/internal/command"
	"github.com/brandonc/tfcloud/internal/config"
	cli "github.com/hashicorp/cli"
	"github.com/mattn/go-isatty"
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if exitErr, ok := err.(command.ExitError); ok {
			os.Exit(exitErr.Code)
		}
	}
}

func realMain() error {
	stdoutIsTTY := isTerminal(os.Stdout)
	stderrIsTTY := isTerminal(os.Stderr)

	ui := &cli.BasicUi{
		Reader:      os.Stdin,
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}

	meta := &command.Meta{
		UI:          ui,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		StdoutIsTTY: stdoutIsTTY,
		StderrIsTTY: stderrIsTTY,
		HumanOutput: stdoutIsTTY,
	}

	c := cli.NewCLI(config.Name, config.Version)
	c.Args = os.Args[1:]
	c.HelpWriter = os.Stdout
	c.ErrorWriter = os.Stderr
	c.Commands = command.Commands(meta)

	exitCode, err := c.Run()
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return command.ExitError{Code: exitCode}
	}

	return nil
}

func isTerminal(f *os.File) bool {
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
