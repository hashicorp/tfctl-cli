// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/pflag"
)

type simpleSliceValue[T any] struct {
	value   *[]T
	changed bool
}

// SimpleSlice returns a pflags.Value that sets the slice at p with the default
// val or the value(s) provided via a flag.
func SimpleSlice[T SimpleValue](val []T, p *[]T) *simpleSliceValue[T] {
	isv := new(simpleSliceValue[T])
	isv.value = p
	*isv.value = val
	return isv
}

func (s *simpleSliceValue[T]) Set(val string) error {
	ss := strings.Split(val, ",")
	out := make([]T, len(ss))
	for i, val := range ss {
		_, err := fmt.Sscanf(val, "%v", &out[i])
		if err != nil {
			return err
		}
	}
	if !s.changed {
		*s.value = out
	} else {
		*s.value = append(*s.value, out...)
	}
	s.changed = true
	return nil
}

func (s *simpleSliceValue[T]) Type() string {
	return fmt.Sprintf("%sSlice", reflect.TypeOf(*s.value).Elem().Name())
}

func (s *simpleSliceValue[T]) String() string {
	out := make([]string, len(*s.value))
	for i, val := range *s.value {
		out[i] = fmt.Sprintf("%v", val)
	}
	return "[" + strings.Join(out, ",") + "]"
}

func (s *simpleSliceValue[T]) Append(val string) error {
	i, err := s.fromString(val)
	if err != nil {
		return err
	}
	*s.value = append(*s.value, i)
	return nil
}

func (s *simpleSliceValue[T]) Replace(val []string) error {
	out := make([]T, len(val))
	for i, d := range val {
		var err error
		out[i], err = s.fromString(d)
		if err != nil {
			return err
		}
	}
	*s.value = out
	return nil
}

func (s *simpleSliceValue[T]) GetSlice() []string {
	out := make([]string, len(*s.value))
	for i, d := range *s.value {
		out[i] = s.toString(d)
	}
	return out
}

func (s *simpleSliceValue[T]) fromString(val string) (T, error) {
	var out T
	_, err := fmt.Sscanf(val, "%v", &out)
	if err != nil {
		return out, err
	}

	return out, nil
}

func (s *simpleSliceValue[T]) toString(val T) string {
	return fmt.Sprintf("%v", val)
}

// Ensure we meet the interface
var _ pflag.Value = &simpleSliceValue[string]{}
var _ pflag.SliceValue = &simpleSliceValue[string]{}
