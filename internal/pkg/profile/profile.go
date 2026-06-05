// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/posener/complete"
)

const (
	// ActiveProfileFileName is the file name of the active profile stored in
	// the ConfigDir.
	ActiveProfileFileName = "active_profile.hcl"

	// VerbosityTrace is the trace verbosity level, which logs all messages including very detailed tracing messages.
	VerbosityTrace = "trace"

	// VerbosityDebug is the debug verbosity level, which logs debugging messages and above.
	VerbosityDebug = "debug"

	// VerbosityInfo is the info verbosity level, which logs informational messages and above.
	VerbosityInfo = "info"

	// VerbosityWarn is the warning verbosity level, which logs warning messages and above.
	VerbosityWarn = "warn"

	// VerbosityError is the error verbosity level, which only logs error messages.
	VerbosityError = "error"
)

var (
	// ErrNoProfileFilePresent is returned when the requested profile does not
	// exist.
	ErrNoProfileFilePresent = errors.New("profile configuration file doesn't exist")

	// ErrInvalidProfileName is returned if a profile is created with an invalid
	// profile name.
	ErrInvalidProfileName = errors.New("profile name may only include a-z, A-Z, 0-9, or '_', must start with a letter, and can be no longer than 64 characters")
)

// ActiveProfile stores the active profile.
type ActiveProfile struct {
	Name string `hcl:"name"`

	// dir is the directory the active profile should be written to.
	dir string
}

// Write writes the active profile to disk.
func (c *ActiveProfile) Write() error {
	path := filepath.Join(c.dir, ActiveProfileFileName)
	f := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(c, f.Body())
	return os.WriteFile(path, f.Bytes(), 0o666)
}

// Profile is a named set of configuration for the CLI. It captures common
// configuration values such as the organization and project being interacted
// with, but also allows storing service specific configuration.

//      _
//     | |
//   __| | __ _ _ __   __ _  ___ _ __
//  / _` |/ _` | '_ \ / _` |/ _ \ '__|
// | (_| | (_| | | | | (_| |  __/ |
//  \__,_|\__,_|_| |_|\__, |\___|_|
//                     __/ |
//                    |___/

// As long as hclsimple.Decode is used to load the profile, you can't remove any of these fields
// without causing a loading error.
type Profile struct {
	// Name is the name of the profile
	Name string `hcl:"name"`

	// Organization stores the organization to make requests against.
	Organization string `hcl:"organization"`

	// NoColor disables color output
	NoColor *bool `hcl:"no_color,optional" json:",omitempty"`

	// Verbosity is the default verbosity to log at
	Verbosity *string `hcl:"verbosity,optional" json:",omitempty"`

	// Quiet is whether the CLI should minimize output
	Quiet *bool `hcl:"quiet,optional" json:",omitempty"`

	// Hostname is the profile's configured hostname for API requests. If not set, the default is app.terraform.io.
	Hostname string `hcl:"hostname,optional" json:",omitempty"`

	// Token is the API token to use for API requests. If not set, the CLI will look for the token in the environment or terraform credentials.
	Token string `hcl:"token,optional" json:",omitempty"`

	// tokenFromEnv is the token extracted from the environment. This is not written to disk and is only used to allow GetToken
	// to return a token from the environment if one is not set on the profile.
	tokenFromEnv string

	// dir is the directory the profile should write to.
	dir string
}

// Predict predicts the HCL key names and basic settable values.
func (p *Profile) Predict(args complete.Args) []string {
	properties := map[string][]string{
		"no_color":  {"true", "false"},
		"verbosity": {VerbosityTrace, VerbosityDebug, VerbosityInfo, VerbosityWarn, VerbosityError},
		"quiet":     {"true", "false"},
	}

	// If the property has been specified, return possible values.
	if len(args.All) >= 1 {
		prediction, ok := properties[args.All[0]]
		if ok {
			return prediction
		}
	}

	// predicting the property
	if len(args.All) == 1 {
		return []string{"organization", "no_color", "verbosity", "quiet", "hostname", "token"}
	}

	return nil
}

// Validate validates that the set values are valid. It validates parameters
// that do not require any communication with HCP.
func (p *Profile) Validate() error {
	err := &multierror.Error{}

	const nameRegex = "^[A-Za-z][A-Za-z0-9_]{0,63}$"
	if matched, _ := regexp.MatchString(nameRegex, p.Name); !matched {
		err = multierror.Append(err, ErrInvalidProfileName)
	}

	allowedVerbosities := []string{VerbosityTrace, VerbosityDebug, VerbosityInfo, VerbosityWarn, VerbosityError}
	if f := p.GetVerbosity(); f != "" && !slices.Contains(allowedVerbosities, f) {
		err = multierror.Append(err, fmt.Errorf("invalid verbosity %q. Must be one of: %q", f, allowedVerbosities))
	}

	err.ErrorFormat = func(errors []error) string {
		if len(errors) == 1 {
			return errors[0].Error()
		}

		numErrors := len(errors)
		var buf bytes.Buffer
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf)
		for i, e := range errors {
			fmt.Fprintf(&buf, "  * %s", e)
			if i != numErrors-1 {
				fmt.Fprintln(&buf)
			}
		}
		return buf.String()
	}

	return err.ErrorOrNil()
}

// Clean nils any empty component.
func (p *Profile) Clean() {
}

// Write writes the profile to disk.
func (p *Profile) Write() error {
	// Remove any empty components before writing
	p.Clean()

	path := fmt.Sprintf("%s/%s.hcl", p.dir, p.Name)
	f := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(p, f.Body())
	return os.WriteFile(path, f.Bytes(), 0o666)
}

// String returns an HCL formatted string representation of the profile.
func (p Profile) String() string {
	f := hclwrite.NewEmptyFile()
	p.Token = "(sensitive)"
	gohcl.EncodeIntoBody(p, f.Body())
	return strings.TrimSpace(string(f.Bytes()))
}

// PropertyNames returns the name of the properties in a profile. If the
// property is in a struct, such as Core, the property name will be
// <struct_name>/<property_name>, such as "core/no_color".
func PropertyNames() map[string]struct{} {
	keys := make(map[string]struct{})
	var p Profile
	doWalkStructElements("", reflect.TypeOf(p), keys)
	return keys
}

func doWalkStructElements(path string, t reflect.Type, keys map[string]struct{}) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Get the tag
		name := field.Tag.Get("hcl")
		if name == "" {
			continue
		}

		name = strings.Split(name, ",")[0]
		if path != "" {
			name = fmt.Sprintf("%s/%s", path, name)
		}

		v := field.Type
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			doWalkStructElements(name, v, keys)
		} else {
			keys[name] = struct{}{}
		}

	}
}

// GetVerbosity returns the set verbosity or an empty string if it has not been
// configured.
func (p *Profile) GetVerbosity() string {
	if p == nil {
		return ""
	}

	if p.Verbosity == nil {
		return ""
	}

	return *p.Verbosity
}

// GetToken returns the token set on the profile, or the token extracted from the environment
// if one is available.
func (p *Profile) GetToken() string {
	if p == nil {
		return ""
	}

	if p.Token != "" {
		return p.Token
	}

	return p.tokenFromEnv
}

// GetHostname returns the set hostname or the default hostname if it has not been configured.
func (p *Profile) GetHostname() string {
	if p == nil {
		return ""
	}

	if p.Hostname == "" {
		return DefaultHostname
	}

	return p.Hostname
}

// SetOrg sets the Organization.
func (p *Profile) SetOrg(name string) *Profile {
	if p == nil {
		return nil
	}

	p.Organization = name
	return p
}

// IsQuiet returns whether the quiet property has been configured to be quiet.
func (p *Profile) IsQuiet() bool {
	if p == nil {
		return false
	}

	if p.Quiet == nil {
		return false
	}

	return *p.Quiet
}
