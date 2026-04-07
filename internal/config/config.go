// Package config resolves CLI configuration and build metadata.
package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"golang.org/x/net/idna"
)

// DefaultHostname is the hostname used when no default profile is configured.
const DefaultHostname = "app.terraform.io"

// Config is the fully resolved CLI configuration after all configuration
// layers have been applied.
type Config struct {
	// Hostname is the HCP Terraform hostname to target.
	Hostname string
	// DefaultOrganization is the fallback organization for commands that need one.
	DefaultOrganization string
	// Token is the resolved API token used for authentication.
	Token string
	// DefaultHeaders are added to every outgoing API request.
	DefaultHeaders http.Header
}

type credentialsFile struct {
	Credentials map[string]struct {
		Token string `json:"token"`
	} `json:"credentials"`
}

type hclConfig struct {
	Profiles []hclProfile `hcl:"profile,block"`
}

type hclProfile struct {
	Name         string  `hcl:",label"`
	Hostname     string  `hcl:",label"`
	Token        *string `hcl:"token,optional"`
	Organization *string `hcl:"organization,optional"`
}

type fileConfig struct {
	ProfileName         string
	Hostname            string
	Token               string
	DefaultOrganization string
}

type resolvedProfile struct {
	Name     string
	Hostname string
}

// Load resolves CLI configuration from config files, credentials, and
// environment variables using the default profile.
func Load() (*Config, error) {
	profile, err := resolveProfile()
	if err != nil {
		return nil, err
	}

	resolved := fileConfig{ProfileName: profile.Name, Hostname: profile.Hostname}
	for _, path := range configSearchPaths() {
		cfg, err := loadHCLConfig(path, profile)
		if err != nil {
			return nil, err
		}
		resolved = mergeConfig(resolved, cfg)
	}

	if resolved.Token == "" {
		resolved.Token, err = tokenFromCredentials(profile.Hostname)
		if err != nil {
			return nil, err
		}
	}

	if envToken := os.Getenv(profileTokenEnvVar(resolved.ProfileName)); envToken != "" {
		resolved.Token = envToken
	} else if resolved.Token == "" {
		resolved.Token = os.Getenv(legacyTokenEnvVar(profile.Hostname))
	}

	if resolved.Token == "" {
		return nil, fmt.Errorf("missing token for %s; set %s, add it to tfcloud.hcl, add it to ~/.terraform.d/credentials.tfrc.json, or use terraform-compatible %s", profile.Hostname, profileTokenEnvVar(resolved.ProfileName), legacyTokenEnvVar(profile.Hostname))
	}

	headers := make(http.Header)
	headers.Set("User-Agent", fmt.Sprintf("tfcloud CLI %s", Version))

	return &Config{
		Hostname:            profile.Hostname,
		DefaultOrganization: resolved.DefaultOrganization,
		Token:               resolved.Token,
		DefaultHeaders:      headers,
	}, nil
}

// resolveProfile selects the active default profile for configuration loading.
func resolveProfile() (resolvedProfile, error) {
	profile := resolvedProfile{Name: "default"}
	for _, path := range configSearchPaths() {
		cfg, err := loadDefaultProfile(path)
		if err != nil {
			return resolvedProfile{}, err
		}
		if cfg.Hostname != "" {
			profile.Name = cfg.ProfileName
			profile.Hostname = cfg.Hostname
		}
	}
	if profile.Hostname == "" {
		profile.Hostname = DefaultHostname
	}
	return profile, nil
}

func configSearchPaths() []string {
	paths := make([]string, 0, 2)
	if userConfigDir, err := userConfigDir(); err == nil {
		paths = append(paths, filepath.Join(userConfigDir, "tfcloud", "tfcloud.hcl"))
	}
	paths = append(paths, ".tfcloud.hcl")
	return paths
}

// userConfigDir returns the base directory for user-specific configuration files. On
// most platforms, this the os.UserConfigDir(), but on darwin, if XDG_CONFIG_HOME is not set,
// this falls back to $HOME/.config to align with other unix systems.
func userConfigDir() (string, error) {
	if runtime.GOOS == "darwin" && os.Getenv("XDG_CONFIG_HOME") == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config"), nil
	}
	return os.UserConfigDir()
}

func loadHCLConfig(path string, target resolvedProfile) (fileConfig, error) {
	decoded, err := readHCLConfig(path)
	if err != nil {
		return fileConfig{}, err
	}

	selected, err := selectProfileByName(decoded.Profiles, target.Name)
	if err != nil {
		return fileConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if selected == nil {
		return fileConfig{}, nil
	}

	return profileToFileConfig(*selected), nil
}

func loadDefaultProfile(path string) (fileConfig, error) {
	decoded, err := readHCLConfig(path)
	if err != nil {
		return fileConfig{}, err
	}

	selected, err := selectProfileByName(decoded.Profiles, "default")
	if err != nil {
		return fileConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if selected == nil {
		return fileConfig{}, nil
	}

	return profileToFileConfig(*selected), nil
}

func readHCLConfig(path string) (hclConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return hclConfig{}, nil
		}
		return hclConfig{}, err
	}

	var decoded hclConfig
	if err := hclsimple.Decode(path, data, nil, &decoded); err != nil {
		return hclConfig{}, formatHCLError(path, err)
	}
	return decoded, nil
}

// selectProfileByName returns the matching profile block for name after
// validating supported profile names and normalizing hostnames.
func selectProfileByName(profiles []hclProfile, name string) (*hclProfile, error) {
	var selected *hclProfile
	for i := range profiles {
		profile := profiles[i]
		if err := validateProfileName(profile.Name); err != nil {
			return nil, err
		}
		profile.Hostname = normalizeHostname(profile.Hostname)
		if profile.Name == name {
			selected = &profile
		}
	}
	return selected, nil
}

// profileToFileConfig converts a decoded HCL profile block into the internal
// file-backed configuration shape used during layering.
func profileToFileConfig(profile hclProfile) fileConfig {
	cfg := fileConfig{ProfileName: profile.Name, Hostname: profile.Hostname}
	if profile.Token != nil {
		cfg.Token = *profile.Token
	}
	if profile.Organization != nil {
		cfg.DefaultOrganization = *profile.Organization
	}
	return cfg
}

func mergeConfig(base fileConfig, overlay fileConfig) fileConfig {
	if overlay.ProfileName != "" {
		base.ProfileName = overlay.ProfileName
	}
	if overlay.Hostname != "" {
		base.Hostname = overlay.Hostname
	}
	if overlay.Token != "" {
		base.Token = overlay.Token
	}
	if overlay.DefaultOrganization != "" {
		base.DefaultOrganization = overlay.DefaultOrganization
	}
	return base
}

func validateProfileName(name string) error {
	if name == "default" {
		return nil
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return fmt.Errorf("invalid profile name %q; only letters, digits, and underscores are supported", name)
	}
	return nil
}

func formatHCLError(path string, err error) error {
	if diags, ok := err.(hcl.Diagnostics); ok {
		return fmt.Errorf("parse %s: %s", path, strings.TrimSpace(diags.Error()))
	}
	return fmt.Errorf("parse %s: %w", path, err)
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
		return "TFCLOUD_TOKEN"
	}
	return "TFCLOUD_TOKEN_" + profileName
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

func tokenFromCredentials(hostname string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(home, ".terraform.d", "credentials.tfrc.json")
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
