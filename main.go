// Package main provides the tfcloud CLI entrypoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"

	"github.com/hashicorp/tfcloud/internal/cmd"
	"github.com/hashicorp/tfcloud/internal/command"
	"github.com/hashicorp/tfcloud/internal/config"
	"github.com/hashicorp/tfcloud/internal/format"
	"github.com/hashicorp/tfcloud/internal/iostreams"
	"github.com/hashicorp/tfcloud/internal/profile"
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	args := os.Args[1:]

	// Listen for interrupts
	shutdownCtx, shutdown := context.WithCancelCause(context.Background())
	defer shutdown(nil)
	go func() {
		signalCh := make(chan os.Signal, 1)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
		sig := <-signalCh
		shutdown(fmt.Errorf("command received signal: %s", sig))
	}()

	// Create our iostreams
	io, err := iostreams.System(shutdownCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure iostreams: %v\n", err)
		return 1
	}
	defer func() {
		if err := io.RestoreConsole(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to restore console output: %v\n", err)
		}
	}()

	// TODO: check version for updates?

	activeProfile, err := loadProfile(shutdownCtx)
	if err != nil {
		fmt.Fprintln(io.Err(), err)
		return 1
	}

	// If the profile has disabled color, disable on the iostream.
	if activeProfile != nil && activeProfile.NoColor != nil && *activeProfile.NoColor {
		io.ForceNoColor()
	}

	// Create the command context
	cCtx := &cmd.Context{
		IO:          io,
		Profile:     activeProfile,
		Output:      format.New(io),
		ShutdownCtx: shutdownCtx,
	}

	// Get the HCP Root command
	tfcloudCmd := NewCmdRoot(cCtx)
	cmdMap := cmd.ToCommandMap(tfcloudCmd)

	c := cli.CLI{
		Version:                    config.Version,
		Name:                       config.Name,
		Args:                       args,
		Commands:                   cmdMap,
		HelpFunc:                   cmd.RootHelpFunc(tfcloudCmd),
		Autocomplete:               true,
		AutocompleteNoDefaultFlags: true,
		AutocompleteGlobalFlags: map[string]complete.Predictor{
			"--help":    complete.PredictNothing,
			"--version": complete.PredictNothing,
			"--json":    complete.PredictAnything,
			"--quiet":   complete.PredictAnything,
			"--agent":   complete.PredictAnything,
		},
	}

	status, err := c.Run()
	if err != nil {
		fmt.Fprintf(io.Err(), "Error executing tfcloud: %s\n", err.Error())
	}

	return status
}

// loadActiveProfile loads the active profile.
func loadActiveProfile() (*profile.Profile, error) {
	// Create the profile loader
	loader, err := profile.NewLoader()
	if err != nil {
		return nil, fmt.Errorf("failed to create profile loader: %w", err)
	}

	// Load the active profile
	activeProfile, err := loader.GetActiveProfile()
	if err != nil {
		if !errors.Is(err, profile.ErrNoActiveProfileFilePresent) && !errors.Is(err, profile.ErrActiveProfileFileEmpty) {
			return nil, fmt.Errorf("failed to read active profile: %w", err)
		}

		if err := loader.DefaultActiveProfile().Write(); err != nil {
			return nil, fmt.Errorf("failed to save default active profile config: %w", err)
		}

		if err := loader.DefaultProfile().Write(); err != nil {
			return nil, fmt.Errorf("failed to save default profile config: %w", err)
		}

		activeProfile, err = loader.GetActiveProfile()
		if err != nil {
			return nil, fmt.Errorf("failed to save default active profile config: %w", err)
		}
	}

	return loader.LoadProfile(activeProfile.Name)
}

// loadProfile loads the active profile and if one doesn't exist, a default
// profile is created.
func loadProfile(_ context.Context) (*profile.Profile, error) {
	// Get the active profile
	p, err := loadActiveProfile()
	if err != nil {
		return nil, err
	}

	// Save the profile.
	if err := p.Write(); err != nil {
		return nil, fmt.Errorf("failed to save default profile: %w", err)
	}

	return p, nil
}

func NewCmdRoot(ctx *cmd.Context) *cmd.Command {
	c := &cmd.Command{
		Name:      config.Name,
		ShortHelp: "Interact with HCP Terraform and Terraform Enterprise.",
		LongHelp:  "The tfcloud command-line interface (CLI) is a unified tool to managing HCP Terraform and Terraform Enterpise from the command line.",
	}

	//  _   _  ___ _____ _____
	// | \ | |/ _ \_   _| ____|
	// |  \| | | | || | |  _|
	// | |\  | |_| || | | |___
	// |_| \_|\___/ |_| |_____|
	//
	// When adding a top level command group, be sure to regenerate the
	// screenshot in the README by running `make gen/screenshot`.

	// Add the subcommands
	c.AddChild(command.NewCmdAPI(ctx))
	c.AddChild(command.NewCmdVariable(ctx))

	// Configure the command as the root command.
	cmd.ConfigureRootCommand(ctx, c)

	return c
}

// IsAutocomplete returns true if the CLI is being run in an autocomplete
// context.
func IsAutocomplete() bool {
	return os.Getenv("COMP_LINE") != "" && os.Getenv("COMP_POINT") != ""
}
