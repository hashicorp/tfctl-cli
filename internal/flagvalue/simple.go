// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/spf13/pflag"
	"golang.org/x/exp/constraints"
)

// Value is the interface to the dynamic value stored in a flag.
type Value = pflag.Value

type SimpleValue interface {
	constraints.Float | constraints.Integer | ~string | ~bool |
		*string | *bool
}

type simpleValue[T any] struct {
	value *T
}

// Simple returns a pflags.Value that sets the value at p with the default
// val or the value provided via a flag.
//
// If the type of the value is a boolean, set the NoOptDefVal to "true", on the
// Flag. Otherwise the flag will have to have a value set to be parsed. As an
// example if the boolean flag had the name "force" and NoOptDefVal is not set,
// the flag will have to be set as --force=true.
func Simple[T SimpleValue](val T, p *T) *simpleValue[T] {
	v := new(simpleValue[T])
	v.value = p
	*p = val
	return v
}

func (i *simpleValue[T]) Set(s string) error {
	var err error
	switch v := any(i.value).(type) {
	case **string:
		*v = new(string)
		**v = s
	case *string:
		*v = s
	case **bool:
		*v = new(bool)
		err = parseBool[bool](s, *v)
	case *bool:
		err = parseBool[bool](s, v)
	default:
		_, err = fmt.Sscanf(s, "%v", v)
	}
	return err
}

func parseBool[T bool](s string, v *T) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("failed to parse %q as a boolean", s)
	}

	*v = T(b)
	return nil
}

func (i *simpleValue[T]) Type() string {
	return reflect.TypeOf(*i.value).Name()
}

func (i *simpleValue[T]) String() string {
	return fmt.Sprintf("%v", *i.value)
}

// Ensure we meet the interface
var _ pflag.Value = &simpleValue[bool]{}
