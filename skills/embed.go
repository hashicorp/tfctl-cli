// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package skills contains the embedded skill definitions for tfctl.
package skills

import "embed"

// FS is the embedded filesystem containing the skill definitions for tfctl.
//
//go:embed tfctl
var FS embed.FS

// EmbeddedChecksum returns the CRC32 checksum of the embedded SKILL.md file.
func EmbeddedChecksum() uint32 {
	file, err := FS.Open(TFCTLSkillPath)
	if err != nil {
		return 0
	}
	checksum, err := calculateCRC32(file)
	if err != nil {
		return 0
	}
	return checksum
}
