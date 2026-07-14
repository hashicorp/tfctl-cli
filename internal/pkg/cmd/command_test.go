// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"fmt"
	"net"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/cli"
	"github.com/hashicorp/go-tfe/v2"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestCommand_PersistentPrerun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create the command tree
	io := iostreams.Test()
	cCtx := &Invocation{
		IO: io,
	}
	root := &Command{
		Name: "root",
		io:   io,
	}
	child := &Command{
		Name: "child",
		RunF: func(c *Command, args []string) error {
			return nil
		},
	}
	childContainer := &Command{Name: "child-group"}
	grandchild := &Command{
		Name: "grandchild",
		RunF: func(c *Command, args []string) error {
			return nil
		},
	}
	root.AddChild(child)
	root.AddChild(childContainer)
	childContainer.AddChild(grandchild)

	// Add the persistent preruns
	rootPreRunCount := 0
	containerPreRunCount := 0
	root.PersistentPreRun = func(c *Command, args []string) error {
		rootPreRunCount++
		return nil
	}
	childContainer.PersistentPreRun = func(c *Command, args []string) error {
		containerPreRunCount++
		return nil
	}

	// Run the grandchild and the child
	r.Zero(grandchild.Run(nil, cCtx))
	r.Zero(child.Run(nil, cCtx))

	// Expect the prerun commmands were called
	r.Equal(2, rootPreRunCount)
	r.Equal(1, containerPreRunCount)
}

func TestCommand_Flags(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create the command tree
	io := iostreams.Test()
	cCtx := &Invocation{
		IO: io,
	}
	root := &Command{
		Name: "root",
		io:   io,
	}
	rootFlag := root.persistentFlags().String("root-flag", "", "testing")

	seenFlags := 0
	child := &Command{
		Name: "child",
		RunF: func(c *Command, args []string) error {
			c.allFlags().VisitAll(func(_ *pflag.Flag) {
				seenFlags++
			})
			return nil
		},
	}
	root.AddChild(child)
	childFlag := child.allFlags().String("child-flag", "", "testing")

	r.Zero(child.Run([]string{"--root-flag=root-set", "--child-flag=child-set"}, cCtx))
	r.Equal(2, seenFlags)
	r.Equal("root-set", *rootFlag)
	r.Equal("child-set", *childFlag)
}

func TestCommand_ExitCode(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create the command tree
	io := iostreams.Test()
	cCtx := &Invocation{
		IO: io,
	}
	code := 42
	err := fmt.Errorf("bad bad bad")
	root := &Command{
		Name: "root",
		io:   io,
		RunF: func(c *Command, args []string) error {
			return NewExitError(code, err)
		},
	}
	r.Equal(code, root.Run([]string{}, cCtx))
	r.Contains(io.Error.String(), err.Error())
}

func TestCommand_GlobalExitCode(t *testing.T) {
	t.Parallel()

	opErr := &net.OpError{Err: fmt.Errorf("some network error")}

	tests := []struct {
		err         error
		expected    int
		errContains string
	}{
		{err: ErrDisplayHelp, expected: cli.RunResultHelp},
		{err: ErrDisplayUsage, expected: 1},
		{err: tfe.ErrNotFound, expected: 2, errContains: "Resource or path not found on app.test.terraform.io. Verify the API path and any resource IDs are correct."},
		{err: tfe.ErrUnauthorized, expected: 3, errContains: "tfctl auth login"},
		{err: opErr, expected: 4, errContains: "network error"},
		{err: tfe.ErrInternalServer, expected: 5, errContains: "Internal Server Error"},
		{err: fmt.Errorf("some other error"), expected: 1, errContains: "ERROR: some other error"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("err %T exits with code %d", tt.err, tt.expected), func(t *testing.T) {
			r := require.New(t)

			// Create the command tree
			io := iostreams.Test()
			cCtx := &Invocation{
				IO:      io,
				Profile: profile.TestProfile(t),
			}
			root := &Command{
				Name: "root",
				io:   io,
				RunF: func(c *Command, args []string) error {
					return tt.err
				},
			}
			r.Equal(tt.expected, root.Run([]string{}, cCtx))
			if tt.errContains != "" {
				r.Contains(io.Error.String(), tt.errContains)
			}
		})
	}
}

func TestCommand_CommandPath(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	root := &Command{Name: "tfctl"}
	run := &Command{Name: "run"}
	start := &Command{Name: "start"}

	root.AddChild(run)
	run.AddChild(start)

	// Root command has no path (parent is nil).
	r.Equal("", root.CommandPath())

	// Direct child of root.
	r.Equal("run", run.CommandPath())

	// Grandchild: should return full path excluding root.
	r.Equal("run start", start.CommandPath())
}

func TestCommand_ValidatesArgInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		skip     bool
		wantCode int
		wantRun  bool
	}{
		{name: "clean args run", args: []string{"ws-abc", "my-org"}, wantCode: 0, wantRun: true},
		{name: "control char rejected", args: []string{"bad\x1b[31m"}, wantCode: 1, wantRun: false},
		{name: "newline rejected", args: []string{"a\nb"}, wantCode: 1, wantRun: false},
		{name: "invalid utf8 rejected", args: []string{"a\xffb"}, wantCode: 1, wantRun: false},
		{name: "skip opt-out allows control char", args: []string{"raw\x1b[0m"}, skip: true, wantCode: 0, wantRun: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)

			io := iostreams.Test()
			cCtx := &Invocation{IO: io}

			ran := false
			root := &Command{
				Name: "root",
				io:   io,
				Args: PositionalArguments{
					Validate:            ArbitraryArgs,
					SkipInputValidation: tc.skip,
				},
				RunF: func(c *Command, args []string) error {
					ran = true
					return nil
				},
			}

			r.Equal(tc.wantCode, root.Run(tc.args, cCtx))
			r.Equal(tc.wantRun, ran)
			if !tc.wantRun {
				r.Contains(io.Error.String(), "invalid input")
			}
		})
	}
}
