package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// jsonConfig mirrors the JSON config file schema. Pointer fields let us
// distinguish "not present in the file" from "present with a zero value",
// which matters for correct precedence layering in Load.
type jsonConfig struct {
	Server     *string `json:"server"`
	Port       *int    `json:"port"`
	Username   *string `json:"username"`
	Password   *string `json:"password"`
	Resource   *string `json:"resource"`
	ArchiveDir *string `json:"archivedir"`
	PanRun     *string `json:"pan_run"`
	PanRunLog  *string `json:"pan_run_log"`
	Retry      *bool   `json:"retry"`
	UseTLS     *bool   `json:"use_tls"`
}

// loadJSONFile reads and parses the JSON config file at path. If the file
// doesn't exist and required is false, it returns (nil, nil) rather than an
// error — the caller is using the default path and simply has no config
// file, which is fine. If required is true (the caller explicitly asked for
// this path), a missing file is an error.
func loadJSONFile(path string, required bool) (*jsonConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var jc jsonConfig
	if err := json.Unmarshal(data, &jc); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}
	return &jc, nil
}

// applyJSON overlays any fields present in jc onto cfg. A nil jc is a no-op.
func applyJSON(cfg *Config, jc *jsonConfig) {
	if jc == nil {
		return
	}
	if jc.Server != nil {
		cfg.Server = *jc.Server
	}
	if jc.Port != nil {
		cfg.Port = *jc.Port
	}
	if jc.Username != nil {
		cfg.Username = *jc.Username
	}
	if jc.Password != nil {
		cfg.Password = *jc.Password
	}
	if jc.Resource != nil {
		cfg.Resource = *jc.Resource
	}
	if jc.ArchiveDir != nil {
		cfg.ArchiveDir = *jc.ArchiveDir
	}
	if jc.PanRun != nil {
		cfg.PanRun = *jc.PanRun
	}
	if jc.PanRunLog != nil {
		cfg.PanRunLog = *jc.PanRunLog
	}
	if jc.Retry != nil {
		cfg.Retry = *jc.Retry
	}
	if jc.UseTLS != nil {
		cfg.UseTLS = *jc.UseTLS
	}
}
