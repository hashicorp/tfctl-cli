// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cmd

import "fmt"

// ValidateArgsFunc is a function used to validate a command has received valid
// arguments.
type ValidateArgsFunc func(c *Command, args []string) error

// NoArgs is a ValidateArgsFunc that validates that no arguments are received.
func NoArgs(c *Command, args []string) error {
	if l := len(args); l > 0 {
		return fmt.Errorf("no arguments allowed, but received %d", l)
	}

	return nil
}

// ArbitraryArgs never returns an error and is used to bypass argument
// validation at the command level.
func ArbitraryArgs(c *Command, args []string) error {
	return nil
}

// MinimumNArgs returns an error if there is not at least N (inclusive) args.
func MinimumNArgs(n int) ValidateArgsFunc {
	return func(cmd *Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("requires at least %d arg(s), only received %d", n, len(args))
		}
		return nil
	}
}

// MaximumNArgs returns an error if there are more than N (inclusive) args.
func MaximumNArgs(n int) ValidateArgsFunc {
	return func(cmd *Command, args []string) error {
		if len(args) > n {
			return fmt.Errorf("accepts at most %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}

// ExactArgs returns an error if there are not exactly N args.
func ExactArgs(n int) ValidateArgsFunc {
	return func(cmd *Command, args []string) error {
		if len(args) != n {
			return fmt.Errorf("accepts %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}

// RangeArgs returns an error if the number of args is not within the expected range.
func RangeArgs(min int, max int) ValidateArgsFunc {
	return func(cmd *Command, args []string) error {
		if len(args) < min || len(args) > max {
			return fmt.Errorf("accepts between %d and %d arg(s), received %d", min, max, len(args))
		}
		return nil
	}
}
