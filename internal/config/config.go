package config

import "math/rand/v2"

// Config holds all settings the client needs, resolved from CLI flags,
// environment variables, and an optional JSON file (see Load).
type Config struct {
	Server     string
	Port       int
	Username   string
	Password   string
	Resource   string
	ArchiveDir string
	PanRun     string
	PanRunLog  string
	Retry      bool
	UseTLS     bool
}

const (
	DefaultServer     = "nwws-oi.weather.gov"
	DefaultPort       = 5222
	DefaultArchiveDir = "./products/"
	DefaultRetry      = true
	DefaultUseTLS     = true
	DefaultConfigPath = "./config.json"
)

// Defaults returns a Config populated with the documented default values.
// Username and Password are left empty since they have no default and are
// required to be supplied by the caller.
func Defaults() Config {
	return Config{
		Server:     DefaultServer,
		Port:       DefaultPort,
		ArchiveDir: DefaultArchiveDir,
		Retry:      DefaultRetry,
		UseTLS:     DefaultUseTLS,
		Resource:   "nwws-go-client-" + randomSuffix(5),
	}
}

func randomSuffix(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rand.IntN(len(alphabet))]
	}
	return string(b)
}
