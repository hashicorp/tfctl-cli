// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package skills contains the embedded skill definitions for tfctl.
package skills

import "embed"

// FS is the embedded filesystem containing the skill definitions for tfctl.
//
//go:embed tfctl
var FS embed.FS

// EmbeddedSkillHash returns the SHA256 hash of the embedded SKILL.md file.
func EmbeddedSkillHash() string {
	file, err := FS.Open(TFCTLSkillPath)
	if err != nil {
		return ""
	}
	hash, err := hashSHA256Hex(file)
	if err != nil {
		return ""
	}
	return hash
}
