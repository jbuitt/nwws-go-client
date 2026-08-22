package config

import "testing"

func TestApplyEnv_Overrides(t *testing.T) {
	t.Setenv("NWWS_SERVER", "env-server")
	t.Setenv("NWWS_PORT", "9999")
	t.Setenv("NWWS_USERNAME", "env-user")
	t.Setenv("NWWS_PASSWORD", "env-pass")
	t.Setenv("NWWS_RESOURCE", "env-resource")
	t.Setenv("NWWS_ARCHIVEDIR", "/tmp/env-products")
	t.Setenv("NWWS_PAN_RUN", "/usr/local/bin/pan.sh")
	t.Setenv("NWWS_PAN_RUN_LOG", "/tmp/pan.log")
	t.Setenv("NWWS_RETRY", "false")
	t.Setenv("NWWS_USE_TLS", "false")

	cfg := Defaults()
	if err := applyEnv(&cfg); err != nil {
		t.Fatalf("applyEnv: %v", err)
	}

	if cfg.Server != "env-server" {
		t.Errorf("Server = %q, want env-server", cfg.Server)
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want 9999", cfg.Port)
	}
	if cfg.Username != "env-user" {
		t.Errorf("Username = %q, want env-user", cfg.Username)
	}
	if cfg.Password != "env-pass" {
		t.Errorf("Password = %q, want env-pass", cfg.Password)
	}
	if cfg.Resource != "env-resource" {
		t.Errorf("Resource = %q, want env-resource", cfg.Resource)
	}
	if cfg.ArchiveDir != "/tmp/env-products" {
		t.Errorf("ArchiveDir = %q, want /tmp/env-products", cfg.ArchiveDir)
	}
	if cfg.PanRun != "/usr/local/bin/pan.sh" {
		t.Errorf("PanRun = %q, want /usr/local/bin/pan.sh", cfg.PanRun)
	}
	if cfg.PanRunLog != "/tmp/pan.log" {
		t.Errorf("PanRunLog = %q, want /tmp/pan.log", cfg.PanRunLog)
	}
	if cfg.Retry {
		t.Error("Retry = true, want false")
	}
	if cfg.UseTLS {
		t.Error("UseTLS = true, want false")
	}
}

func TestApplyEnv_InvalidPort(t *testing.T) {
	t.Setenv("NWWS_PORT", "not-a-number")
	cfg := Defaults()
	if err := applyEnv(&cfg); err == nil {
		t.Fatal("applyEnv: expected error for invalid NWWS_PORT")
	}
}

func TestApplyEnv_InvalidRetry(t *testing.T) {
	t.Setenv("NWWS_RETRY", "not-a-bool")
	cfg := Defaults()
	if err := applyEnv(&cfg); err == nil {
		t.Fatal("applyEnv: expected error for invalid NWWS_RETRY")
	}
}
