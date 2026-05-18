// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestPositionalArgs_validateFunc(t *testing.T) {
	t.Parallel()

	argValidationCommand := func(io iostreams.IOStreams, f ValidateArgsFunc) *Command {
		return &Command{
			Name:      "testing",
			ShortHelp: "testing",
			RunF: func(c *Command, args []string) error {
				return nil
			},
			NoAuthRequired: true,
			Args:           PositionalArguments{Validate: f},
			io:             io,
			logger:         hclog.NewNullLogger(),
		}
	}

	cases := []struct {
		Name      string
		ValidateF ValidateArgsFunc
		Args      []string
		Error     string
	}{
		{
			Name:  "default to no args",
			Args:  []string{"bad"},
			Error: "no arguments allowed, but received 1\n\nUsage: testing",
		},
		{
			Name:      "explicit no args - bad",
			ValidateF: NoArgs,
			Args:      []string{"bad", "bad"},
			Error:     "no arguments allowed, but received 2\n\nUsage: testing",
		},
		{
			Name:      "explicit no args - good",
			ValidateF: NoArgs,
			Args:      []string{},
		},
		{
			Name:      "arbitrary args",
			ValidateF: ArbitraryArgs,
			Args:      []string{"a", "b", "c", "d", "e"},
		},
		{
			Name:      "minimum args - bad",
			ValidateF: MinimumNArgs(3),
			Args:      []string{"bad", "bad"},
			Error:     "requires at least 3 arg(s), only received 2\n\nUsage: testing",
		},
		{
			Name:      "minimum args - good",
			ValidateF: MinimumNArgs(3),
			Args:      []string{"good", "good", "good", "good"},
		},
		{
			Name:      "maximum args - bad",
			ValidateF: MaximumNArgs(3),
			Args:      []string{"bad", "bad"},
		},
		{
			Name:      "maximum args - good",
			ValidateF: MaximumNArgs(3),
			Args:      []string{"good", "good", "good", "good"},
			Error:     "accepts at most 3 arg(s), received 4\n\nUsage: testing",
		},
		{
			Name:      "exact args - bad",
			ValidateF: ExactArgs(3),
			Args:      []string{"bad", "bad"},
			Error:     "accepts 3 arg(s), received 2\n\nUsage: testing",
		},
		{
			Name:      "exact args - good",
			ValidateF: ExactArgs(4),
			Args:      []string{"good", "good", "good", "good"},
		},
		{
			Name:      "range args - bad low",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"bad"},
			Error:     "accepts between 2 and 4 arg(s), received 1\n\nUsage: testing",
		},
		{
			Name:      "range args - bad high",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"1", "2", "3", "4", "5"},
			Error:     "accepts between 2 and 4 arg(s), received 5\n\nUsage: testing",
		},
		{
			Name:      "range args - good low inclusive",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"good", "good"},
		},
		{
			Name:      "range args - good middle",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"good", "good", "good"},
		},
		{
			Name:      "range args - good high inclusive",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"good", "good", "good", "good"},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			io := iostreams.Test()
			command := argValidationCommand(io, c.ValidateF)
			code := command.Run(c.Args, &Context{IO: io})
			if c.Error == "" {
				r.Zero(code, io.Error.String())
				return
			}

			// Expect an error
			r.NotZero(code)
			r.Contains(io.Error.String(), c.Error)
		})
	}
}

func TestCommand_ArgsValidation(t *testing.T) {
	t.Parallel()

	argValidationCommand := func(io iostreams.IOStreams, f ValidateArgsFunc) *Command {
		return &Command{
			Name:      "testing",
			ShortHelp: "testing",
			RunF: func(c *Command, args []string) error {
				return nil
			},
			NoAuthRequired: true,
			Args:           PositionalArguments{Validate: f},
			io:             io,
			logger:         hclog.NewNullLogger(),
		}
	}

	cases := []struct {
		Name      string
		ValidateF ValidateArgsFunc
		Args      []string
		Error     string
	}{
		{
			Name:  "default to no args",
			Args:  []string{"bad"},
			Error: "no arguments allowed, but received 1\n\nUsage: testing",
		},
		{
			Name:      "explicit no args - bad",
			ValidateF: NoArgs,
			Args:      []string{"bad", "bad"},
			Error:     "no arguments allowed, but received 2\n\nUsage: testing",
		},
		{
			Name:      "explicit no args - good",
			ValidateF: NoArgs,
			Args:      []string{},
		},
		{
			Name:      "arbitrary args",
			ValidateF: ArbitraryArgs,
			Args:      []string{"a", "b", "c", "d", "e"},
		},
		{
			Name:      "minimum args - bad",
			ValidateF: MinimumNArgs(3),
			Args:      []string{"bad", "bad"},
			Error:     "requires at least 3 arg(s), only received 2\n\nUsage: testing",
		},
		{
			Name:      "minimum args - good",
			ValidateF: MinimumNArgs(3),
			Args:      []string{"good", "good", "good", "good"},
		},
		{
			Name:      "maximum args - bad",
			ValidateF: MaximumNArgs(3),
			Args:      []string{"bad", "bad"},
		},
		{
			Name:      "maximum args - good",
			ValidateF: MaximumNArgs(3),
			Args:      []string{"good", "good", "good", "good"},
			Error:     "accepts at most 3 arg(s), received 4\n\nUsage: testing",
		},
		{
			Name:      "exact args - bad",
			ValidateF: ExactArgs(3),
			Args:      []string{"bad", "bad"},
			Error:     "accepts 3 arg(s), received 2\n\nUsage: testing",
		},
		{
			Name:      "exact args - good",
			ValidateF: ExactArgs(4),
			Args:      []string{"good", "good", "good", "good"},
		},
		{
			Name:      "range args - bad low",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"bad"},
			Error:     "accepts between 2 and 4 arg(s), received 1\n\nUsage: testing",
		},
		{
			Name:      "range args - bad high",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"1", "2", "3", "4", "5"},
			Error:     "accepts between 2 and 4 arg(s), received 5\n\nUsage: testing",
		},
		{
			Name:      "range args - good low inclusive",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"good", "good"},
		},
		{
			Name:      "range args - good middle",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"good", "good", "good"},
		},
		{
			Name:      "range args - good high inclusive",
			ValidateF: RangeArgs(2, 4),
			Args:      []string{"good", "good", "good", "good"},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := require.New(t)

			io := iostreams.Test()
			command := argValidationCommand(io, c.ValidateF)
			code := command.Run(c.Args, &Context{IO: io})
			if c.Error == "" {
				r.Zero(code, io.Error.String())
				return
			}

			// Expect an error
			r.NotZero(code)
			r.Contains(io.Error.String(), c.Error)
		})
	}
}
