package config

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server != "nwws-oi.weather.gov" {
		t.Errorf("Server = %q, want nwws-oi.weather.gov", cfg.Server)
	}
	if cfg.Port != 5222 {
		t.Errorf("Port = %d, want 5222", cfg.Port)
	}
	if cfg.ArchiveDir != "./products/" {
		t.Errorf("ArchiveDir = %q, want ./products/", cfg.ArchiveDir)
	}
	if !cfg.Retry {
		t.Error("Retry = false, want true")
	}
	if !cfg.UseTLS {
		t.Error("UseTLS = false, want true")
	}
	if cfg.Username != "" || cfg.Password != "" || cfg.PanRun != "" || cfg.PanRunLog != "" {
		t.Error("Username, Password, PanRun, and PanRunLog should default to empty")
	}

	resourcePattern := regexp.MustCompile(`^nwws-go-client-[A-Za-z0-9]{5}$`)
	if !resourcePattern.MatchString(cfg.Resource) {
		t.Errorf("Resource = %q, does not match expected pattern nwws-go-client-XXXXX", cfg.Resource)
	}
}

func TestDefaults_RandomResourceVaries(t *testing.T) {
	a := Defaults().Resource
	b := Defaults().Resource
	if a == b {
		t.Errorf("expected two calls to Defaults() to generate different resources, both were %q", a)
	}
}

func TestLoad_RequiresUsernameAndPassword(t *testing.T) {
	_, err := load(nil, filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error when username/password are missing")
	}
}

func TestLoad_DefaultConfigPathMissingIsNotError(t *testing.T) {
	_, err := load([]string{"-username", "u", "-password", "p"}, filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("load: unexpected error for missing default config path: %v", err)
	}
}

func TestLoad_MissingExplicitConfigFileIsError(t *testing.T) {
	dir := t.TempDir()
	_, err := load([]string{
		"-config", filepath.Join(dir, "does-not-exist.json"),
		"-username", "u",
		"-password", "p",
	}, filepath.Join(dir, "unused-default.json"))
	if err == nil {
		t.Fatal("expected error for an explicitly-passed missing config file")
	}
}

func TestLoad_Precedence(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	jsonContent := `{
		"server": "json-server",
		"port": 1111,
		"username": "json-user",
		"password": "json-pass",
		"retry": false
	}`
	if err := os.WriteFile(configPath, []byte(jsonContent), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	t.Setenv("NWWS_SERVER", "env-server")
	t.Setenv("NWWS_USERNAME", "env-user")

	cfg, err := load([]string{
		"-config", configPath,
		"-username", "flag-user",
	}, filepath.Join(dir, "unused.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Server != "env-server" {
		t.Errorf("Server = %q, want env-server (env should beat json)", cfg.Server)
	}
	if cfg.Username != "flag-user" {
		t.Errorf("Username = %q, want flag-user (flag should beat env and json)", cfg.Username)
	}
	if cfg.Password != "json-pass" {
		t.Errorf("Password = %q, want json-pass (json should beat default)", cfg.Password)
	}
	if cfg.Port != 1111 {
		t.Errorf("Port = %d, want 1111 from json", cfg.Port)
	}
	if cfg.Retry {
		t.Error("Retry = true, want false from json")
	}
}

func TestLoad_FlagExplicitlySetToZeroValueWins(t *testing.T) {
	cfg, err := load([]string{
		"-username", "u",
		"-password", "p",
		"-retry=false",
	}, filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Retry {
		t.Error("Retry = true, want false: an explicitly-passed -retry=false must win over the true default")
	}
}

func TestLoad_DebugXMPPLog(t *testing.T) {
	cfg, err := load([]string{
		"-username", "u",
		"-password", "p",
		"-debug_xmpp_log", "/tmp/wire.log",
	}, filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DebugXMPPLog != "/tmp/wire.log" {
		t.Errorf("DebugXMPPLog = %q, want /tmp/wire.log", cfg.DebugXMPPLog)
	}
}

func TestLoad_DebugXMPPLogDefaultsToDisabled(t *testing.T) {
	cfg, err := load([]string{
		"-username", "u",
		"-password", "p",
	}, filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DebugXMPPLog != "" {
		t.Errorf("DebugXMPPLog = %q, want empty (disabled by default)", cfg.DebugXMPPLog)
	}
}
