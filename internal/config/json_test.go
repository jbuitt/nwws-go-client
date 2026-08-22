package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJSONFile_MissingNotRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	jc, err := loadJSONFile(path, false)
	if err != nil {
		t.Fatalf("loadJSONFile: unexpected error: %v", err)
	}
	if jc != nil {
		t.Errorf("loadJSONFile: got %+v, want nil", jc)
	}
}

func TestLoadJSONFile_MissingRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	_, err := loadJSONFile(path, true)
	if err == nil {
		t.Fatal("loadJSONFile: expected error for a required-but-missing file")
	}
}

func TestLoadJSONFile_Invalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, err := loadJSONFile(path, false)
	if err == nil {
		t.Fatal("loadJSONFile: expected error for malformed JSON")
	}
}

func TestLoadJSONFile_ValidAndApplyJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	content := `{
		"server": "json-server",
		"port": 1234,
		"username": "json-user",
		"archivedir": "/tmp/json-products"
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	jc, err := loadJSONFile(path, false)
	if err != nil {
		t.Fatalf("loadJSONFile: %v", err)
	}
	if jc == nil {
		t.Fatal("loadJSONFile: got nil, want a populated jsonConfig")
	}

	cfg := Defaults()
	applyJSON(&cfg, jc)

	if cfg.Server != "json-server" {
		t.Errorf("Server = %q, want json-server", cfg.Server)
	}
	if cfg.Port != 1234 {
		t.Errorf("Port = %d, want 1234", cfg.Port)
	}
	if cfg.Username != "json-user" {
		t.Errorf("Username = %q, want json-user", cfg.Username)
	}
	if cfg.ArchiveDir != "/tmp/json-products" {
		t.Errorf("ArchiveDir = %q, want /tmp/json-products", cfg.ArchiveDir)
	}
	// Retry was not present in the JSON, so the default must survive.
	if !cfg.Retry {
		t.Error("Retry = false, want true (default, since JSON didn't set it)")
	}
}

func TestApplyJSON_NilIsNoOp(t *testing.T) {
	cfg := Defaults()
	want := cfg
	applyJSON(&cfg, nil)
	if cfg != want {
		t.Errorf("applyJSON with nil jsonConfig changed cfg: got %+v, want %+v", cfg, want)
	}
}
