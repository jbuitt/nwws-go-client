package config

import (
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
