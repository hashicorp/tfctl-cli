// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/posener/complete"
	"golang.org/x/net/idna"
)

const (
	// ActiveProfileFileName is the file name of the active profile stored in
	// the ConfigDir.
	ActiveProfileFileName = "active_profile.hcl"

	// DeviceIDFileName is the file name of the uuid used to identify this CLI installation for
	// telemetry purposes, stored in the ConfigDir.
	DeviceIDFileName = "device_id"
)

var (
	// ErrNoProfileFilePresent is returned when the requested profile does not
	// exist.
	ErrNoProfileFilePresent = errors.New("profile configuration file doesn't exist")

	// ErrInvalidProfileName is returned if a profile is created with an invalid
	// profile name.
	ErrInvalidProfileName = errors.New("profile name may only include a-z, A-Z, 0-9, or '_', must start with a letter, and can be no longer than 64 characters")

	validHostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:\d+)?$`)
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

// Profile is a named set of configuration for the CLI. It captures common
// configuration values such as the organization and project being interacted
// with, but also allows storing service specific configuration.
type Profile struct {
	// Name is the name of the profile
	Name string `hcl:"name"`

	// DefaultOrganization stores the default organization to make requests against.
	DefaultOrganization string `hcl:"default_organization"`

	// NoColor disables color output
	NoColor *bool `hcl:"no_color,optional" json:",omitempty"`

	// Hostname is the profile's configured hostname for API requests. If not set, the default is app.terraform.io.
	Hostname string `hcl:"hostname,optional" json:",omitempty"`

	// Token is the API token to use for API requests. If not set, the CLI will look for the token in the environment or terraform credentials.
	Token string `hcl:"token,optional" json:",omitempty"`

	// Telemetry controls telemetry behavior. Values: "false"/"disabled" to disable,
	// "log" to write spans to stderr, or any other value (including empty) to enable OTLP export.
	Telemetry *string `hcl:"telemetry,optional" json:",omitempty"`

	// tokenFromEnv is the token extracted from the environment. This is not written to disk and is only used to allow GetToken
	// to return a token from the environment if one is not set on the profile.
	tokenFromEnv string

	// dir is the directory the profile should write to.
	dir string

	// hostCacheDir is the directory the profile should write host-specific cache files to.
	hostCacheDir string
}

// Predict predicts the HCL key names and basic settable values.
func (p *Profile) Predict(args complete.Args) []string {
	properties := map[string][]string{
		"no_color":  {"true", "false"},
		"telemetry": {"true", "false", "disabled", "log"},
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
		return []string{"default_organization", "no_color", "hostname", "token", "telemetry"}
	}

	return nil
}

// HostCache returns a HostCacheLoader for the profile, which can be used to
// read and write host-specific cache files.
func (p *Profile) HostCache(ctx context.Context) (*HostCacheLoader, error) {
	hostname := p.GetHostname()
	if hostname == "" {
		return nil, fmt.Errorf("cannot get host cache with empty hostname")
	}

	return NewHostCacheLoader(ctx, p.hostCacheDir, hostname)
}

// Validate validates that the set values are valid. It validates parameters
// that do not require any communication with HCP.
func (p *Profile) Validate() error {
	err := &multierror.Error{}

	const nameRegex = "^[A-Za-z][A-Za-z0-9_]{0,63}$"
	if matched, _ := regexp.MatchString(nameRegex, p.Name); !matched {
		err = multierror.Append(err, ErrInvalidProfileName)
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

	return os.WriteFile(path, f.Bytes(), os.FileMode(0o600))
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

// SetHostname sets the profile's hostname after validating it. The hostname should be a hostname
// with an optional port, and should not include a scheme. If the hostname includes a scheme, the
// scheme will be stripped.
func (p *Profile) SetHostname(hostname string) error {
	if p == nil {
		return nil
	}

	hostname, err := NormalizeHostname(hostname)
	if err != nil {
		return err
	}
	p.Hostname = hostname
	return nil
}

func identifyIP(s string) (normalized string, isIP bool) {
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		// If SplitHostPort fails, it's either because there is no port
		// or the address is malformed.
		host = s
		port = ""

		// Handle IPv6 addresses that might be wrapped in brackets but don't have a port.
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Not an IP address (likely a hostname)
		return s, false
	}

	if ip.To4() != nil {
		// IPv4
		if port != "" {
			return fmt.Sprintf("%s:%s", ip.String(), port), true
		}
		return ip.String(), true
	}

	// IPv6
	// IPv6 addresses are normalized to always include brackets for consistency,
	// especially useful if a port is ever appended later.
	if port != "" {
		return fmt.Sprintf("[%s]:%s", ip.String(), port), true
	}
	return fmt.Sprintf("[%s]", ip.String()), true
}

// NormalizeHostname validates and normalizes the given hostname by stripping any extra URL data,
// like paths. It also converts domain names to their idna ASCII form.
func NormalizeHostname(hostname string) (string, error) {
	if ip, isIP := identifyIP(hostname); isIP {
		return ip, nil
	}

	u, err := url.Parse(hostname)
	if err != nil {
		return "", fmt.Errorf("invalid hostname %q: must be a valid hostname (with optional port)", hostname)
	}

	if err == nil && u.Host != "" {
		hostname = u.Host
	}

	if asciiHost, err := idna.Lookup.ToASCII(hostname); err == nil {
		return asciiHost, nil
	}

	if !validHostnamePattern.MatchString(hostname) {
		return "", fmt.Errorf("invalid hostname %q: must be a valid hostname (with optional port)", hostname)
	}

	return hostname, nil
}

// SetDefaultOrganization sets the default organization.
func (p *Profile) SetDefaultOrganization(name string) *Profile {
	if p == nil {
		return nil
	}

	p.DefaultOrganization = name
	return p
}

// GetTelemetry returns the telemetry setting or an empty string if unset.
func (p *Profile) GetTelemetry() string {
	if p == nil {
		return ""
	}

	if p.Telemetry == nil {
		return ""
	}

	return *p.Telemetry
}
