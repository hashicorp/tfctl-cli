// Copyright IBM Corp. 2020, 2026

package config

import (
	"fmt"
	"time"

	goversion "github.com/hashicorp/go-version"
)

// Name is the application name used throughout the CLI.
const Name = "tfcloud"

var (
	// Version defines what version this application is currently running as.
	// This needs to be a variable rather than a constant as we use build
	// arguments to overwrite this when we release a new version.
	version = "dev"

	// Version defines what version this application is currently running as. It
	// is the publicly used version, which will be prefixed with a `v` if it is
	// a SemVer version.
	Version = publicVersion(version)

	// Commit defines the git commit used for this specific version.
	Commit = "HEAD"

	// committedTime defines the time at which the compiled binary's latest git
	// commit was committed. This needs to be a string so the build flags can
	// overwrite it upon building official releases.
	committedTime = ""

	// CommitTime is the exposed time.Time version of the commitTime. It's
	// introduced so we can do time comparison as desired.
	CommitTime = mustParseTime(committedTime)
)

// IsDev returns true if the current version is a development version.
func IsDev() bool {
	return version == "dev"
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
