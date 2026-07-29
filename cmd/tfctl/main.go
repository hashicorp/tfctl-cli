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
	"strings"
	"syscall"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/commands/profile/profiles"
	"github.com/hashicorp/tfctl-cli/internal/commands/root"
	"github.com/hashicorp/tfctl-cli/internal/pkg/checkpoint"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/internal/pkg/telemetry"
	"github.com/hashicorp/tfctl-cli/skills"
	"github.com/hashicorp/tfctl-cli/version"
)

var (
	envSkipMigrate       = "TFCTL_SKIP_MIGRATE"
	envCheckpointDisable = "CHECKPOINT_DISABLE"
)

func main() {
	os.Exit(realMain())
}

func isGlobalBooleanArg(arg string, bareForm string) bool {
	return arg == bareForm || arg == fmt.Sprintf("%s=true", bareForm)
}

func isDryRun(args []string) bool {
	for _, a := range args {
		if isGlobalBooleanArg(a, "--dry-run") {
			return true
		}
	}
	return false
}

func isVersion(args []string) bool {
	if len(args) == 0 {
		return true
	}

	allowVersionCommand := true
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// A non-flag argument before version flags indicates that this is not a version request.
			return false
		}

		if !isGlobalBooleanArg(arg, "--no-color") && !isGlobalBooleanArg(arg, "--debug") && !isGlobalBooleanArg(arg, "--quiet") {
			allowVersionCommand = false
		}

		// Any of these coming first indicate a version command
		if arg == "-v" || arg == "-version" || arg == "--version" {
			return true
		}
	}
	return allowVersionCommand
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
		if isGlobalBooleanArg(a, "--debug") {
			initialLogLevel = logging.LevelDebug
		}
		if isGlobalBooleanArg(a, "--no-color") {
			io.ForceNoColor()
		}
		if isGlobalBooleanArg(a, "--quiet") {
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
	go checkpoint.Run(shutdownCtx, os.Getenv(envCheckpointDisable) != "")

	// Conditionally begin migrating any existing skills that match an older version to the embedded version.
	var migration *skills.Migration
	if !isDryRun(args) && os.Getenv(envSkipMigrate) == "" {
		migration = skills.StartMigration(shutdownCtx)
	}

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
			"--dry-run":  complete.PredictAnything,
		},
	}

	// Override the hashicorp/cli behavior of `tfctl --version` by rewriting the arguments to invoke the
	// hidden "version" command. It's important not to call c.IsVersion() here because that would
	// init the args, making overwriting them ineffective.
	if isVersion(c.Args) || len(c.Args) == 0 {
		newArgs := []string{"version"}
		for _, arg := range c.Args {
			// Strip all the possible version flags from the arguments
			if arg != "--version" && arg != "-version" && arg != "-v" {
				newArgs = append(newArgs, arg)
			}
		}
		c.Args = newArgs
	}

	status, err := c.Run()
	if err != nil {
		fmt.Fprintf(io.Err(), "Error executing %s: %s\n", version.Name, err.Error())
	}

	shutdownMain(shutdownCtx, status, migration)

	return status
}

func shutdownMain(ctx context.Context, exitCode int, migration *skills.Migration) {
	logger := logging.FromContext(ctx)
	tel := telemetry.FromContext(ctx)

	// Wait for any ongoing skill migrations to complete
	if migration != nil {
		migrationResults, err := migration.Wait(ctx)
		if err != nil {
			logger.Debug("Skipped skill migration", "error", err)
		}

		for _, result := range migrationResults {
			if result.FailedReason != nil {
				logger.Error("Failed to migrate skill", "path", result.SkillPath, "reason", result.FailedReason.Error())
			} else {
				logger.Debug("Migrated skill", "path", result.SkillPath, "from", result.PreviousVersion)
			}
		}
	}

	// Don't worry about telemetry errors at all
	if err := tel.Shutdown(ctx, exitCode); err != nil {
		logger.Debug("Error occurred while shutting down telemetry", "error", err)
	}
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
