// Copyright IBM Corp. 2026

// Package config contains runtime configuration related code for the CLI, such as
// version information.
package config

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
)

// Name is the application name used throughout the CLI.
const Name = "tfctl"

var (
	// Version defines what version this application is currently running as.
	//go:embed VERSION
	version string

	// Version defines what version this application is currently running as. It
	// is the publicly used version, which will be prefixed with a `v` if it is
	// a SemVer version.
	Version = publicVersion(version)

	// Commit defines the git commit used for this specific version.
	// CHANGE THIS VALUE WITH A BUILD ARGUMENT.
	commit = "HEAD"

	// committedTime defines the time at which the compiled binary's latest git
	// commit was committed.
	// CHANGE THIS VALUE WITH A BUILD ARGUMENT.
	committedTime = ""

	// CommitTime is the exposed time.Time version of the commitTime. It's
	// introduced so we can do time comparison as desired.
	CommitTime = mustParseTime(committedTime)
)

// IsDev returns true if the current version is a development version.
func IsDev() bool {
	return strings.HasSuffix(version, "-dev")
}

// Commit returns the git commit used for this specific version.
func Commit() string {
	return commit
}

// mustParseTime will parse a time string and panic if it is not able to.
func mustParseTime(ts string) time.Time {
	if ts == "" {
		ts = time.Now().Format(time.RFC3339)
	}

	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		panic(err)
	}
	return t
}

// publicVersion takes a version string and converts it into a publicly
// displayable string. This means that if the given version string is a SemVer
// version, we will ensure it is prefixed with a `v`. If not, we will leave the
// version as is and return it.
func publicVersion(v string) string {
	sv, err := goversion.NewSemver(v)
	if err != nil {
		return v
	}

	return fmt.Sprintf("v%s", sv.String())
}
