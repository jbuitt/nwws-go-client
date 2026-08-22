package config

import (
	"fmt"
	"os"
	"strconv"
)

// applyEnv overlays any set NWWS_* environment variables onto cfg.
func applyEnv(cfg *Config) error {
	if v, ok := os.LookupEnv("NWWS_SERVER"); ok {
		cfg.Server = v
	}
	if v, ok := os.LookupEnv("NWWS_PORT"); ok {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid NWWS_PORT %q: %w", v, err)
		}
		cfg.Port = p
	}
	if v, ok := os.LookupEnv("NWWS_USERNAME"); ok {
		cfg.Username = v
	}
	if v, ok := os.LookupEnv("NWWS_PASSWORD"); ok {
		cfg.Password = v
	}
	if v, ok := os.LookupEnv("NWWS_RESOURCE"); ok {
		cfg.Resource = v
	}
	if v, ok := os.LookupEnv("NWWS_ARCHIVEDIR"); ok {
		cfg.ArchiveDir = v
	}
	if v, ok := os.LookupEnv("NWWS_PAN_RUN"); ok {
		cfg.PanRun = v
	}
	if v, ok := os.LookupEnv("NWWS_PAN_RUN_LOG"); ok {
		cfg.PanRunLog = v
	}
	if v, ok := os.LookupEnv("NWWS_RETRY"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid NWWS_RETRY %q: %w", v, err)
		}
		cfg.Retry = b
	}
	if v, ok := os.LookupEnv("NWWS_USE_TLS"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid NWWS_USE_TLS %q: %w", v, err)
		}
		cfg.UseTLS = b
	}
	return nil
}
