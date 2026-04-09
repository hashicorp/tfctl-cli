// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package flagvalue

import (
	"time"
)

type durationValue time.Duration

// Duration returns a pflag.Value that sets the duration at p with the default
// val or the value provided via a flag.
func Duration(val time.Duration, p *time.Duration) Value {
	*p = val
	return (*durationValue)(p)
}

// Set implements the pflag.Value interface.
func (d *durationValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	*d = durationValue(v)
	return err
}

// Type implements the pflag.Value interface.
func (d *durationValue) Type() string {
	return "duration"
}

// String implements the pflag.Value interface.
func (d *durationValue) String() string {
	return (*time.Duration)(d).String()
}
