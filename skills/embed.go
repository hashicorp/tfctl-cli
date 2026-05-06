// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package skills contains the embedded skill definitions for tfcloud.
package skills

import "embed"

// FS is the embedded filesystem containing the skill definitions for tfcloud.
//
//go:embed tfcloud
var FS embed.FS
