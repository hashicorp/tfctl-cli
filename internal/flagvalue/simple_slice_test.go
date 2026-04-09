// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue_test

import (
	"testing"

	"github.com/hashicorp/tfcloud/internal/flagvalue"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

func ExampleSimpleSlice() {
	var secrets []string
	f := pflag.NewFlagSet("example", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "secret",
		Usage: "secret is a secret to read. Multiple values may be specified.",
		Value: flagvalue.SimpleSlice[string]([]string{}, &secrets),
	})

	// Fetch the secrets
}

func TestSimpleSlice_Type(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	var i8 []int8
	var i16 []int16
	var i32 []int32
	var i64 []int64
	var ui8 []uint8
	var ui16 []uint16
	var ui32 []uint32
	var ui64 []uint64
	var f32 []float32
	var f64 []float64
	var b []bool
	var s []string

	r.Equal("int8Slice", flagvalue.SimpleSlice[int8](nil, &i8).Type())
	r.Equal("int16Slice", flagvalue.SimpleSlice[int16](nil, &i16).Type())
	r.Equal("int32Slice", flagvalue.SimpleSlice[int32](nil, &i32).Type())
	r.Equal("int64Slice", flagvalue.SimpleSlice[int64](nil, &i64).Type())
	r.Equal("uint8Slice", flagvalue.SimpleSlice[uint8](nil, &ui8).Type())
	r.Equal("uint16Slice", flagvalue.SimpleSlice[uint16](nil, &ui16).Type())
	r.Equal("uint32Slice", flagvalue.SimpleSlice[uint32](nil, &ui32).Type())
	r.Equal("uint64Slice", flagvalue.SimpleSlice[uint64](nil, &ui64).Type())
	r.Equal("float32Slice", flagvalue.SimpleSlice[float32](nil, &f32).Type())
	r.Equal("float64Slice", flagvalue.SimpleSlice[float64](nil, &f64).Type())
	r.Equal("boolSlice", flagvalue.SimpleSlice[bool](nil, &b).Type())
	r.Equal("stringSlice", flagvalue.SimpleSlice[string](nil, &s).Type())
}

func TestSimpleSlice_Bool(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var b []bool
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "bools",
		Value: flagvalue.SimpleSlice[bool]([]bool{}, &b),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal([]bool{}, b)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--bools", "true", "--bools", "false", "--bools", "true"}))
	r.Equal([]bool{true, false, true}, b)
}

func TestSimpleSlice_String(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var s []string
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "strings",
		Value: flagvalue.SimpleSlice[string]([]string{"test"}, &s),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal([]string{"test"}, s)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--strings", "hello", "--strings", "false", "--strings", "123"}))
	r.Equal([]string{"hello", "false", "123"}, s)
}

func TestSimpleSlice_Int(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var i []int
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "ints",
		Value: flagvalue.SimpleSlice[int]([]int{12}, &i),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal([]int{12}, i)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--ints", "123", "--ints", "1", "--ints", "-123"}))
	r.Equal([]int{123, 1, -123}, i)
}

func TestSimpleSlice_Float64(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	var floats []float64
	f := pflag.NewFlagSet("test", pflag.ContinueOnError)
	f.AddFlag(&pflag.Flag{
		Name:  "floats",
		Value: flagvalue.SimpleSlice[float64]([]float64{12.12}, &floats),
	})

	// Parse an empty set of args
	r.NoError(f.Parse([]string{}))

	// Expect the default
	r.Equal([]float64{12.12}, floats)

	// Parse with the flag set
	r.NoError(f.Parse([]string{"--floats", "123.123", "--floats", "1", "--floats", "-123.123"}))
	r.Equal([]float64{123.123, 1, -123.123}, floats)
}
