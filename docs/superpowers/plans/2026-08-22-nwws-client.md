# NWWS-OI Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI that connects to NOAA's NWWS-OI XMPP service, joins the `nwws@conference.nwws-oi.weather.gov` MUC room, parses broadcast weather products, saves them under `products/<cccc>/`, and optionally runs a PAN script per saved product.

**Architecture:** Five independently-testable packages (`config`, `product`, `store`, `pan`, `nwwsclient`) wired together by a thin `cmd/nwws-go-client/main.go`. `go-xmpp`'s router delivers message stanzas synchronously to a handler that parses and writes the product inline; only the PAN script runs in its own goroutine.

**Tech Stack:** Go (stdlib `flag`, `log/slog`, `encoding/xml`, `os/exec`), `gosrc.io/xmpp` (the `FluuxIO/go-xmpp` module) for XMPP.

**Spec:** [docs/superpowers/specs/2026-08-22-nwws-client-design.md](../specs/2026-08-22-nwws-client-design.md)

---

## Task 1: Project scaffolding

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `config.example.json`

- [ ] **Step 1: Verify the Go toolchain**

Run: `go version`
Expected: `go version go1.22` or newer (this plan uses `math/rand/v2` and `log/slog`, which need Go 1.22+). If the installed version is older than 1.22, note it now — Task 2's `randomSuffix` will need `math/rand` instead of `math/rand/v2` (same logic, swap `rand.IntN(n)` for `rand.Intn(n)`).

- [ ] **Step 2: Initialize the module**

```bash
go mod init github.com/jbuitt/nwws-go-client
```

- [ ] **Step 3: Add the XMPP dependency**

```bash
go get gosrc.io/xmpp@latest
```

- [ ] **Step 4: Create the directory layout**

```bash
mkdir -p cmd/nwws-go-client internal/config internal/product internal/store internal/pan internal/nwwsclient
```

- [ ] **Step 5: Create `.gitignore`**

```
/nwws-go-client
/products/
/config.json
*.log
```

- [ ] **Step 6: Create `config.example.json`**

```json
{
  "server": "nwws-oi.weather.gov",
  "port": 5222,
  "username": "[username]",
  "password": "[password]",
  "resource": "[resource]",
  "archivedir": "[archivedir]",
  "pan_run": "[pan_run]",
  "pan_run_log": "[pan_run_log]",
  "retry": true,
  "use_tls": true
}
```

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum .gitignore config.example.json
git commit -m "Scaffold Go module and project layout"
```

---

## Task 2: Config struct and defaults

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — `undefined: Defaults` (package doesn't exist yet)

- [ ] **Step 3: Write the implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/...`
Expected: `ok  	github.com/jbuitt/nwws-go-client/internal/config`

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Add Config struct and defaults"
```

---

## Task 3: JSON config file layer

**Files:**
- Create: `internal/config/json.go`
- Test: `internal/config/json_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — `undefined: loadJSONFile`

- [ ] **Step 3: Write the implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/...`
Expected: `ok  	github.com/jbuitt/nwws-go-client/internal/config`

- [ ] **Step 5: Commit**

```bash
git add internal/config/json.go internal/config/json_test.go
git commit -m "Add JSON config file loading layer"
```

---

## Task 4: Environment variable layer

**Files:**
- Create: `internal/config/env.go`
- Test: `internal/config/env_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — `undefined: applyEnv`

- [ ] **Step 3: Write the implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/...`
Expected: `ok  	github.com/jbuitt/nwws-go-client/internal/config`

- [ ] **Step 5: Commit**

```bash
git add internal/config/env.go internal/config/env_test.go
git commit -m "Add environment variable config layer"
```

---

## Task 5: CLI flags and full precedence (Load)

**Files:**
- Modify: `internal/config/config.go` (append `Load`/`load`)
- Modify: `internal/config/config_test.go` (append integration tests)

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
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
```

Add `"os"` and `"path/filepath"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — `undefined: load`

- [ ] **Step 3: Write the implementation**

Append to `internal/config/config.go` (add `"flag"` to the imports):

```go
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

	if cfg.Username == "" || cfg.Password == "" {
		return Config{}, fmt.Errorf("username and password are required (set via -username/-password flags, NWWS_USERNAME/NWWS_PASSWORD env vars, or the config file)")
	}

	return cfg, nil
}
```

Add `"fmt"` to the imports too (used by the validation error).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: all tests PASS, `ok  	github.com/jbuitt/nwws-go-client/internal/config`

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Add CLI flag parsing and full config precedence resolution"
```

---

## Task 6: Product parsing and filenames

**Files:**
- Create: `internal/product/product.go`
- Test: `internal/product/product_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package product

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"gosrc.io/xmpp/stanza"
)

const validMessageXML = `<message from="nwws@conference.nwws-oi.weather.gov/KKCI" to="user@nwws-oi.weather.gov/res">
  <x xmlns="nwws-oi" cccc="KKCI" ttaaii="FTUS21" awipsid="TAFKORD" issue="2026-08-22T14:32:00Z" id="12345">SAMPLE PRODUCT TEXT</x>
</message>`

func unmarshalMessage(t *testing.T, xmlStr string) stanza.Message {
	t.Helper()
	var msg stanza.Message
	if err := xml.Unmarshal([]byte(xmlStr), &msg); err != nil {
		t.Fatalf("unmarshaling test message: %v", err)
	}
	return msg
}

func TestParseMessage_Valid(t *testing.T) {
	msg := unmarshalMessage(t, validMessageXML)

	p, err := ParseMessage(msg)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	if p.CCCC != "KKCI" {
		t.Errorf("CCCC = %q, want KKCI", p.CCCC)
	}
	if p.TTAAII != "FTUS21" {
		t.Errorf("TTAAII = %q, want FTUS21", p.TTAAII)
	}
	if p.AWIPSID != "TAFKORD" {
		t.Errorf("AWIPSID = %q, want TAFKORD", p.AWIPSID)
	}
	if p.ID != "12345" {
		t.Errorf("ID = %q, want 12345", p.ID)
	}
	if strings.TrimSpace(p.Text) != "SAMPLE PRODUCT TEXT" {
		t.Errorf("Text = %q, want SAMPLE PRODUCT TEXT", p.Text)
	}
	wantIssue := time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC)
	if !p.Issue.Equal(wantIssue) {
		t.Errorf("Issue = %v, want %v", p.Issue, wantIssue)
	}
}

func TestParseMessage_NoExtension(t *testing.T) {
	msg := unmarshalMessage(t, `<message><body>just a chat message</body></message>`)

	_, err := ParseMessage(msg)
	if err == nil {
		t.Fatal("ParseMessage: expected error for a message with no nwws-oi extension")
	}
}

func TestParseMessage_MissingAttribute(t *testing.T) {
	xmlStr := `<message><x xmlns="nwws-oi" ttaaii="FTUS21" awipsid="TAFKORD" issue="2026-08-22T14:32:00Z" id="12345">TEXT</x></message>`
	msg := unmarshalMessage(t, xmlStr)

	_, err := ParseMessage(msg)
	if err == nil {
		t.Fatal("ParseMessage: expected error for missing cccc attribute")
	}
	if !strings.Contains(err.Error(), "cccc") {
		t.Errorf("error %q does not mention the missing field cccc", err.Error())
	}
}

func TestParseMessage_InvalidIssueTimestamp(t *testing.T) {
	xmlStr := `<message><x xmlns="nwws-oi" cccc="KKCI" ttaaii="FTUS21" awipsid="TAFKORD" issue="not-a-timestamp" id="12345">TEXT</x></message>`
	msg := unmarshalMessage(t, xmlStr)

	_, err := ParseMessage(msg)
	if err == nil {
		t.Fatal("ParseMessage: expected error for invalid issue timestamp")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/product/...`
Expected: FAIL — `undefined: ParseMessage`

- [ ] **Step 3: Write the implementation**

```go
package product

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"gosrc.io/xmpp/stanza"
)

// nwwsExtension is the <x xmlns="nwws-oi"> child element carrying product
// metadata and the raw product text on a MUC message stanza.
type nwwsExtension struct {
	stanza.MsgExtension
	XMLName xml.Name `xml:"nwws-oi x"`
	CCCC    string   `xml:"cccc,attr"`
	TTAAII  string   `xml:"ttaaii,attr"`
	AWIPSID string   `xml:"awipsid,attr"`
	Issue   string   `xml:"issue,attr"`
	ID      string   `xml:"id,attr"`
	Text    string   `xml:",chardata"`
}

func init() {
	// go-xmpp silently drops any message sub-element it doesn't recognize,
	// so the nwws-oi extension must be registered before any message is
	// parsed, or product data is lost with no error.
	stanza.TypeRegistry.MapExtension(stanza.PKTMessage, xml.Name{Space: "nwws-oi", Local: "x"}, nwwsExtension{})
}

// Product is a parsed NWWS-OI weather product, ready to be filed to disk.
type Product struct {
	CCCC    string
	TTAAII  string
	AWIPSID string
	Issue   time.Time
	ID      string
	Text    string
}

// ParseMessage extracts a Product from an XMPP message stanza carrying an
// nwws-oi extension. It returns an error describing what's missing or
// invalid if the message doesn't contain a usable product.
func ParseMessage(msg stanza.Message) (Product, error) {
	var ext nwwsExtension
	if !msg.Get(&ext) {
		return Product{}, fmt.Errorf("message has no nwws-oi extension")
	}

	var missing []string
	if ext.CCCC == "" {
		missing = append(missing, "cccc")
	}
	if ext.TTAAII == "" {
		missing = append(missing, "ttaaii")
	}
	if ext.AWIPSID == "" {
		missing = append(missing, "awipsid")
	}
	if ext.Issue == "" {
		missing = append(missing, "issue")
	}
	if ext.ID == "" {
		missing = append(missing, "id")
	}
	if len(missing) > 0 {
		return Product{}, fmt.Errorf("nwws-oi product missing required attribute(s): %s", strings.Join(missing, ", "))
	}

	issue, err := time.Parse(time.RFC3339, ext.Issue)
	if err != nil {
		return Product{}, fmt.Errorf("nwws-oi product has invalid issue timestamp %q: %w", ext.Issue, err)
	}

	return Product{
		CCCC:    ext.CCCC,
		TTAAII:  ext.TTAAII,
		AWIPSID: ext.AWIPSID,
		Issue:   issue.UTC(),
		ID:      ext.ID,
		Text:    ext.Text,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/product/...`
Expected: `ok  	github.com/jbuitt/nwws-go-client/internal/product`

- [ ] **Step 5: Commit**

```bash
git add internal/product/product.go internal/product/product_test.go
git commit -m "Add NWWS-OI message parsing"
```

- [ ] **Step 6: Write the failing tests for Filename/Dir**

Append to `internal/product/product_test.go`:

```go
func TestProduct_Filename(t *testing.T) {
	p := Product{
		CCCC:    "KKCI",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "12345",
	}

	got := p.Filename()
	want := "KKCI_FTUS21-TAFKORD.221432_12345.txt"
	if got != want {
		t.Errorf("Filename() = %q, want %q", got, want)
	}
}

func TestProduct_Filename_SanitizesID(t *testing.T) {
	p := Product{
		CCCC:    "KKCI",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "abc/def ghi",
	}

	got := p.Filename()
	if strings.ContainsAny(got, "/ ") {
		t.Errorf("Filename() = %q, contains unsafe characters from the raw ID", got)
	}
}

func TestProduct_Dir(t *testing.T) {
	p := Product{CCCC: "KKCI"}
	if p.Dir() != "KKCI" {
		t.Errorf("Dir() = %q, want KKCI", p.Dir())
	}
}
```

- [ ] **Step 7: Run test to verify it fails**

Run: `go test ./internal/product/...`
Expected: FAIL — `p.Filename undefined` / `p.Dir undefined`

- [ ] **Step 8: Write the implementation**

Append to `internal/product/product.go` (add `"regexp"` to the imports):

```go
var idSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// Filename returns the archive filename for this product:
// [cccc]_[ttaaii]-[awipsid].[ddHHMM]_[id].txt
func (p Product) Filename() string {
	ddHHMM := p.Issue.UTC().Format("021504")
	id := idSanitizer.ReplaceAllString(p.ID, "_")
	return fmt.Sprintf("%s_%s-%s.%s_%s.txt", p.CCCC, p.TTAAII, p.AWIPSID, ddHHMM, id)
}

// Dir returns the subdirectory (relative to the archive root) this product
// belongs in.
func (p Product) Dir() string {
	return p.CCCC
}
```

- [ ] **Step 9: Run test to verify it passes**

Run: `go test ./internal/product/... -v`
Expected: all tests PASS

- [ ] **Step 10: Commit**

```bash
git add internal/product/product.go internal/product/product_test.go
git commit -m "Add product filename generation"
```

---

## Task 7: Product storage with duplicate detection

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jbuitt/nwws-go-client/internal/product"
)

func testProduct() product.Product {
	return product.Product{
		CCCC:    "KKCI",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "12345",
		Text:    "SAMPLE PRODUCT TEXT",
	}
}

func TestWriteProduct_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	p := testProduct()

	result, path, err := WriteProduct(dir, p)
	if err != nil {
		t.Fatalf("WriteProduct: %v", err)
	}
	if result != Written {
		t.Errorf("result = %v, want Written", result)
	}

	wantPath := filepath.Join(dir, "KKCI", "KKCI_FTUS21-TAFKORD.221432_12345.txt")
	if path != wantPath {
		t.Errorf("path = %q, want %q", path, wantPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(data) != p.Text {
		t.Errorf("file content = %q, want %q", string(data), p.Text)
	}
}

func TestWriteProduct_DuplicateIsSkippedNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	p := testProduct()

	if _, _, err := WriteProduct(dir, p); err != nil {
		t.Fatalf("first WriteProduct: %v", err)
	}

	p2 := p
	p2.Text = "DIFFERENT TEXT THAT SHOULD NEVER BE WRITTEN"
	result, path, err := WriteProduct(dir, p2)
	if err != nil {
		t.Fatalf("second WriteProduct: %v", err)
	}
	if result != DuplicateSkipped {
		t.Errorf("result = %v, want DuplicateSkipped", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != p.Text {
		t.Errorf("file content = %q, want original %q (must not be overwritten)", string(data), p.Text)
	}
}

func TestWriteProduct_PermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission checks don't apply")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(dir, 0o700)

	_, _, err := WriteProduct(filepath.Join(dir, "readonly-parent"), testProduct())
	if err == nil {
		t.Fatal("WriteProduct: expected an error writing under a read-only directory")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/...`
Expected: FAIL — `undefined: WriteProduct`

- [ ] **Step 3: Write the implementation**

```go
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jbuitt/nwws-go-client/internal/product"
)

// Result indicates what WriteProduct did with a product.
type Result int

const (
	Written Result = iota
	DuplicateSkipped
)

// WriteProduct saves p under archiveDir/<cccc>/<filename>. If that file
// already exists (a duplicate product), it is left untouched and
// DuplicateSkipped is returned instead of an error. The returned path is
// valid regardless of the result.
func WriteProduct(archiveDir string, p product.Product) (Result, string, error) {
	dir := filepath.Join(archiveDir, p.Dir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, "", fmt.Errorf("creating directory %q: %w", dir, err)
	}

	path := filepath.Join(dir, p.Filename())
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return DuplicateSkipped, path, nil
		}
		return 0, "", fmt.Errorf("creating file %q: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(p.Text); err != nil {
		return 0, "", fmt.Errorf("writing file %q: %w", path, err)
	}

	return Written, path, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/... -v`
Expected: all tests PASS (the permission test skips only if run as root)

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "Add product storage with duplicate detection"
```

---

## Task 8: PAN script execution

**Files:**
- Create: `internal/pan/pan.go`
- Test: `internal/pan/pan_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package pan

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}
	return path
}

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func TestRun_Success(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "success.sh", `echo "ran: $1"; exit 0`)

	var buf bytes.Buffer
	Run(newTestLogger(&buf), script, "/tmp/products/KKCI/some-file.txt")

	out := buf.String()
	if !strings.Contains(out, "PAN script completed") {
		t.Errorf("log output = %q, want it to mention success", out)
	}
	if !strings.Contains(out, "ran: /tmp/products/KKCI/some-file.txt") {
		t.Errorf("log output = %q, want it to include the script's stdout", out)
	}
}

func TestRun_Failure(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "failure.sh", `echo "failing on purpose"; exit 1`)

	var buf bytes.Buffer
	Run(newTestLogger(&buf), script, "/tmp/products/KKCI/some-file.txt")

	out := buf.String()
	if !strings.Contains(out, "PAN script failed") {
		t.Errorf("log output = %q, want it to mention failure", out)
	}
}

func TestRun_Timeout(t *testing.T) {
	original := Timeout
	Timeout = 50 * time.Millisecond
	defer func() { Timeout = original }()

	dir := t.TempDir()
	script := writeScript(t, dir, "slow.sh", `sleep 5; exit 0`)

	var buf bytes.Buffer
	Run(newTestLogger(&buf), script, "/tmp/products/KKCI/some-file.txt")

	out := buf.String()
	if !strings.Contains(out, "PAN script failed") {
		t.Errorf("log output = %q, want it to report failure due to timeout", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pan/...`
Expected: FAIL — `undefined: Run` / `undefined: Timeout`

- [ ] **Step 3: Write the implementation**

```go
package pan

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"time"
)

// Timeout bounds how long a PAN script is allowed to run before it's
// killed. It's a var (not a const) so tests can shorten it.
var Timeout = 30 * time.Second

// Run executes panRun with filePath as its sole argument, logging its exit
// status and combined output via logger. Run is meant to be launched in its
// own goroutine by the caller so a slow script never blocks product
// ingestion; a caller that needs to wait for completion (e.g. during
// shutdown) should track that itself, such as with a sync.WaitGroup around
// the "go Run(...)" call.
func Run(logger *slog.Logger, panRun, filePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, panRun, filePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	attrs := []any{
		slog.String("pan_run", panRun),
		slog.String("file", filePath),
		slog.Duration("duration", duration),
		slog.String("output", out.String()),
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
		logger.Error("PAN script failed", attrs...)
		return
	}
	logger.Info("PAN script completed", attrs...)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pan/... -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pan/pan.go internal/pan/pan_test.go
git commit -m "Add PAN script execution"
```

---

## Task 9: nwwsclient — pure helpers (backoff, MUC JID)

**Files:**
- Create: `internal/nwwsclient/client.go`
- Test: `internal/nwwsclient/client_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package nwwsclient

import (
	"testing"
	"time"
)

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 60 * time.Second},
		{7, 60 * time.Second},
		{20, 60 * time.Second},
	}
	for _, c := range cases {
		got := nextBackoff(c.attempt)
		if got != c.want {
			t.Errorf("nextBackoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestMucJID(t *testing.T) {
	got := mucJID("nwws-go-client-abc12")
	want := "nwws@conference.nwws-oi.weather.gov/nwws-go-client-abc12"
	if got != want {
		t.Errorf("mucJID(...) = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nwwsclient/...`
Expected: FAIL — `undefined: nextBackoff`

- [ ] **Step 3: Write the implementation**

```go
package nwwsclient

import "time"

const mucRoom = "nwws@conference.nwws-oi.weather.gov"

// mucJID returns the full MUC occupant JID for the given resource/nickname.
func mucJID(resource string) string {
	return mucRoom + "/" + resource
}

const (
	backoffBase = 1 * time.Second
	backoffCap  = 60 * time.Second
)

// nextBackoff returns the delay before reconnect attempt number attempt
// (0-indexed): 1s, 2s, 4s, ... capped at 60s.
func nextBackoff(attempt int) time.Duration {
	d := backoffBase
	for i := 0; i < attempt; i++ {
		d *= 2
		if d >= backoffCap {
			return backoffCap
		}
	}
	return d
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/nwwsclient/... -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nwwsclient/client.go internal/nwwsclient/client_test.go
git commit -m "Add reconnect backoff and MUC JID helpers"
```

---

## Task 10: nwwsclient — message handling pipeline

**Files:**
- Modify: `internal/nwwsclient/client.go` (append `Client`, `New`, `handleMessage`, `waitForPAN`)
- Modify: `internal/nwwsclient/client_test.go` (append `TestHandleMessage_SavesProduct`)

- [ ] **Step 1: Write the failing test**

`internal/nwwsclient/client_test.go` already has an import block from Task 9
(`"testing"`, `"time"`). Replace that whole block with the merged version
below (do not add a second `import` block):

```go
import (
	"bytes"
	"encoding/xml"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gosrc.io/xmpp/stanza"

	"github.com/jbuitt/nwws-go-client/internal/config"
)
```

Then append to `internal/nwwsclient/client_test.go`:

```go
const testMessageXML = `<message from="nwws@conference.nwws-oi.weather.gov/KKCI" to="user@nwws-oi.weather.gov/res">
  <x xmlns="nwws-oi" cccc="KKCI" ttaaii="FTUS21" awipsid="TAFKORD" issue="2026-08-22T14:32:00Z" id="99">HANDLER TEST TEXT</x>
</message>`

func TestHandleMessage_SavesProduct(t *testing.T) {
	dir := t.TempDir()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	c := New(config.Config{ArchiveDir: dir}, logger, logger)

	var msg stanza.Message
	if err := xml.Unmarshal([]byte(testMessageXML), &msg); err != nil {
		t.Fatalf("unmarshaling test message: %v", err)
	}

	c.handleMessage(nil, msg)

	wantPath := filepath.Join(dir, "KKCI", "KKCI_FTUS21-TAFKORD.221432_99.txt")
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("expected product file at %s, got error: %v", wantPath, err)
	}
	if string(data) != "HANDLER TEST TEXT" {
		t.Errorf("file content = %q, want HANDLER TEST TEXT", string(data))
	}
}

func TestHandleMessage_IgnoresNonMessagePackets(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(config.Config{ArchiveDir: dir}, logger, logger)

	c.handleMessage(nil, stanza.Presence{})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files to be written for a non-message packet, found %d", len(entries))
	}
}

func TestWaitForPAN_ReturnsWhenWorkDone(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(config.Config{ArchiveDir: dir}, logger, logger)

	c.panWG.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		c.panWG.Done()
	}()

	start := time.Now()
	c.waitForPAN(2 * time.Second)
	if time.Since(start) > time.Second {
		t.Error("waitForPAN took far longer than the in-flight work needed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nwwsclient/...`
Expected: FAIL — `undefined: New`

- [ ] **Step 3: Write the implementation**

Append to `internal/nwwsclient/client.go` (add these imports: `"log/slog"`, `"sync"`, `"gosrc.io/xmpp"`, `"gosrc.io/xmpp/stanza"`, `"github.com/jbuitt/nwws-go-client/internal/config"`, `"github.com/jbuitt/nwws-go-client/internal/pan"`, `"github.com/jbuitt/nwws-go-client/internal/product"`, `"github.com/jbuitt/nwws-go-client/internal/store"`):

```go
// Client runs the NWWS-OI connection: connecting, joining the MUC room,
// handling incoming products, and reconnecting on drop per cfg.Retry.
type Client struct {
	cfg    config.Config
	logger *slog.Logger
	panLog *slog.Logger

	panWG sync.WaitGroup
}

// New creates a Client. panLog is the logger used for PAN script output —
// it may be the same as logger when no dedicated PAN log file is configured.
func New(cfg config.Config, logger, panLog *slog.Logger) *Client {
	return &Client{cfg: cfg, logger: logger, panLog: panLog}
}

// handleMessage is the router handler for incoming "message" stanzas. It
// parses the nwws-oi product, saves it, and (if configured) launches the PAN
// script asynchronously. It never returns an error: parse/save failures are
// logged and the message is dropped, since the receive loop must keep going.
func (c *Client) handleMessage(_ xmpp.Sender, p stanza.Packet) {
	msg, ok := p.(stanza.Message)
	if !ok {
		return
	}

	prod, err := product.ParseMessage(msg)
	if err != nil {
		c.logger.Warn("skipping unparseable message", slog.Any("error", err))
		return
	}

	result, path, err := store.WriteProduct(c.cfg.ArchiveDir, prod)
	if err != nil {
		c.logger.Error("failed to save product",
			slog.String("cccc", prod.CCCC), slog.String("id", prod.ID), slog.Any("error", err))
		return
	}
	if result == store.DuplicateSkipped {
		c.logger.Info("skipped duplicate product", slog.String("path", path))
		return
	}
	c.logger.Info("saved product", slog.String("path", path))

	if c.cfg.PanRun != "" {
		c.panWG.Add(1)
		go func() {
			defer c.panWG.Done()
			pan.Run(c.panLog, c.cfg.PanRun, path)
		}()
	}
}

// waitForPAN blocks until all in-flight PAN goroutines finish, or until
// timeout elapses, whichever comes first.
func (c *Client) waitForPAN(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		c.panWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		c.logger.Warn("timed out waiting for in-flight PAN scripts")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/nwwsclient/... -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nwwsclient/client.go internal/nwwsclient/client_test.go
git commit -m "Add message handling pipeline to nwwsclient"
```

---

## Task 11: nwwsclient — connect, MUC join, reconnect, shutdown

This task wires together the XMPP connection lifecycle. It has no live
server to test against (per the design spec, live/mocked XMPP integration
tests are out of scope), so verification here is build/vet correctness plus
a manual smoke-test checklist, not automated tests.

**Files:**
- Modify: `internal/nwwsclient/client.go` (append `Run`)

- [ ] **Step 1: Write the implementation**

Append to `internal/nwwsclient/client.go` (add `"context"`, `"fmt"` to the imports):

```go
// Run connects to the NWWS-OI server, joins the MUC room, and processes
// incoming products until ctx is cancelled. It reconnects on connection
// loss according to cfg.Retry. It returns nil on a clean shutdown (ctx
// cancelled) or an error if the connection was lost and Retry is false.
func (c *Client) Run(ctx context.Context) error {
	if !c.cfg.UseTLS {
		c.logger.Warn("use_tls is disabled; connecting without STARTTLS")
	}

	router := xmpp.NewRouter()
	router.HandleFunc("message", c.handleMessage)

	errCh := make(chan error, 1)
	reportErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	xmppCfg := xmpp.Config{
		TransportConfiguration: xmpp.TransportConfiguration{
			Address: fmt.Sprintf("%s:%d", c.cfg.Server, c.cfg.Port),
		},
		Jid:        fmt.Sprintf("%s@%s/%s", c.cfg.Username, c.cfg.Server, c.cfg.Resource),
		Credential: xmpp.Password(c.cfg.Password),
		Insecure:   !c.cfg.UseTLS,
	}

	client, err := xmpp.NewClient(&xmppCfg, router, reportErr)
	if err != nil {
		return fmt.Errorf("configuring xmpp client: %w", err)
	}

	joinMUC := func() error {
		c.logger.Info("joining MUC room", slog.String("room", mucRoom))
		return client.Send(stanza.Presence{
			Attrs: stanza.Attrs{To: mucJID(c.cfg.Resource)},
			Extensions: []stanza.PresExtension{
				stanza.MucPresence{History: stanza.History{MaxStanzas: stanza.NewNullableInt(0)}},
			},
		})
	}
	client.PostConnectHook = joinMUC
	client.PostResumeHook = joinMUC

	shutdown := func() {
		c.logger.Info("shutting down, leaving MUC room")
		_ = client.Send(stanza.Presence{
			Attrs: stanza.Attrs{To: mucJID(c.cfg.Resource), Type: stanza.StanzaType("unavailable")},
		})
		_ = client.Disconnect()
		c.waitForPAN(5 * time.Second)
	}

	c.logger.Info("connecting to NWWS-OI", slog.String("server", c.cfg.Server), slog.Int("port", c.cfg.Port))
	if err := client.Connect(); err != nil {
		return fmt.Errorf("connecting to %s:%d: %w", c.cfg.Server, c.cfg.Port, err)
	}

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			shutdown()
			return nil

		case connErr := <-errCh:
			c.logger.Warn("xmpp connection error", slog.Any("error", connErr))
			if !c.cfg.Retry {
				return fmt.Errorf("disconnected from NWWS-OI server: %w", connErr)
			}

			delay := nextBackoff(attempt)
			attempt++
			c.logger.Info("reconnecting to NWWS-OI", slog.Duration("delay", delay), slog.Int("attempt", attempt))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				shutdown()
				return nil
			}

			if err := client.Resume(); err != nil {
				c.logger.Error("reconnect attempt failed", slog.Any("error", err))
				reportErr(err)
				continue
			}
			c.logger.Info("reconnected to NWWS-OI")
			attempt = 0
		}
	}
}
```

- [ ] **Step 2: Verify it builds and vets cleanly**

Run: `go build ./... && go vet ./...`
Expected: no output, exit code 0

- [ ] **Step 3: Commit**

```bash
git add internal/nwwsclient/client.go
git commit -m "Add XMPP connection lifecycle: connect, MUC join, reconnect, shutdown"
```

- [ ] **Step 4: Note for later manual verification**

Once Task 12 (main.go) is done, this task's `Run` method needs a real smoke test against actual NWWS-OI credentials (see Task 13's manual verification checklist) — there's no way to verify MUC join, product receipt, or reconnect behavior without a live connection.

---

## Task 12: main.go wiring

**Files:**
- Create: `cmd/nwws-go-client/main.go`

- [ ] **Step 1: Write the implementation**

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jbuitt/nwws-go-client/internal/config"
	"github.com/jbuitt/nwws-go-client/internal/nwwsclient"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		logger.Error("configuration error", slog.Any("error", err))
		os.Exit(1)
	}

	panLogger := logger
	if cfg.PanRunLog != "" {
		f, err := os.OpenFile(cfg.PanRunLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			logger.Error("failed to open pan_run_log", slog.String("path", cfg.PanRunLog), slog.Any("error", err))
			os.Exit(1)
		}
		defer f.Close()
		panLogger = slog.New(slog.NewTextHandler(f, nil))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := nwwsclient.New(cfg, logger, panLogger)
	if err := client.Run(ctx); err != nil {
		logger.Error("client exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify it builds**

```bash
go build -o nwws-go-client ./cmd/nwws-go-client
```

Expected: no output, produces a `nwws-go-client` binary in the repo root

- [ ] **Step 3: Verify fail-fast validation manually**

```bash
./nwws-go-client
```

Expected: a log line like `level=ERROR msg="configuration error" error="username and password are required..."` and exit status 1. Confirm with:

```bash
echo $?
```

Expected: `1`

- [ ] **Step 4: Verify config file loading manually**

```bash
cp config.example.json config.json
# edit config.json: fill in a fake username/password so validation passes
./nwws-go-client
```

Expected: a log line for `connecting to NWWS-OI`, then (since the fake credentials will be rejected) a connection/auth error and exit status 1. This confirms config loading and the connection attempt reach the real server. Delete `config.json` afterward (it's gitignored, but don't leave real credentials on disk if you used any).

- [ ] **Step 5: Commit**

```bash
git add cmd/nwws-go-client/main.go
git commit -m "Add main entrypoint with config loading and signal handling"
```

---

## Task 13: README and final verification

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write the README**

```markdown
# nwws-go-client

A console client for NOAA's NWWS-OI (NOAA Weather Wire Service) that joins
the `nwws@conference.nwws-oi.weather.gov` XMPP MUC room, parses broadcast
weather products, and saves them to `products/<cccc>/`.

## Build

    go build -o nwws-go-client ./cmd/nwws-go-client

## Configure

Configuration is resolved from, in order of priority (highest wins):

1. CLI flags
2. Environment variables
3. A JSON config file (see `config.example.json`)
4. Built-in defaults

| Flag             | Env var             | JSON key      | Default                          |
|------------------|----------------------|---------------|-----------------------------------|
| `-server`        | `NWWS_SERVER`        | `server`      | `nwws-oi.weather.gov`             |
| `-port`          | `NWWS_PORT`          | `port`        | `5222`                            |
| `-username`      | `NWWS_USERNAME`      | `username`    | *(required)*                      |
| `-password`      | `NWWS_PASSWORD`      | `password`    | *(required)*                      |
| `-resource`      | `NWWS_RESOURCE`      | `resource`    | `nwws-go-client-XXXXX` (random)   |
| `-archivedir`    | `NWWS_ARCHIVEDIR`    | `archivedir`  | `./products/`                     |
| `-pan_run`       | `NWWS_PAN_RUN`       | `pan_run`     | *(disabled)*                      |
| `-pan_run_log`   | `NWWS_PAN_RUN_LOG`   | `pan_run_log` | *(logs to stdout)*                |
| `-retry`         | `NWWS_RETRY`         | `retry`       | `true`                            |
| `-use_tls`       | `NWWS_USE_TLS`       | `use_tls`     | `true`                            |
| `-config <path>` | —                    | —             | `./config.json` (optional if absent) |

## Run

    ./nwws-go-client -username myuser -password mypass

Or with a config file:

    cp config.example.json config.json
    # edit config.json
    ./nwws-go-client

Stop with Ctrl+C (SIGINT) or SIGTERM — the client leaves the MUC room and
disconnects cleanly before exiting.

## Product Arrival Notification (PAN)

If `pan_run` is set, it's invoked as `pan_run <path-to-saved-product-file>`
after each newly-saved (non-duplicate) product, asynchronously with a 30s
timeout. Its output goes to `pan_run_log` if set, otherwise to the main log.
```

- [ ] **Step 2: Run the full test suite**

```bash
go test ./... -v
```

Expected: every package reports `ok`, no failures

- [ ] **Step 3: Vet and tidy**

```bash
go vet ./...
go mod tidy
git diff go.mod go.sum
```

Expected: `go vet` produces no output; `go mod tidy` makes no unexpected changes (review the diff before committing if it does)

- [ ] **Step 4: Full build**

```bash
go build -o nwws-go-client ./cmd/nwws-go-client
```

Expected: no output, exit code 0

- [ ] **Step 5: Commit**

```bash
git add README.md go.mod go.sum
git commit -m "Add README and finalize dependencies"
```

---

## Post-implementation: manual live verification

Automated tests stop at the boundary of a real XMPP connection (per the
design spec). Before considering this done, verify against the real service
with real NWWS-OI credentials:

1. Run `./nwws-go-client -username <real> -password <real>` and confirm the
   log shows: connecting, joining the MUC room, and (once a product
   broadcasts) a "saved product" line with a real file appearing under
   `products/<cccc>/`.
2. Confirm reconnect behavior by killing your network connection briefly (or
   using a firewall rule) and watching the backoff/reconnect log lines, then
   confirm it resumes receiving products after connectivity returns.
3. Confirm graceful shutdown: press Ctrl+C and confirm the "shutting down,
   leaving MUC room" log line appears before the process exits.
4. If using `pan_run`, confirm the script actually receives the file path
   and that its output shows up in the configured log.
