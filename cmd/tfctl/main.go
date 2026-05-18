// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package main provides the tfctl CLI entrypoint.
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

	"github.com/hashicorp/tfctl-cli/internal/commands/profile/profiles"
	"github.com/hashicorp/tfctl-cli/internal/commands/root"
	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
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
	tfctlCmd := root.NewCmdRoot(cCtx)
	cmdMap := cmd.ToCommandMap(tfctlCmd, cCtx)

	c := cli.CLI{
		Version:                    config.Version,
		Name:                       config.Name,
		Args:                       args,
		Commands:                   cmdMap,
		HelpFunc:                   cmd.RootHelpFunc(tfctlCmd),
		Autocomplete:               true,
		AutocompleteNoDefaultFlags: true,
		AutocompleteGlobalFlags: map[string]complete.Predictor{
			"--help":     complete.PredictNothing,
			"--version":  complete.PredictNothing,
			"--agent":    complete.PredictAnything,
			"--debug":    complete.PredictAnything,
			"--jq":       complete.PredictAnything,
			"--json":     complete.PredictAnything,
			"--markdown": complete.PredictAnything,
			"--no-color": complete.PredictAnything,
			"--profile":  profiles.PredictProfiles(false, true),
			"--quiet":    complete.PredictAnything,
		},
	}

	status, err := c.Run()
	if err != nil {
		fmt.Fprintf(io.Err(), "Error executing %s: %s\n", config.Name, err.Error())
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

	return p, nil
}

// IsAutocomplete returns true if the CLI is being run in an autocomplete
// context.
func IsAutocomplete() bool {
	return os.Getenv("COMP_LINE") != "" && os.Getenv("COMP_POINT") != ""
}
