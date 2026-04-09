// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue

import (
	"fmt"

	"github.com/spf13/pflag"
)

type counterValue struct {
	value *int
}

// Simple returns a pflags.Value that sets the value at p with the default
// val or the value provided via a flag.
//
// If the type of the value is a boolean, set the NoOptDefVal to "true", on the
// Flag. Otherwise the flag will have to have a value set to be parsed. As an
// example if the boolean flag had the name "force" and NoOptDefVal is not set,
// the flag will have to be set as --force=true.
func Counter(val int, p *int) *counterValue {
	v := new(counterValue)
	v.value = p
	*p = val
	return v
}

func (i *counterValue) Set(s string) error {
	*i.value++
	return nil
}

func (i *counterValue) Type() string {
	return "int"
}

func (i *counterValue) String() string {
	return fmt.Sprintf("%d", *i.value)
}

// Ensure we meet the interface
var _ pflag.Value = &counterValue{}
