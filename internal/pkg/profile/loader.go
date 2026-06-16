// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/net/idna"

	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/version"
)

var (
	// ConfigDir is the directory that contains CLI configuration.
	ConfigDir = fmt.Sprintf("~/.config/%s/", version.Name)
)

const (
	// ProfileDir is the directory that contains CLI configuration profiles.
	ProfileDir = "profiles/"

	// ProfileNameDefault is the default profile name.
	ProfileNameDefault = "default"

	// TerraformCredentialsPath is the path to the terraform credentials file that we will check for
	// tokens if they're not set in the profiler.
	TerraformCredentialsPath = "~/.terraform.d/credentials.tfrc.json"

	// DefaultHostname is the default hostname to use if one is not set in the profile or environment variable.
	DefaultHostname = "app.terraform.io"
)

var (
	// ErrNoActiveProfileFilePresent is returned if no active profile file
	// exists.
	ErrNoActiveProfileFilePresent = errors.New("active profile file doesn't exist")

	// ErrActiveProfileFileEmpty is returned if the active profile file is
	// empty.
	ErrActiveProfileFileEmpty = errors.New("active profile is unset")
)

// Loader is used to load and interact with profiles on disk.
type Loader struct {
	// configDir is the configuration directory.
	configDir string

	// profilesDir is the directory containing profiles.
	profilesDir string
}

// NewLoader returns a new loader or an error if the loader can't be
// instantiated.
func NewLoader() (*Loader, error) {
	return newLoader(ConfigDir)
}

// newLoader returns a new loader for the given config directory.
func newLoader(dir string) (*Loader, error) {
	path, err := homedir.Expand(dir)
	if err != nil {
		return nil, fmt.Errorf("error expanding %s config directory path %q: %w", version.Name, dir, err)
	}

	// Ensure the config directory exists.
	_, err = os.Stat(path)
	if err != nil {
		// If the directory doesn't exist, create it.
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(path, 0700); err != nil {
				return nil, fmt.Errorf("failed to created %s config directory %q: %w", version.Name, path, err)
			}
		} else {
			return nil, fmt.Errorf("failed to check if %s config directory exists: %w", version.Name, err)
		}
	}

	// Ensure the profiles directory exists.
	profilesDir := filepath.Join(path, ProfileDir)
	_, err = os.Stat(profilesDir)
	if err != nil {
		// If the directory doesn't exist, create it.
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(profilesDir, 0700); err != nil {
				return nil, fmt.Errorf("failed to created %s profiles directory %q: %w", version.Name, profilesDir, err)
			}
		} else {
			return nil, fmt.Errorf("failed to check if %s profiles directory exists: %w", version.Name, err)
		}
	}

	return &Loader{
		configDir:   path,
		profilesDir: profilesDir,
	}, nil
}

// GetDeviceID returns the unique identifier for this CLI installation, used for telemetry purposes.
func (l *Loader) GetDeviceID(ctx context.Context) string {
	logger := logging.FromContext(ctx)
	deviceIDPath := filepath.Join(l.configDir, DeviceIDFileName)
	_, err := os.Stat(deviceIDPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If the device ID file doesn't exist, create it with a new UUID.
			if err := os.WriteFile(deviceIDPath, []byte(uuid.New().String()), 0600); err != nil {
				logger.Error("Failed to write device ID file", "error", err)
			}
		}
	}

	deviceID, err := os.ReadFile(deviceIDPath)
	if err != nil {
		logger.Error("Failed to read device ID file, generating a temporary device ID for this session", "error", err)
		deviceID = []byte(uuid.New().String())
	}

	return string(deviceID)
}

// GetActiveProfile returns the current profile.
func (l *Loader) GetActiveProfile() (*ActiveProfile, error) {
	// Expand the active profile path.
	path := filepath.Join(l.configDir, ActiveProfileFileName)

	// Check if the file exists.
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoActiveProfileFilePresent
		}

		return nil, err
	}

	// Decode the file
	var c ActiveProfile
	if err := hclsimple.DecodeFile(path, nil, &c); err != nil {
		return nil, err
	}

	// Check if no profile has been set.
	if c.Name == "" {
		return nil, ErrActiveProfileFileEmpty
	}

	c.dir = l.configDir
	return &c, nil
}

// DefaultActiveProfile returns an active profile set to default.
func (l *Loader) DefaultActiveProfile() *ActiveProfile {
	return &ActiveProfile{
		Name: ProfileNameDefault,
		dir:  l.configDir,
	}
}

// ListProfiles returns the available profile names.
func (l *Loader) ListProfiles() ([]string, error) {
	files, err := os.ReadDir(l.profilesDir)
	if err != nil {
		return nil, fmt.Errorf("unable to list profiles: %w", err)
	}

	profiles := make([]string, 0, len(files))
	for _, file := range files {
		n := file.Name()
		if file.IsDir() {
			return nil, fmt.Errorf("unexpected directory %q in profile %q directory. Please delete to recover", n, l.configDir)
		}

		if !strings.HasSuffix(n, ".hcl") {
			return nil, fmt.Errorf("unexpected non-hcl file %q in profile %q directory. Please delete to recover", n, l.configDir)
		}

		profiles = append(profiles, strings.TrimSuffix(n, ".hcl"))
	}

	return profiles, nil
}

// LoadProfile loads a profile given its name. If the profile can not be found,
// ErrNoProfileFilePresent will be returned. Otherwise, an error will be
// returned if the profile is invalid.
func (l *Loader) LoadProfile(name string) (*Profile, error) {
	// Expand the directory.
	path := filepath.Join(l.profilesDir, fmt.Sprintf("%s.hcl", name))

	// Check that the profile exists.
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoProfileFilePresent
		}

		return nil, err
	}

	// Decode the profile.
	var c Profile
	if err := hclsimple.DecodeFile(path, nil, &c); err != nil {
		return nil, fmt.Errorf("failed to decode profile: %w", err)
	}

	// Validate the name matches in the path and file.
	if name != c.Name {
		return nil, fmt.Errorf("profile path name does not match name in file. %q versus %q. Please rename file or name within the profile file to reconcile", name, c.Name)
	}

	// If there's no default organization set, use the environment variable if it's set.
	if c.DefaultOrganization == "" {
		if orgID, ok := os.LookupEnv(envVarOrganization); ok && orgID != "" {
			c.DefaultOrganization = orgID
		}
	}

	// If there's no token set, check the credentials file and environment variables.
	if c.Token == "" {
		credsToken, err := tokenFromCredentials(c.Hostname)
		if err != nil {
			return nil, err
		}
		c.tokenFromEnv = credsToken
	}

	if c.Token == "" {
		if envToken := os.Getenv(profileTokenEnvVar(c.Name)); envToken != "" {
			c.tokenFromEnv = envToken
		}
	}

	if c.Token == "" {
		if envToken := os.Getenv(legacyTokenEnvVar(c.Hostname)); envToken != "" {
			c.tokenFromEnv = envToken
		}
	}

	hostCacheDir := filepath.Join(l.configDir, "caches")
	c.dir = l.profilesDir
	c.hostCacheDir = hostCacheDir

	return &c, nil
}

// LoadProfiles loads all the available profiles.
func (l *Loader) LoadProfiles() ([]*Profile, error) {
	profileNames, err := l.ListProfiles()
	if err != nil {
		return nil, err
	}

	var profiles []*Profile
	for _, n := range profileNames {
		p, err := l.LoadProfile(n)
		if err != nil {
			return nil, fmt.Errorf("failed to load profile %q: %w", n, err)
		}
		profiles = append(profiles, p)
	}

	return profiles, nil
}

// DeleteProfile deletes the profile with the given name. If the profile can not be found,
// ErrNoProfileFilePresent will be returned. Otherwise, an error will be
// returned if the profile can not be deleted for any other reason..
func (l *Loader) DeleteProfile(name string) error {
	// Expand the directory.
	path := filepath.Join(l.profilesDir, fmt.Sprintf("%s.hcl", name))

	// Try to delete the file
	err := os.Remove(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNoProfileFilePresent
		}

		return err
	}

	return nil
}

const (
	envVarHostname           = "TFCTL_HOSTNAME"
	envVarOrganization       = "TFCTL_ORGANIZATION"
	envVarToken              = "TFCTL_TOKEN"
	envVarTokenProfileFormat = "TFCTL_TOKEN_%s"
)

// DefaultProfile returns the minimal default profile. If environment
// variables related to organization and project are set, they are honored here.
func (l *Loader) DefaultProfile() *Profile {
	profile, err := l.NewProfile(ProfileNameDefault)
	if err != nil {
		panic("The default profile should always be valid. This is always a developer error: " + err.Error())
	}

	org, orgOK := os.LookupEnv(envVarOrganization)
	if orgOK {
		profile.DefaultOrganization = org
	}

	hostname := DefaultHostname
	if envHostname, ok := os.LookupEnv(envVarHostname); ok && envHostname != "" {
		hostname = envHostname
	}

	profile.Hostname = hostname

	return profile
}

// NewProfile returns an new profile with defaults.
func (l *Loader) NewProfile(name string) (*Profile, error) {
	p := &Profile{
		Name: name,
		dir:  l.profilesDir,
	}

	return p, p.Validate()
}

func normalizeHostname(hostname string) string {
	hostname = strings.TrimSpace(hostname)
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimRight(hostname, "/")
	if asciiHost, err := idna.Lookup.ToASCII(hostname); err == nil {
		return asciiHost
	}
	return hostname
}

func profileTokenEnvVar(profileName string) string {
	if profileName == "" || profileName == "default" {
		return envVarToken
	}
	return fmt.Sprintf(envVarTokenProfileFormat, profileName)
}

func legacyTokenEnvVar(hostname string) string {
	hostname = normalizeHostname(hostname)

	var b strings.Builder
	b.WriteString("TF_TOKEN_")
	for _, r := range strings.ToUpper(hostname) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	return b.String()
}

type credentialsFile struct {
	Credentials map[string]struct {
		Token string `json:"token"`
	} `json:"credentials"`
}

func tokenFromCredentials(hostname string) (string, error) {
	path, err := homedir.Expand(TerraformCredentialsPath)
	if err != nil {
		return "", fmt.Errorf("error expanding %s config directory path %q: %w", version.Name, TerraformCredentialsPath, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}

	hostname = normalizeHostname(hostname)
	entry, ok := creds.Credentials[hostname]
	if !ok {
		return "", nil
	}

	return entry.Token, nil
}
