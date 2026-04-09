// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import "testing"

// TestProfile returns a profile appropriate for use during testing. If
// interacting with more than one profile, prefer using TestLoader.
func TestProfile(t *testing.T) *Profile { //nolint:paralleltest
	return TestLoader(t).DefaultProfile()
}

// TestLoader returns a Loader suitable for testing. All profiles that are
// accessed will be in the context of a temporary directory.
func TestLoader(t *testing.T) *Loader { //nolint:paralleltest
	l, err := newLoader(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create profile loader: %v", err)
	}

	if err := l.DefaultActiveProfile().Write(); err != nil {
		t.Fatalf("failed to create default active profile file: %v", err)
	}

	return l
}
