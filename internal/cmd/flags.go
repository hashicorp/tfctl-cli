// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cmd

const (
	// flagAnnotationGlobal is an annotation key that marks the flag as a
	// global flag.
	flagAnnotationGlobal = "cmd:global"

	// flagAnnotationRequired is an annotation key that marks the flag as a
	// required flag.
	flagAnnotationRequired = "cmd:required"

	// flagAnnotationRepeatable is an annotation key that marks the flag as a
	// repeatable flag.
	flagAnnotationRepeatable = "cmd:repeatable"

	// flagAnnotationDisplayValue is an annotation key that stores the optional
	// display value.
	flagAnnotationDisplayValue = "cmd:display_value"
)

// flagAnnotations is a set of annotations on flags to customize output.
type flagAnnotations map[string][]string

// newFlagAnnotations returns a new set of flag annotations.
func newFlagAnnotations() flagAnnotations {
	return make(map[string][]string)
}

// Global marks a flag as global
func (a flagAnnotations) Global() flagAnnotations {
	a[flagAnnotationGlobal] = nil
	return a
}

// isFlagGlobal returns whether the flag is global
func isFlagGlobal(a flagAnnotations) bool {
	_, ok := a[flagAnnotationGlobal]

	return ok
}

// Required marks a flag as required.
func (a flagAnnotations) Required() {
	a[flagAnnotationRequired] = nil
}

// isFlagRequired returns whether the flag is required.
func isFlagRequired(a flagAnnotations) bool {
	_, ok := a[flagAnnotationRequired]
	return ok
}

// DisplayValue stores the display value for the flag.
func (a flagAnnotations) DisplayValue(v string) {
	a[flagAnnotationDisplayValue] = []string{v}
}

// getFlagDisplayValue returns the display value for the flag or an empty string
// if it wasn't set.
func getFlagDisplayValue(a flagAnnotations) string {
	set, ok := a[flagAnnotationDisplayValue]
	if !ok {
		return ""
	}
	return set[0]
}

// Repeatable marks a flag as repeatable.
func (a flagAnnotations) Repeatable() {
	a[flagAnnotationRepeatable] = nil
}

// isFlagRepeatable returns whether the flag is repeatable.
func isFlagRepeatable(a flagAnnotations) bool {
	_, ok := a[flagAnnotationRepeatable]
	return ok
}
