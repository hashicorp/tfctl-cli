// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/spf13/pflag"
)

type enumValue[T comparable] struct {
	allowed []T
	value   *T
}

// Enum returns a pflags.Value that sets the value at p with the default
// val or the value provided via a flag. The provided value must be in the
// allowed list or an error is returned.
func Enum[T comparable](allowed []T, val T, p *T) *enumValue[T] {
	v := new(enumValue[T])
	v.allowed = allowed
	v.value = p
	*p = val
	return v
}

func (i *enumValue[T]) Set(s string) error {
	var v T
	if _, err := fmt.Sscanf(s, "%v", &v); err != nil {
		return err
	}

	// Check the value is allowed
	if !slices.Contains(i.allowed, v) {
		return fmt.Errorf("must be one of %v", i.allowed)
	}

	*i.value = v
	return nil
}

func (i *enumValue[T]) Type() string {
	return reflect.TypeOf(*i.value).Name()
}

func (i *enumValue[T]) String() string {
	return fmt.Sprintf("%v", *i.value)
}

// Ensure we meet the interface
var _ pflag.Value = &simpleValue[bool]{}
