// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

// Package iostreams provides access to the terminal outputs and inputs in a
// centralized and mockable fashion. All terminal access should happen using
// this package.
package iostreams

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

const (
	// TerminalDefaultWidth is the default width of the terminal if the width
	// cannot be determined.
	TerminalDefaultWidth = 80
)

// IOStreams is an interface for interacting with IO and general terminal
// output. Commands should not directly interact with os.Stdout/Stderr/Stdin but
// utilize the passed IOStreams.
type IOStreams interface {
	// In returns an io.Reader for reading input
	In() io.Reader

	// Out returns an io.Writer for outputting non-error output
	Out() io.Writer

	// Err returns an io.Writer for outputting error output
	Err() io.Writer

	// ColorEnabled returns if color is enabled.
	ColorEnabled() bool

	// ForceNoColor forces no color output
	ForceNoColor()

	// ColorScheme returns a ColorScheme for coloring and formatting output.
	ColorScheme() *ColorScheme

	// RestoreConsole should be called the console to its original state. This
	// should be called in a defer method after retrieving an IOStream.
	RestoreConsole() error

	// ReadSecret reads a line of input without local echo. The returned data
	// does not include the \n delimeter.
	ReadSecret() ([]byte, error)

	// PromptConfirm prompts for a confirmation from the user.
	PromptConfirm(prompt string) (bool, error)

	// IsInputTTY returns whether the input is a TTY
	IsInputTTY() bool

	// IsOutputTTY returns whether the output is a TTY
	IsOutputTTY() bool

	// IsErrorTTY returns whether the error output is a TTY
	IsErrorTTY() bool

	// TerminalWidth returns the width of the terminal that controls the process
	TerminalWidth() int

	// CanPrompt returns true if prompting is available. Both the input and
	// error output must be a TTY for this to return true as well as SetQuiet
	// must not be true.
	CanPrompt() bool

	// SetQuiet updates the iostream to disable Err output and prompting.
	SetQuiet(quiet bool)
}

// system is the IOStreams for interacting with an actual terminal.
type system struct {
	in  *os.File
	out *termenv.Output
	err *termenv.Output

	// Capture the restore functions
	restoreConsoleFn []func() error

	// forceNoColor forces no color output
	forceNoColor bool

	// quiet stores if quiet mode has been enabled.
	quiet bool

	// ctx is used to cancel any blocking read in the event the context is
	// cancelled.
	ctx context.Context
}

// System returns an IOStreams meant to interact with the systems terminal.
func System(ctx context.Context) (IOStreams, error) {
	io := &system{
		in:  os.Stdin,
		out: termenv.NewOutput(os.Stdout),
		err: termenv.NewOutput(os.Stderr),
		ctx: ctx,
	}

	restoreOut, err := termenv.EnableVirtualTerminalProcessing(io.out)
	if err != nil {
		return nil, fmt.Errorf("failed to enable virtual terminal processing: %w", err)
	}

	restoreErr, err := termenv.EnableVirtualTerminalProcessing(io.err)
	if err != nil {
		return nil, fmt.Errorf("failed to enable virtual terminal processing: %w", err)
	}
	io.restoreConsoleFn = []func() error{restoreOut, restoreErr}

	return io, nil
}

func (s *system) In() io.Reader {
	return s.in
}

func (s *system) Out() io.Writer {
	return s.out
}

func (s *system) Err() io.Writer {
	if s.quiet {
		return io.Discard
	}

	return s.err
}

func (s *system) SetQuiet(quiet bool) {
	s.quiet = quiet
}

func (s *system) ColorEnabled() bool {
	if s.forceNoColor {
		return false
	}

	return !s.out.EnvNoColor()
}

func (s *system) ForceNoColor() {
	s.forceNoColor = true
}

func (s *system) ColorScheme() *ColorScheme {
	if !s.ColorEnabled() {
		return &ColorScheme{profile: termenv.Ascii}
	}
	return &ColorScheme{profile: s.out.EnvColorProfile()}
}

func (s *system) ReadSecret() ([]byte, error) {
	if !s.CanPrompt() {
		return nil, fmt.Errorf("prompting is disabled")
	}

	fd := int(s.in.Fd())

	// Store and restore the terminal status on interruptions to
	// avoid that the terminal remains in the password state
	// This is necessary as for https://github.com/golang/go/issues/31180
	oldState, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}

	type Buffer struct {
		Buffer []byte
		Error  error
	}
	errorChannel := make(chan Buffer, 1)
	doneChannel := make(chan struct{})
	defer close(doneChannel)

	// Cancelled context restores the terminal, otherwise the no-echo mode would remain intact
	go func() {
		select {
		case <-doneChannel:
			return
		case <-s.ctx.Done():
		}

		if oldState != nil {
			_ = term.Restore(fd, oldState)
		}
		errorChannel <- Buffer{Buffer: make([]byte, 0), Error: context.Cause(s.ctx)}
	}()

	go func() {
		buf, err := term.ReadPassword(fd)
		errorChannel <- Buffer{Buffer: buf, Error: err}
	}()

	buf := <-errorChannel

	return buf.Buffer, buf.Error
}

func (s *system) PromptConfirm(prompt string) (confirmed bool, err error) {
	if !s.CanPrompt() {
		return false, fmt.Errorf("prompting is disabled")
	}

	// Prompt
	fmt.Fprintf(s.err, "%s (y/n)? ", prompt)

	// Read the input in a goroutine so we can handle the command be signaled
	// or STDIN being closed.
	doneCh := make(chan bool, 1)
	go func() {
		defer close(doneCh)
		r := bufio.NewReader(s.in)
		for {
			read, readErr := r.ReadString('\n')
			if readErr != nil {
				err = readErr
				return
			}

			read = strings.ToLower(strings.TrimSpace(read))
			switch read {
			case "y", "yes":
				confirmed = true
				fmt.Fprintln(s.err)
				return
			case "n", "no":
				confirmed = false
				fmt.Fprintln(s.err)
				return
			default:
				fmt.Fprint(s.err, "Please enter 'y' or 'n': ")
			}
		}
	}()

	select {
	case <-s.ctx.Done():
		// If we are cancelled, try to extract the reason.
		if cause := context.Cause(s.ctx); cause != nil {
			err = cause
		} else {
			err = s.ctx.Err()
		}
	case <-doneCh:
	}

	if err != nil {
		// Print new lines to separate the error from any user input.
		fmt.Fprintln(s.err)
		fmt.Fprintln(s.err)
		return false, err
	}

	return confirmed, nil
}

func (s *system) IsInputTTY() bool {
	return term.IsTerminal(int(s.in.Fd()))
}

func (s *system) IsOutputTTY() bool {
	return term.IsTerminal(int(s.out.TTY().Fd()))
}

func (s *system) IsErrorTTY() bool {
	return term.IsTerminal(int(s.err.TTY().Fd()))
}

func (s *system) CanPrompt() bool {
	return !s.quiet && s.IsErrorTTY() && s.IsInputTTY()
}

// TerminalWidth returns the width of the terminal that controls the process
func (s *system) TerminalWidth() int {
	if !s.IsOutputTTY() {
		return TerminalDefaultWidth
	}

	width, _, err := term.GetSize(int(s.out.TTY().Fd()))
	if err != nil {
		return TerminalDefaultWidth
	}

	return width
}

func (s *system) RestoreConsole() error {
	var returnErr error
	for _, f := range s.restoreConsoleFn {
		if err := f(); err != nil {
			returnErr = err
		}
	}
	return returnErr
}
