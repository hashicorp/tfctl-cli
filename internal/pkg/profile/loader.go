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

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/mitchellh/go-homedir"

	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/version"
)

var (
	// defaultConfigDir is the default directory that contains CLI
	// configuration when TFCTL_CONFIG_DIR is not set.
	defaultConfigDir = fmt.Sprintf("~/.config/%s/", version.Name)
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

// ConfigDir returns the resolved CLI configuration directory. It honors the
// TFCTL_CONFIG_DIR override and expands a leading ~ to the user's home
// directory. This is the single source of truth for where tfctl reads and
// writes configuration, so every consumer (profiles, exec sessions, caches)
// resolves the same location.
func ConfigDir() (string, error) {
	dir := defaultConfigDir
	if envDir := os.Getenv(envVarConfigDir); envDir != "" {
		dir = envDir
	}
	path, err := homedir.Expand(dir)
	if err != nil {
		return "", fmt.Errorf("error expanding %s config directory path %q: %w", version.Name, dir, err)
	}
	return path, nil
}

// NewLoader returns a new loader or an error if the loader can't be
// instantiated. The configuration directory defaults to ~/.config/tfctl but can
// be overridden with the TFCTL_CONFIG_DIR environment variable.
func NewLoader() (*Loader, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	return newLoader(dir)
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
func (l *Loader) LoadProfile(ctx context.Context, name string) (*Profile, error) {
	logger := logging.FromContext(ctx)
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
			logger.Debug("Setting default_organization from "+envVarOrganization, "organization", orgID)
			c.DefaultOrganization = orgID
		}
	}

	// If there's no token set, check the credentials file and environment variables. These are
	// checked in a careful order of precedence.

	if c.Token != "" {
		logger.Debug("Using token from profile", "name", c.Name)
	}

	// 1. Check for a token specific to tfctl (TFCTL_TOKEN_{profileName} or TFCTL_TOKEN for the default profile)
	if c.GetToken() == "" {
		if envToken := os.Getenv(profileTokenEnvVar(c.Name)); envToken != "" {
			logger.Debug("Setting token from environment", "var", profileTokenEnvVar(c.Name))
			c.tokenFromEnv = envToken
		}
	}

	// 2. Check for a token in the terraform credentials file that matches the hostname of the profile
	if c.GetToken() == "" {
		credsToken, err := tokenFromCredentials(c.GetHostname())
		if err != nil {
			return nil, err
		}
		if credsToken != "" {
			logger.Debug("Setting token from terraform credentials file", "hostname", c.GetHostname())
			c.tokenFromEnv = credsToken
		}
	}

	// 3. Check for a token in a terraform environment variable that matches the hostname of the
	// profile (support for TF_TOKEN_{host}).
	if c.GetToken() == "" {
		if envToken := tokenFromTerraformEnv(c.GetHostname()); envToken != "" {
			logger.Debug("Setting token from terraform environment", "hostname", c.GetHostname())
			c.tokenFromEnv = envToken
		}
	}

	hostCacheDir := filepath.Join(l.configDir, "caches")
	c.dir = l.profilesDir
	c.hostCacheDir = hostCacheDir

	return &c, nil
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

	// envVarConfigDir overrides the CLI configuration directory. It lets
	// callers (e.g. test harnesses and evals) point tfctl at a throwaway
	// config dir instead of ~/.config/tfctl, so they never read or mutate a
	// developer's real profiles.
	envVarConfigDir = "TFCTL_CONFIG_DIR"
)

// DefaultProfile returns the minimal default profile. If environment
// variables related to organization and project are set, they are honored here.
func (l *Loader) DefaultProfile(ctx context.Context) *Profile {
	logger := logging.FromContext(ctx)
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
		hostnameNormal, err := NormalizeHostname(envHostname)
		if err != nil {
			logger.Debug("Invalid hostname set by environment (using default)", "error", err)
			hostnameNormal = DefaultHostname
		} else {
			logger.Debug("Using hostname from "+envVarHostname, "hostname", hostnameNormal)
		}
		hostname = hostnameNormal
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

func profileTokenEnvVar(profileName string) string {
	if profileName == "" || profileName == "default" {
		return envVarToken
	}
	return fmt.Sprintf(envVarTokenProfileFormat, profileName)
}

// tokenFromTerraformEnv returns the token from a Terraform-style TF_TOKEN_<host>
// environment variable that matches the given hostname, mirroring Terraform CLI's
// resolution. Terraform scans every environment variable with the TF_TOKEN_ prefix
// and decodes the remainder of the name back into a hostname: double underscores
// become hyphens and any remaining single underscore becomes a period. This means a
// single hostname may be expressed by several variable names (for example the
// punycode host xn--caf-dma.fr can be written as TF_TOKEN_xn--caf-dma_fr,
// TF_TOKEN_xn--caf-dma.fr, or TF_TOKEN_xn____caf__dma_fr). If multiple variables
// resolve to the same hostname, the one defined last wins.
// See https://developer.hashicorp.com/terraform/cli/config/config-file#environment-variable-credentials
func tokenFromTerraformEnv(hostname string) string {
	target, err := NormalizeHostname(hostname)
	if err != nil {
		return ""
	}
	// Terraform's encoding can only produce periods (via single underscores), so a
	// hostname's port separator (":") is indistinguishable from a period once
	// encoded. Normalize both sides to periods so ported hosts like
	// app.terraform.io:8443 still match TF_TOKEN_app_terraform_io_8443.
	target = normalizeTerraformTokenHost(target)

	const prefix = "TF_TOKEN_"
	var token string
	for _, env := range os.Environ() {
		name, value, ok := strings.Cut(env, "=")
		if !ok || !strings.HasPrefix(name, prefix) {
			continue
		}

		// Decode Terraform's encoding of the hostname portion: double underscores
		// are hyphens, and any remaining single underscore is a period.
		rawHost := name[len(prefix):]
		rawHost = strings.ReplaceAll(rawHost, "__", "-")
		rawHost = strings.ReplaceAll(rawHost, "_", ".")

		candidate, err := NormalizeHostname(rawHost)
		if err != nil {
			continue
		}
		if normalizeTerraformTokenHost(candidate) == target {
			// Keep going so the last-defined matching variable wins.
			token = value
		}
	}
	return token
}

// normalizeTerraformTokenHost lowercases a hostname and treats the port separator
// as a period so that encoded and decoded forms compare equal.
func normalizeTerraformTokenHost(hostname string) string {
	return strings.ReplaceAll(strings.ToLower(hostname), ":", ".")
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

	hostname, err = NormalizeHostname(hostname)
	if err != nil {
		return "", err
	}

	entry, ok := creds.Credentials[hostname]
	if !ok {
		return "", nil
	}

	return entry.Token, nil
}
