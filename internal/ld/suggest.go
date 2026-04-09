// Copyright IBM Corp. 2026

// package ld implements levenshtein distance in order to provide suggestions
// based on similarity.
package ld

import "strings"

// Distance compares two strings and returns the levenshtein distance between them.
func Distance(s, t string, ignoreCase bool) int {
	if ignoreCase {
		s = strings.ToLower(s)
		t = strings.ToLower(t)
	}
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
	}
	for i := range d {
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for j := 1; j <= len(t); j++ {
		for i := 1; i <= len(s); i++ {
			if s[i-1] == t[j-1] {
				d[i][j] = d[i-1][j-1]
			} else {
				min := d[i-1][j]
				if d[i][j-1] < min {
					min = d[i][j-1]
				}
				if d[i-1][j-1] < min {
					min = d[i-1][j-1]
				}
				d[i][j] = min + 1
			}
		}

	}
	return d[len(s)][len(t)]
}

// Suggestions takes in an input and a set of valid options. It returns a set of
// suggested values based on the levenshtein distance between them. The
// distanceCutoff can be used to control the required similarity of the option
// to the input for it to be included in the suggestions.
func Suggestions(input string, options []string, distanceCutoff int, ignoreCase bool) []string {
	return SuggestionsWithOverride(input, options, distanceCutoff, ignoreCase, nil)
}

// SuggestionsWithOverride is similar to Suggestions, except it takes an
// optional include function. If the distance is less than the cutoff, the
// passed override function will be invoked, and if it returns true, the option
// will be added to the suggestions list. This allows custom suggestion logic to
// be added.
func SuggestionsWithOverride(input string, options []string, distanceCutoff int, ignoreCase bool, override func(input, option string) bool) []string {
	suggestions := []string{}
	for _, o := range options {
		d := Distance(input, o, ignoreCase)
		if d <= distanceCutoff {
			suggestions = append(suggestions, o)
		} else if override != nil && override(input, o) {
			suggestions = append(suggestions, o)
		}
	}

	return suggestions
}
