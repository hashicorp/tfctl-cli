// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue

import (
	"fmt"
	"reflect"
	"strings"
)

type simpleMapValue[K, V SimpleValue] struct {
	value   *map[K]V
	changed bool
}

// SimpleMap returns a pflags.Value that sets the map at p with the default
// val or the value(s) provided via a flag.
func SimpleMap[K, V SimpleValue](val map[K]V, p *map[K]V) *simpleMapValue[K, V] {
	isv := new(simpleMapValue[K, V])
	isv.value = p
	*isv.value = val
	return isv
}

func (m *simpleMapValue[K, V]) Set(s string) error {
	// Split the string, KEY=VALUE, into its parts
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected key=value, got %q", s)
	}

	// Parse the key
	var key K
	_, err := fmt.Sscanf(parts[0], "%v", &key)
	if err != nil {
		return fmt.Errorf("failed to parse key: %w", err)
	}

	// Parse the value
	var value V
	_, err = fmt.Sscanf(parts[1], "%v", &value)
	if err != nil {
		return fmt.Errorf("failed to parse value: %w", err)
	}

	if !m.changed {
		*m.value = map[K]V{
			key: value,
		}
	} else {
		(*m.value)[key] = value
	}

	m.changed = true
	return nil
}

func (m *simpleMapValue[K, V]) Type() string {
	return reflect.TypeOf(*m.value).String()
}

func (m *simpleMapValue[K, V]) String() string {
	out := make([]string, 0, len(*m.value))
	for k, v := range *m.value {
		out = append(out, fmt.Sprintf("%v=%v", k, v))
	}
	return "[" + strings.Join(out, ",") + "]"
}
