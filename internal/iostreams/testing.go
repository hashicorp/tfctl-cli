// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iostreams

import (
	"bytes"
	"fmt"
	"io"

	"github.com/muesli/termenv"
)

// Testing implements the IOStreams interface and provides programtic access to
// both input and output.
type Testing struct {
	// Input should be written to simulate Stdin.
	Input *bytes.Buffer

	// Output contains all data emitted to the Out stream.
	Output *bytes.Buffer

	// Error contains all data emitted to the Err stream.
	Error *bytes.Buffer

	// InputTTY, OutputTTY, and ErrorTTY control whether the testing IOStreams
	// treats each respective FD as a TTY.
	InputTTY  bool
	OutputTTY bool
	ErrorTTY  bool

	// profile is the terminal color profile to use.
	profile termenv.Profile

	// quiet stores if quiet mode has been enabled.
	quiet bool
}

// Test returns a new IOStreams for testing.
func Test() *Testing {
	t := &Testing{
		Input:   &bytes.Buffer{},
		Output:  &bytes.Buffer{},
		Error:   &bytes.Buffer{},
		profile: termenv.Ascii, // Default to no color
	}

	return t
}

func (t *Testing) In() io.Reader  { return t.Input }
func (t *Testing) Out() io.Writer { return t.Output }
func (t *Testing) Err() io.Writer {
	if t.quiet {
		return io.Discard
	}

	return t.Error
}

func (t *Testing) ColorEnabled() bool    { return false }
func (t *Testing) ForceNoColor()         {}
func (t *Testing) RestoreConsole() error { return nil }
func (t *Testing) SetQuiet(quiet bool)   { t.quiet = quiet }

func (t *Testing) ColorScheme() *ColorScheme {
	return &ColorScheme{profile: t.profile}
}

// ForcedColorProfile allows forcing a specific color profile for testing.
func (t *Testing) ForcedColorProfile(profile termenv.Profile) {
	t.profile = profile
}

func (t *Testing) ReadSecret() ([]byte, error) {
	if !t.CanPrompt() {
		return nil, fmt.Errorf("prompting is disabled")
	}

	var buf [1]byte
	var ret []byte

	for {
		n, err := t.Input.Read(buf[:])
		if n > 0 {
			switch buf[0] {
			case '\b':
				if len(ret) > 0 {
					ret = ret[:len(ret)-1]
				}
			case '\n', '\r':
				return ret, nil
			default:
				ret = append(ret, buf[0])
			}
			continue
		}
		if err != nil {
			if err == io.EOF && len(ret) > 0 {
				return ret, nil
			}
			return ret, err
		}
	}
}

// Prompt confirm for testing attempts to read a single byte from stdin. If the
// byte is y, true is returned, if it is n, false is returned. Any other value
// is an error
func (t *Testing) PromptConfirm(prompt string) (bool, error) {
	if !t.CanPrompt() {
		return false, fmt.Errorf("prompting is disabled")
	}

	// Output the prompt
	fmt.Fprintf(t.Error, "%s (y/n)? ", prompt)

	// Try to read a single byte from stdin.
	b, err := t.Input.ReadByte()
	if err != nil {
		return false, err
	}

	switch b {
	case byte('y'):
		return true, nil
	case byte('n'):
		return false, nil
	default:
		return false, fmt.Errorf("invalid character: %v", b)
	}
}

func (t *Testing) IsInputTTY() bool {
	return t.InputTTY
}

func (t *Testing) IsOutputTTY() bool {
	return t.OutputTTY
}

func (t *Testing) IsErrorTTY() bool {
	return t.ErrorTTY
}

func (t *Testing) CanPrompt() bool {
	return !t.quiet && t.IsErrorTTY() && t.IsInputTTY()
}

func (t *Testing) TerminalWidth() int {
	return TerminalDefaultWidth
}

// nopCloser wraps an io.Writer with a Close method
type nopCloser struct{ io.Writer }

// NopWriteCloser wraps an io.Writer with a Close method
func NopWriteCloser(w io.Writer) io.WriteCloser { return nopCloser{w} }
func (nopCloser) Close() error                  { return nil }

// Ensure we meet the interface
var _ IOStreams = &Testing{}
