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
	"github.com/hashicorp/go-tfe"

	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
)

func TestCommand_PersistentPrerun(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create the command tree
	io := iostreams.Test()
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
	r.Zero(grandchild.Run(nil))
	r.Zero(child.Run(nil))

	// Expect the prerun commmands were called
	r.Equal(2, rootPreRunCount)
	r.Equal(1, containerPreRunCount)
}

func TestCommand_Flags(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create the command tree
	io := iostreams.Test()
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

	r.Zero(child.Run([]string{"--root-flag=root-set", "--child-flag=child-set"}))
	r.Equal(2, seenFlags)
	r.Equal("root-set", *rootFlag)
	r.Equal("child-set", *childFlag)
}

func TestCommand_Logger(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create the command tree
	io := iostreams.Test()
	root := &Command{
		Name: "root",
		io:   io,
	}
	child := &Command{
		Name: "child",
		RunF: func(c *Command, args []string) error {
			c.Logger().Error("hello, world!")
			return nil
		},
	}
	root.AddChild(child)
	r.Zero(child.Run([]string{}))
	r.Contains(io.Error.String(), "tfcloud.child: hello, world!")
}

func TestCommand_ExitCode(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	// Create the command tree
	io := iostreams.Test()
	code := 42
	err := fmt.Errorf("bad bad bad")
	root := &Command{
		Name: "root",
		io:   io,
		RunF: func(c *Command, args []string) error {
			return NewExitError(code, err)
		},
	}
	r.Equal(code, root.Run([]string{}))
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
		{err: tfe.ErrNotFound, expected: 2, errContains: "Resource not found or you are unauthorized to this action"},
		{err: tfe.ErrUnauthorized, expected: 3, errContains: "tfcloud auth login"},
		{err: opErr, expected: 4, errContains: "network error"},
		{err: tfe.ErrInternalServer, expected: 5, errContains: "Internal Server Error"},
		{err: fmt.Errorf("some other error"), expected: 1, errContains: "ERROR: some other error"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("err %T exits with code %d", tt.err, tt.expected), func(t *testing.T) {
			r := require.New(t)

			// Create the command tree
			io := iostreams.Test()
			root := &Command{
				Name: "root",
				io:   io,
				RunF: func(c *Command, args []string) error {
					return tt.err
				},
			}
			r.Equal(tt.expected, root.Run([]string{}))
			if tt.errContains != "" {
				r.Contains(io.Error.String(), tt.errContains)
			}
		})
	}
}
