// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue_test

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/flagvalue"
)

func ExampleDuration() {
	var sleep time.Duration
	f := pflag.NewFlagSet("example", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:     "wait",
		Usage:    "wait specifies the time to sleep before taking an action",
		DefValue: "5s",
		Value:    flagvalue.Duration(5*time.Second, &sleep),
	})

	time.Sleep(sleep)

	// ... Take an action
}

func TestDuration(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var dur time.Duration
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "dur",
		Value: flagvalue.Duration(time.Second, &dur),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(time.Second, dur)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--dur", "2m"}))
	r.Equal(2*time.Minute, dur)
}
