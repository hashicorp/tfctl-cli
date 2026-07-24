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
	"slices"
	"syscall"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/commands/profile/profiles"
	"github.com/hashicorp/tfctl-cli/internal/commands/root"
	"github.com/hashicorp/tfctl-cli/internal/pkg/checkpoint"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/internal/pkg/telemetry"
	"github.com/hashicorp/tfctl-cli/version"
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

	// Explore relevant global args before the command parses them to set up non-command output
	initialLogLevel := logging.LevelDefault
	for _, a := range args {
		if a == "--debug" {
			initialLogLevel = logging.LevelDebug
		}
		if a == "--no-color" {
			io.ForceNoColor()
		}
		if a == "--quiet" {
			io.SetQuiet(true)
		}
	}

	// The actual logger level will be set by the command after parsing flags.
	logger := logging.NewLogger(io, initialLogLevel)

	// Add the logger to the main context for use everywhere else.
	shutdownCtx = logging.WithLogger(shutdownCtx, logger)

	// Checkpoint is HashiCorp's service for checking the current version against the
	// latest, providing any relevant warnings about the current release in rare situations.
	// Run the request in a separate goroutine. It's important to always execute
	// this without condition because checkForNewVersion will block until it is complete
	go checkpoint.Run(shutdownCtx, os.Getenv("CHECKPOINT_DISABLE") != "")

	// Create the profile loader and load the active profile.
	loader, err := profile.NewLoader()
	if err != nil {
		fmt.Fprintln(io.Err(), err)
		return 1
	}

	activeProfile, err := loadActiveProfile(shutdownCtx, loader)
	if err != nil {
		fmt.Fprintln(io.Err(), err)
		return 1
	}

	// If the profile has disabled color, disable on the iostream.
	if activeProfile != nil && activeProfile.NoColor != nil && *activeProfile.NoColor {
		io.ForceNoColor()
	}

	// Initialize telemetry
	var profileTelemetry string
	if activeProfile != nil {
		profileTelemetry = activeProfile.GetTelemetry()
	}

	tel := telemetry.Init(shutdownCtx, telemetry.Config{
		DeviceID:         loader.GetDeviceID(shutdownCtx),
		Hostname:         activeProfile.GetHostname(),
		ProfileTelemetry: profileTelemetry,
		Version:          version.Version,
		ErrWriter:        io.ErrUnessential(),
		IsTTY:            io.IsOutputTTY(),
	})

	shutdownCtx = telemetry.WithTelemetry(shutdownCtx, tel)

	// Create the command invocation
	inv := &cmd.Invocation{
		IO:          io,
		Profile:     activeProfile,
		Output:      format.New(io),
		ShutdownCtx: shutdownCtx,
	}

	// Get the HCP Root command
	tfctlCmd := root.NewCmdRoot(inv)
	cmdMap := cmd.ToCommandMap(tfctlCmd, inv)

	c := cli.CLI{
		Version:                    version.Version,
		Name:                       version.Name,
		Args:                       args,
		Commands:                   cmdMap,
		HelpFunc:                   cmd.RootHelpFunc(tfctlCmd),
		Autocomplete:               true,
		AutocompleteNoDefaultFlags: true,
		AutocompleteGlobalFlags: map[string]complete.Predictor{
			"--help":     complete.PredictNothing,
			"--version":  complete.PredictNothing,
			"--debug":    complete.PredictAnything,
			"--jq":       complete.PredictAnything,
			"--json":     complete.PredictAnything,
			"--markdown": complete.PredictAnything,
			"--no-color": complete.PredictAnything,
			"--profile":  profiles.PredictProfiles(false, true),
			"--quiet":    complete.PredictAnything,
		},
	}

	// Show the banner instead of running the command under certain conditions
	if bannerShown := maybeShowBanner(&c, inv); bannerShown {
		checkForNewVersion(io)
		return 0
	}

	status, err := c.Run()
	if err != nil {
		fmt.Fprintf(io.Err(), "Error executing %s: %s\n", version.Name, err.Error())
	}

	// Check for new version or outdated skill if --version is specified on any successful command
	if status == 0 && c.IsVersion() {
		if !checkForNewVersion(io) {
			// Only recommend installing the skill if the current version is not outdated.
			root.RunDetectOutdatedSkill(io)
		}
	}

	// Don't worry about telemetry errors at all
	if err = tel.Shutdown(shutdownCtx, status); err != nil {
		logger.Debug("Error occurred while shutting down telemetry", "error", err)
	}

	return status
}

func maybeShowBanner(c *cli.CLI, inv *cmd.Invocation) bool {
	globalFlagsAllowedForBanner := []string{"--debug", "--no-color", "--quiet"}
	for _, arg := range c.Args {
		if !slices.Contains(globalFlagsAllowedForBanner, arg) {
			return false
		}
	}

	// If the user is running the root command, without --help or --version
	// show the banner and exit.
	if !c.IsVersion() && !c.IsHelp() {
		root.RunBanner(inv.ShutdownCtx, &root.BannerOpts{
			IO:              inv.IO,
			TokenConfigured: inv.Profile != nil && inv.Profile.GetToken() != "",
		})
		return true
	}
	return false
}

func checkForNewVersion(io iostreams.IOStreams) bool {
	cs := io.ColorScheme()
	versionInfo := checkpoint.WaitForVersionCheck()
	if versionInfo.Outdated {
		fmt.Fprintf(io.ErrUnessential(), "A new version of %s is available: %s\n", version.Name, cs.String(fmt.Sprintf("v%s", versionInfo.Latest)).Color(cs.Purple()).Bold())
	}
	if len(versionInfo.Alerts) > 0 {
		fmt.Fprintln(io.ErrUnessential(), "")
		fmt.Fprintf(io.ErrUnessential(), "%s: %s\n", cs.WarningLabel(), "There are alerts regarding your current version.")
		for _, alert := range versionInfo.Alerts {
			fmt.Fprintln(io.ErrUnessential(), heredoc.New(io, heredoc.WithNoWrap()).Mustf(" - %s", alert))
		}
	}
	return versionInfo.Outdated
}

// loadActiveProfile loads the active profile.
func loadActiveProfile(ctx context.Context, loader *profile.Loader) (*profile.Profile, error) {
	// Load the active profile
	activeProfile, err := loader.GetActiveProfile()
	if err != nil {
		if !errors.Is(err, profile.ErrNoActiveProfileFilePresent) && !errors.Is(err, profile.ErrActiveProfileFileEmpty) {
			return nil, fmt.Errorf("failed to read active profile: %w", err)
		}

		if err := loader.DefaultActiveProfile().Write(); err != nil {
			return nil, fmt.Errorf("failed to save default active profile config: %w", err)
		}

		if err := loader.DefaultProfile(ctx).Write(); err != nil {
			return nil, fmt.Errorf("failed to save default profile config: %w", err)
		}

		activeProfile, err = loader.GetActiveProfile()
		if err != nil {
			return nil, fmt.Errorf("failed to save default active profile config: %w", err)
		}
	}

	return loader.LoadProfile(ctx, activeProfile.Name)
}

// IsAutocomplete returns true if the CLI is being run in an autocomplete
// context.
func IsAutocomplete() bool {
	return os.Getenv("COMP_LINE") != "" && os.Getenv("COMP_POINT") != ""
}
