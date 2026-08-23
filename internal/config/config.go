package config

import (
	"flag"
	"fmt"
	"math/rand/v2"
)

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

	// DebugXMPPLog, if set, writes the raw XMPP wire traffic (post-STARTTLS,
	// so it includes the base64-encoded SASL auth exchange) to this file
	// path, for diagnosing protocol-level issues. Off by default.
	DebugXMPPLog string
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

// Load resolves the final Config from CLI args, environment variables, and
// an optional JSON config file, in that order of precedence (CLI args win).
func Load(args []string) (Config, error) {
	return load(args, DefaultConfigPath)
}

// load is Load with the default config file path as a parameter, so tests
// can exercise "no JSON file present" without needing to pass -config.
func load(args []string, defaultConfigPath string) (Config, error) {
	cfg := Defaults()

	fs := flag.NewFlagSet("nwws-go-client", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to JSON config file")
	server := fs.String("server", "", "NWWS server hostname")
	port := fs.Int("port", 0, "NWWS server port")
	username := fs.String("username", "", "NWWS username")
	password := fs.String("password", "", "NWWS password")
	resource := fs.String("resource", "", "XMPP resource name")
	archiveDir := fs.String("archivedir", "", "directory to store products in")
	panRun := fs.String("pan_run", "", "PAN script/executable to run after each saved product")
	panRunLog := fs.String("pan_run_log", "", "log file for PAN script output (defaults to main log)")
	retry := fs.Bool("retry", DefaultRetry, "automatically reconnect if disconnected")
	useTLS := fs.Bool("use_tls", DefaultUseTLS, "use STARTTLS when connecting")
	debugXMPPLog := fs.String("debug_xmpp_log", "", "write raw XMPP wire traffic to this file for troubleshooting (contains the base64 SASL auth exchange, so treat it as sensitive)")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	jc, err := loadJSONFile(*configPath, explicit["config"])
	if err != nil {
		return Config{}, err
	}
	applyJSON(&cfg, jc)

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}

	if explicit["server"] {
		cfg.Server = *server
	}
	if explicit["port"] {
		cfg.Port = *port
	}
	if explicit["username"] {
		cfg.Username = *username
	}
	if explicit["password"] {
		cfg.Password = *password
	}
	if explicit["resource"] {
		cfg.Resource = *resource
	}
	if explicit["archivedir"] {
		cfg.ArchiveDir = *archiveDir
	}
	if explicit["pan_run"] {
		cfg.PanRun = *panRun
	}
	if explicit["pan_run_log"] {
		cfg.PanRunLog = *panRunLog
	}
	if explicit["retry"] {
		cfg.Retry = *retry
	}
	if explicit["use_tls"] {
		cfg.UseTLS = *useTLS
	}
	if explicit["debug_xmpp_log"] {
		cfg.DebugXMPPLog = *debugXMPPLog
	}

	if cfg.Username == "" || cfg.Password == "" {
		return Config{}, fmt.Errorf("username and password are required (set via -username/-password flags, NWWS_USERNAME/NWWS_PASSWORD env vars, or the config file)")
	}

	return cfg, nil
}
