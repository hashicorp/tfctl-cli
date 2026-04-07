package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileTokenEnvVar(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":         "TFCLOUD_TOKEN",
		"default":  "TFCLOUD_TOKEN",
		"work":     "TFCLOUD_TOKEN_work",
		"dev_2026": "TFCLOUD_TOKEN_dev_2026",
	}

	for input, want := range tests {
		if got := profileTokenEnvVar(input); got != want {
			t.Fatalf("profileTokenEnvVar(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLegacyTokenEnvVar(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"app.terraform.io":      "TF_TOKEN_APP_TERRAFORM_IO",
		"tfe.example-host.com":  "TF_TOKEN_TFE_EXAMPLE_HOST_COM",
		"xn--bcher-kva.example": "TF_TOKEN_XN__BCHER_KVA_EXAMPLE",
		"xn--caf-dma.fr":        "TF_TOKEN_XN__CAF_DMA_FR",
		"app.terraform.io:443":  "TF_TOKEN_APP_TERRAFORM_IO_443",
	}

	for input, want := range tests {
		if got := legacyTokenEnvVar(input); got != want {
			t.Fatalf("legacyTokenEnvVar(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLoadUsesDefaultProfileAndEnvToken(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.eu.terraform.io" {
  token = "user-token"
  organization = "ops"
}`)
	t.Setenv("TFCLOUD_TOKEN", "env-token")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hostname != "app.eu.terraform.io" {
		t.Fatalf("hostname = %q", cfg.Hostname)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("token = %q", cfg.Token)
	}
	if cfg.DefaultOrganization != "ops" {
		t.Fatalf("default organization = %q", cfg.DefaultOrganization)
	}
}

func TestLoadUsesLayeredHCLPrecedence(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.terraform.io" {
  token        = "user-token"
  organization = "user-org"
}`)
	env.writeLocalConfig(`profile "default" "app.terraform.io" {
  organization = "local-org"
}`)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hostname != "app.terraform.io" {
		t.Fatalf("hostname = %q", cfg.Hostname)
	}
	if cfg.Token != "user-token" {
		t.Fatalf("token = %q", cfg.Token)
	}
	if cfg.DefaultOrganization != "local-org" {
		t.Fatalf("default organization = %q", cfg.DefaultOrganization)
	}
	if got := cfg.DefaultHeaders.Get("User-Agent"); !strings.HasPrefix(got, "tfcloud CLI ") {
		t.Fatalf("user-agent = %q", got)
	}
}

func TestLoadUsesLocalDefaultHostnameWhenNotExplicit(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.terraform.io" {
  token = "user-token"
}`)
	env.writeLocalConfig(`profile "default" "app.eu.terraform.io" {
  token        = "local-token"
  organization = "local-org"
}`)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hostname != "app.eu.terraform.io" {
		t.Fatalf("hostname = %q", cfg.Hostname)
	}
	if cfg.Token != "local-token" {
		t.Fatalf("token = %q", cfg.Token)
	}
	if cfg.DefaultOrganization != "local-org" {
		t.Fatalf("default organization = %q", cfg.DefaultOrganization)
	}
}

func TestLoadFallsBackToCredentialsWhenHCLTokenMissing(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.terraform.io" {
  organization = "from-hcl"
}`)
	env.writeCredentials(`{"credentials":{"app.terraform.io":{"token":"cred-token"}}}`)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "cred-token" {
		t.Fatalf("token = %q", cfg.Token)
	}
	if cfg.DefaultOrganization != "from-hcl" {
		t.Fatalf("default organization = %q", cfg.DefaultOrganization)
	}
}

func TestLoadEnvOverridesCredentialsAndHCLToken(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.terraform.io" {
  token = "user-token"
}`)
	env.writeCredentials(`{"credentials":{"app.terraform.io":{"token":"cred-token"}}}`)
	t.Setenv("TFCLOUD_TOKEN", "env-token")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("token = %q", cfg.Token)
	}
}

func TestLoadIgnoresNamedProfileEnvTokenWhenDefaultProfileIsActive(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.terraform.io" {
  organization = "ops"
}`)
	t.Setenv("TFCLOUD_TOKEN_work", "env-token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing token") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadDefaultsHostnameWithoutConfig(t *testing.T) {
	env := newConfigTestEnv(t)
	t.Setenv("TFCLOUD_TOKEN", "env-token")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hostname != DefaultHostname {
		t.Fatalf("hostname = %q", cfg.Hostname)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("token = %q", cfg.Token)
	}
	_ = env
	if cfg.DefaultOrganization != "" {
		t.Fatalf("default organization = %q", cfg.DefaultOrganization)
	}
}

func TestLoadFallsBackToLegacyHostnameEnvVar(t *testing.T) {
	env := newConfigTestEnv(t)
	t.Setenv("TF_TOKEN_APP_TERRAFORM_IO", "legacy-token")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "legacy-token" {
		t.Fatalf("token = %q", cfg.Token)
	}
	_ = env
}

func TestLoadRejectsMalformedHCL(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.terraform.io" {`)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsUnsupportedProfileName(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "bad-name" "app.terraform.io" {
  token = "token"
}`)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadErrorsWhenTokenStillMissing(t *testing.T) {
	env := newConfigTestEnv(t)
	env.writeUserConfig(`profile "default" "app.terraform.io" {
  organization = "bcroft"
}`)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing token") {
		t.Fatalf("error = %v", err)
	}
}

type configTestEnv struct {
	t             *testing.T
	root          string
	home          string
	userConfigDir string
	originalWD    string
}

func newConfigTestEnv(t *testing.T) *configTestEnv {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	workingDir := filepath.Join(home, "work")
	xdgConfigHome := filepath.Join(home, ".config")
	for _, dir := range []string{home, workingDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	t.Logf("XDG_CONFIG_HOME = %q", os.Getenv("XDG_CONFIG_HOME"))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	userConfigDir, err := userConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}

	return &configTestEnv{t: t, root: root, home: home, userConfigDir: userConfigDir, originalWD: originalWD}
}

func (e *configTestEnv) writeUserConfig(body string) {
	e.t.Helper()
	path := filepath.Join(e.userConfigDir, "tfcloud", "tfcloud.hcl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *configTestEnv) writeLocalConfig(body string) {
	e.t.Helper()
	if err := os.WriteFile(".tfcloud.hcl", []byte(body), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *configTestEnv) writeCredentials(body string) {
	e.t.Helper()
	path := filepath.Join(e.home, ".terraform.d", "credentials.tfrc.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		e.t.Fatal(err)
	}
}
