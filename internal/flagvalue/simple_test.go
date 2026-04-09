// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue_test

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfcloud/internal/flagvalue"
)

func ExampleSimple() {
	var projectID string
	f := pflag.NewFlagSet("example", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "project",
		Usage: "project specifies the HCP Project ID to use.",
		Value: flagvalue.Simple[string]("", &projectID),
	})
}

func ExampleSimple_boolean() {
	var force bool
	f := pflag.NewFlagSet("example", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:      "force",
		Shorthand: "f",
		Usage:     "force force deletes without confirmation.",
		Value:     flagvalue.Simple[bool](false, &force),

		// Critical to set for boolean values. Otherwise -f, --force will not
		// set force to true. Instead the flag parsing will error expecting a
		// value to be set for the flag.
		NoOptDefVal: "true",
	})
}

func TestSimple_Type(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	var i8 int8
	var i16 int16
	var i32 int32
	var i64 int64
	var ui8 uint8
	var ui16 uint16
	var ui32 uint32
	var ui64 uint64
	var f32 float32
	var f64 float64
	var b bool
	var s string

	r.Equal("int8", flagvalue.Simple[int8](10, &i8).Type())
	r.Equal("int16", flagvalue.Simple[int16](10, &i16).Type())
	r.Equal("int32", flagvalue.Simple[int32](10, &i32).Type())
	r.Equal("int64", flagvalue.Simple[int64](10, &i64).Type())
	r.Equal("uint8", flagvalue.Simple[uint8](10, &ui8).Type())
	r.Equal("uint16", flagvalue.Simple[uint16](10, &ui16).Type())
	r.Equal("uint32", flagvalue.Simple[uint32](10, &ui32).Type())
	r.Equal("uint64", flagvalue.Simple[uint64](10, &ui64).Type())
	r.Equal("float32", flagvalue.Simple[float32](10.21, &f32).Type())
	r.Equal("float64", flagvalue.Simple[float64](10.21, &f64).Type())
	r.Equal("bool", flagvalue.Simple[bool](false, &b).Type())
	r.Equal("string", flagvalue.Simple[string]("foo", &s).Type())
}

func TestSimple_Bool(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var b bool
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "bool",
		Value: flagvalue.Simple[bool](false, &b),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(false, b)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--bool", "true"}))
	r.Equal(true, b)

	// Parse with the flag set to an invalid value
	err := f.Parse([]string{"--bool=what"})
	r.Error(err)
	r.ErrorContains(err, `failed to parse "what" as a boolean`)
}

func TestSimple_Bool_Ptr(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var b *bool
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "bool",
		Value: flagvalue.Simple[*bool]((*bool)(nil), &b),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Nil(b)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--bool", "true"}))
	r.NotNil(b)
	r.Equal(true, *b)

	// Parse with the flag set to an invalid value
	err := f.Parse([]string{"--bool=what"})
	r.Error(err)
	r.ErrorContains(err, `failed to parse "what" as a boolean`)
}

func TestSimple_String(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var s string
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "string",
		Value: flagvalue.Simple[string]("foo", &s),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal("foo", s)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--string", "hello"}))
	r.Equal("hello", s)

	// Parse with a long string
	r.NoError(f.Parse([]string{"--string", "hello, world!"}))
	r.Equal("hello, world!", s)
}

func TestSimple_String_Ptr(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var s *string
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "string",
		Value: flagvalue.Simple((*string)(nil), &s),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Nil(s)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--string", "hello"}))
	r.NotNil(s)
	r.Equal("hello", *s)

	// Parse with a long string
	r.NoError(f.Parse([]string{"--string", "hello, world!"}))
	r.NotNil(s)
	r.Equal("hello, world!", *s)
}

func TestSimple_Int(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var i int
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "int",
		Value: flagvalue.Simple[int](10, &i),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(int(10), i)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--int", "42"}))
	r.Equal(int(42), i)

	// Parse with the flag set to an invalid value
	err := f.Parse([]string{"--int=what"})
	r.Error(err)
	r.Error(err)
}

func TestSimple_Int8(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var i int8
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "int",
		Value: flagvalue.Simple[int8](10, &i),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(int8(10), i)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--int", "42"}))
	r.Equal(int8(42), i)

	// Parse with the flag set to an invalid value
	err := f.Parse([]string{"--int=what"})
	r.Error(err)
}

func TestSimple_Uint64(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var i uint64
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "int",
		Value: flagvalue.Simple[uint64](10, &i),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(uint64(10), i)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--int", "42"}))
	r.Equal(uint64(42), i)

	// Parse with the flag set to an invalid value
	err := f.Parse([]string{"--int=what"})
	r.Error(err)
}

func TestSimple_Float32(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var i float32
	defVal := float32(10.3)
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "number",
		Value: flagvalue.Simple[float32](defVal, &i),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal(defVal, i)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--number", "42.42"}))
	r.Equal(float32(42.42), i)

	// Parse with the flag set to an invalid value
	err := f.Parse([]string{"--int=what"})
	r.Error(err)
}
