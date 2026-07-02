// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/execsession"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/version"
)

// ExecOpts defines the options for the `harness exec` command.
type ExecOpts struct {
	IO     iostreams.IOStreams
	DryRun bool

	// AllowDelete holds the raw --allow-delete values (repeatable + CSV).
	AllowDelete []string

	// Argv is the child command and its arguments (everything after `--`).
	Argv []string

	// Injectable seams for tests.
	Store *execsession.Store
	PID   int
	Run   func(ctx context.Context, argv, env []string, io iostreams.IOStreams) (int, error)
}

// NewCmdHarnessExec creates the `harness exec` command.
func NewCmdHarnessExec(inv *cmd.Invocation) *cmd.Command {
	execOpts := ExecOpts{
		IO:  inv.IO,
		PID: os.Getpid(),
		Run: realRunner,
	}

	command := &cmd.Command{
		Name:      "exec",
		ShortHelp: "Run a command with session-scoped tfctl permissions.",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s harness exec" }} command runs a child command (such as a coding agent) with a short-lived, session-scoped permission that lets nested {{ Bold "tfctl" }} invocations perform noninteractive deletes.

		This is a deliberate, per-session opt-in by a human. The permission is tied to the lifetime of this process and {{ Bold "auto-reverts" }} to the safe default (deletes require an interactive confirmation) as soon as the child exits.

		This is a {{ Bold "safety rail, not a security boundary" }}: the child runs as the same OS user, so a true guarantee that an agent cannot delete must come from the API token scope server-side.

		Use {{ template "mdCodeOrBold" "--allow-delete" }} to name the resource classes that may be deleted noninteractively. Repeat the flag or pass a comma-separated list. The special tokens {{ template "mdCodeOrBold" "reversible" }} and {{ template "mdCodeOrBold" "all" }} cover any reversible class, but {{ Bold "never" }} cover the irreversible classes {{ template "mdCodeOrBold" "organizations" }} and {{ template "mdCodeOrBold" "projects" }} — those must always be named explicitly.

		The child command and its arguments must follow a {{ template "mdCodeOrBold" "--" }} separator.
		`, version.Name),
		Examples: []cmd.Example{
			{
				Preamble: "Allow an agent to delete workspaces and runs for one session:",
				Command:  "$ tfctl harness exec --allow-delete=workspaces,runs -- opencode",
			},
			{
				Preamble: "Explicitly allow deleting projects (an irreversible class):",
				Command:  "$ tfctl harness exec --allow-delete=projects -- ./ci-script.sh",
			},
		},
		NoAuthRequired: true,
		Args: cmd.PositionalArguments{
			// The child command + args are passed through verbatim; bypass count
			// validation and enforce "at least one" inside runExec so we can emit
			// a helpful usage message.
			//
			// No Autocomplete is offered here: only the first token is an
			// executable, while everything after it is the child's own
			// arguments/flags, which we cannot meaningfully predict. Completing
			// those tokens as files or executables would mislead, so we predict
			// nothing rather than predict wrongly.
			Validate: cmd.ArbitraryArgs,
			Args: []cmd.PositionalArgument{
				{
					Name:          "COMMAND",
					Documentation: "The command to run (after a `--` separator), plus any arguments.",
					Repeatable:    true,
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:         "allow-delete",
					DisplayValue: "CLASSES",
					Description:  "Resource classes that nested tfctl may delete noninteractively (repeatable, CSV). Special tokens: reversible, all. organizations/projects must be named explicitly.",
					Repeatable:   true,
					Value:        flagvalue.SimpleSlice(nil, &execOpts.AllowDelete),
					Autocomplete: complete.PredictSet(execsession.AllowDeleteCompletions()...),
				},
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			execOpts.Argv = args
			if inv.IsDryRun() {
				execOpts.DryRun = true
			}
			if execOpts.Store == nil {
				store, err := execsession.DefaultStore()
				if err != nil {
					return fmt.Errorf("failed to initialize exec session store: %w", err)
				}
				execOpts.Store = store
			}
			return runExec(inv.ShutdownCtx, &execOpts)
		},
	}

	return command
}

func runExec(ctx context.Context, opts *ExecOpts) error {
	logger := logging.FromContext(ctx)
	cs := opts.IO.ColorScheme()

	if len(opts.Argv) == 0 {
		return errors.New("no command to run; usage: tfctl harness exec [--allow-delete=CLASSES] -- <command> [args...]")
	}

	perms, warnings := execsession.NormalizeAllowDelete(opts.AllowDelete)
	for _, w := range warnings {
		fmt.Fprintf(opts.IO.Err(), "%s %s\n", cs.WarningLabel(), w)
	}

	if opts.DryRun {
		fmt.Fprintf(opts.IO.Err(), "%s would create exec session (allow-delete=%v) and run: %s\n",
			cs.DryRunLabel(), perms, strings.Join(opts.Argv, " "))
		return nil
	}

	handle, err := opts.Store.Create(execsession.Permissions{AllowDelete: perms}, opts.PID)
	if err != nil {
		return fmt.Errorf("failed to create exec session: %w", err)
	}
	defer func() {
		if cerr := handle.Close(); cerr != nil {
			logger.Debug("failed to clean up exec session", "error", cerr)
		}
	}()

	logger.Debug("exec session created", "allow_delete", perms)
	fmt.Fprintf(opts.IO.Err(), "%s tfctl deletes enabled for this session: %v\n", cs.WarningLabel(), perms)

	env := append(os.Environ(), execsession.EnvVar+"="+handle.Token())
	code, runErr := opts.Run(ctx, opts.Argv, env, opts.IO)
	if runErr != nil {
		return fmt.Errorf("failed to run %q: %w", opts.Argv[0], runErr)
	}
	logger.Debug("child command exited", "command", opts.Argv[0], "code", code)
	if code != 0 {
		return cmd.NewExitError(code, nil)
	}
	return nil
}

// realRunner runs the child command with real terminal passthrough so
// interactive agents work. It returns the child's exit code.
func realRunner(ctx context.Context, argv, env []string, _ iostreams.IOStreams) (int, error) {
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Env = env
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Start(); err != nil {
		return 0, err
	}

	err := c.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}
