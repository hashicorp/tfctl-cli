// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package flagvalue_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/pkg/flagvalue"
)

func ExampleSimpleMap() {
	var headers map[string]string
	f := pflag.NewFlagSet("example", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "headers",
		Usage: "headers is a set of headers to send with the request. May be specified multiple times in the form of KEY=VALUE.",
		Value: flagvalue.SimpleMap(nil, &headers),
	})

	// Make the request
}

func TestSimpleMap_StringToString(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var m map[string]string
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "values",
		Value: flagvalue.SimpleMap(map[string]string{"test": "value"}, &m),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(map[string]string{"test": "value"}, m)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--values", "hello=world", "--values", "false=123"}))
	r.EqualValues(map[string]string{
		"hello": "world",
		"false": "123",
	}, m)
}

func TestSimpleMap_StringToInt(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var m map[string]int
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "values",
		Value: flagvalue.SimpleMap(map[string]int{"test": 22}, &m),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(map[string]int{"test": 22}, m)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--values", "hello=49", "--values", "123=123"}))
	r.EqualValues(map[string]int{
		"hello": 49,
		"123":   123,
	}, m)
}

func TestSimpleMap_StringToBool(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var m map[string]bool
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "values",
		Value: flagvalue.SimpleMap(map[string]bool{"test": true}, &m),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(map[string]bool{"test": true}, m)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--values", "hello=true", "--values", "test=false"}))
	r.EqualValues(map[string]bool{
		"hello": true,
		"test":  false,
	}, m)
}

func TestSimpleMap_IntToString(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var m map[int]string
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "values",
		Value: flagvalue.SimpleMap(map[int]string{49: "test"}, &m),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(map[int]string{49: "test"}, m)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--values", "49=other", "--values", "123=123"}))
	r.EqualValues(map[int]string{
		49:  "other",
		123: "123",
	}, m)
}

func TestSimpleMap(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var stringToString map[string]string
	var stringToInt map[string]int
	var i8Tof64 map[int8]float64

	r.Equal("map[string]string", flagvalue.SimpleMap(nil, &stringToString).Type())
	r.Equal("map[string]int", flagvalue.SimpleMap(nil, &stringToInt).Type())
	r.Equal("map[int8]float64", flagvalue.SimpleMap(nil, &i8Tof64).Type())
}
