// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package flagvalue_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
)

func ExampleEnum() {
	var logLevel string
	f := pflag.NewFlagSet("example", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:     "log-level",
		Usage:    "log-level specifies the verbosity to log with.",
		DefValue: "warn",
		Value:    flagvalue.Enum([]string{"trace", "debug", "info", "warn", "error"}, "warn", &logLevel),
	})

	// Setup logger
	// logger := hclog.Default().SetLevel(hclog.LevelFromString(logLevel))
	// logger.Warn("we are using flags!")
}

func TestEnum_String(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var level string
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "log-level",
		Value: flagvalue.Enum([]string{"trace", "debug", "info", "warn", "error"}, "warn", &level),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal("warn", level)

	// Parse with the flag set to a valid enum
	r.NoError(f.Parse([]string{"--log-level", "trace"}))
	r.Equal("trace", level)

	// Parse with the flag set to an invalid enum
	err := f.Parse([]string{"--log-level", "random"})
	r.Error(err)
	r.ErrorContains(err, "must be one of [trace debug info warn error]")
}

func TestEnum_Int(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var level int
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "level",
		Value: flagvalue.Enum([]int{0, 10, 100}, 10, &level),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(10, level)

	// Parse with the flag set to a valid enum
	r.NoError(f.Parse([]string{"--level", "100"}))
	r.Equal(100, level)

	// Parse with the flag set to an invalid enum
	err := f.Parse([]string{"--level", "101"})
	r.Error(err)
	r.ErrorContains(err, "must be one of [0 10 100]")
}
