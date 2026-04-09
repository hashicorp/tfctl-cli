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

// Counter returns a pflags.Value that sets the value at p with the default
// val or the value provided via a flag.
func Counter(val int, p *int) Value {
	v := new(counterValue)
	v.value = p
	*p = val
	return v
}

func (i *counterValue) Set(_ string) error {
	*i.value++
	return nil
}

func (i *counterValue) Type() string {
	return "int"
}

func (i *counterValue) String() string {
	return fmt.Sprintf("%d", *i.value)
}

// Ensure we meet the interface.
var _ pflag.Value = &counterValue{}
