// Package config owns versioned configuration loading, precedence, validation,
// path normalization, and secret references.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const CurrentVersion = 1

// Config contains non-secret runtime settings.
type Config struct {
	Version  int            `yaml:"version" json:"version"`
	Server   ServerConfig   `yaml:"server" json:"server"`
	Upstream UpstreamConfig `yaml:"upstream" json:"upstream"`
	Limits   LimitsConfig   `yaml:"limits" json:"limits"`
	Logging  LoggingConfig  `yaml:"logging" json:"logging"`
}

type ServerConfig struct {
	MySQLListen     string `yaml:"mysql_listen" json:"mysql_listen"`
	MySQLMode       string `yaml:"mysql_mode" json:"mysql_mode"`
	AdminListen     string `yaml:"admin_listen" json:"admin_listen"`
	AdminTokenEnv   string `yaml:"admin_token_env" json:"admin_token_env"`
	ShutdownTimeout string `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	DataDir         string `yaml:"data_dir" json:"data_dir"`
}

type UpstreamConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	Host               string `yaml:"host" json:"host"`
	Port               int    `yaml:"port" json:"port"`
	User               string `yaml:"user" json:"user"`
	PasswordEnv        string `yaml:"password_env" json:"password_env"`
	AllowEmptyPassword bool   `yaml:"allow_empty_password" json:"allow_empty_password"`
	Database           string `yaml:"database" json:"database"`
	TLSMode            string `yaml:"tls_mode" json:"tls_mode"`
	TLSCAFile          string `yaml:"tls_ca_file" json:"tls_ca_file"`
	TLSServerName      string `yaml:"tls_server_name" json:"tls_server_name"`
	ConnectTimeout     string `yaml:"connect_timeout" json:"connect_timeout"`
	ReadTimeout        string `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout       string `yaml:"write_timeout" json:"write_timeout"`
	HealthTimeout      string `yaml:"health_timeout" json:"health_timeout"`
}

type LimitsConfig struct {
	MaxLogicalConnections  int   `yaml:"max_logical_connections" json:"max_logical_connections"`
	MaxUpstreamConnections int   `yaml:"max_upstream_connections" json:"max_upstream_connections"`
	MaxQueuedRequests      int   `yaml:"max_queued_requests" json:"max_queued_requests"`
	MaxQueuedBytes         int64 `yaml:"max_queued_bytes" json:"max_queued_bytes"`
}

type LoggingConfig struct {
	Level string `yaml:"level" json:"level"`
}

// Overrides are explicit command-line values. Nil means no override.
type Overrides struct {
	MySQLListen *string
	AdminListen *string
	DataDir     *string
	LogLevel    *string
}

// LoadOptions control configuration sources. ManagedPath is applied after
// explicit flag overrides by product contract.
type LoadOptions struct {
	Path        string
	ManagedPath string
	Overrides   Overrides
	LookupEnv   func(string) (string, bool)
}

// Default returns safe foundation defaults. Upstream execution is disabled.
func Default() Config {
	return Config{
		Version: CurrentVersion,
		Server: ServerConfig{
			MySQLListen:     "127.0.0.1:3307",
			MySQLMode:       "transparent",
			AdminListen:     "127.0.0.1:9090",
			ShutdownTimeout: "15s",
			DataDir:         "./data",
		},
		Upstream: UpstreamConfig{
			Enabled:        false,
			Host:           "127.0.0.1",
			Port:           3306,
			User:           "accelerator",
			PasswordEnv:    "DBA_UPSTREAM_PASSWORD",
			Database:       "app",
			TLSMode:        "preferred",
			ConnectTimeout: "5s",
			ReadTimeout:    "30s",
			WriteTimeout:   "30s",
			HealthTimeout:  "5s",
		},
		Limits: LimitsConfig{
			MaxLogicalConnections:  10_000,
			MaxUpstreamConnections: 200,
			MaxQueuedRequests:      5_000,
			MaxQueuedBytes:         64 << 20,
		},
		Logging: LoggingConfig{Level: "info"},
	}
}

// Load applies defaults, YAML, environment, flags, and managed overlay in that order.
func Load(options LoadOptions) (Config, error) {
	cfg := Default()
	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	baseDir, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolve working directory: %w", err)
	}
	if options.Path != "" {
		if err := decodeFile(options.Path, &cfg); err != nil {
			return Config{}, fmt.Errorf("load config %q: %w", options.Path, err)
		}
		absolutePath, err := filepath.Abs(options.Path)
		if err != nil {
			return Config{}, fmt.Errorf("resolve config path: %w", err)
		}
		baseDir = filepath.Dir(absolutePath)
	}

	if err := applyEnvironment(&cfg, lookup); err != nil {
		return Config{}, err
	}
	applyOverrides(&cfg, options.Overrides)

	if options.ManagedPath != "" {
		if err := decodeFile(options.ManagedPath, &cfg); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return Config{}, fmt.Errorf("load managed config %q: %w", options.ManagedPath, err)
			}
		}
	}

	if !filepath.IsAbs(cfg.Server.DataDir) {
		cfg.Server.DataDir = filepath.Clean(filepath.Join(baseDir, cfg.Server.DataDir))
	}
	if cfg.Upstream.TLSCAFile != "" && !filepath.IsAbs(cfg.Upstream.TLSCAFile) {
		cfg.Upstream.TLSCAFile = filepath.Clean(filepath.Join(baseDir, cfg.Upstream.TLSCAFile))
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func decodeFile(path string, target *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("configuration contains more than one YAML document")
		}
		return err
	}
	return nil
}

func applyOverrides(cfg *Config, overrides Overrides) {
	if overrides.MySQLListen != nil {
		cfg.Server.MySQLListen = *overrides.MySQLListen
	}
	if overrides.AdminListen != nil {
		cfg.Server.AdminListen = *overrides.AdminListen
	}
	if overrides.DataDir != nil {
		cfg.Server.DataDir = *overrides.DataDir
	}
	if overrides.LogLevel != nil {
		cfg.Logging.Level = *overrides.LogLevel
	}
}

func applyEnvironment(cfg *Config, lookup func(string) (string, bool)) error {
	setString(lookup, "DBA_MYSQL_LISTEN", &cfg.Server.MySQLListen)
	setString(lookup, "DBA_MYSQL_MODE", &cfg.Server.MySQLMode)
	setString(lookup, "DBA_ADMIN_LISTEN", &cfg.Server.AdminListen)
	setString(lookup, "DBA_ADMIN_TOKEN_ENV", &cfg.Server.AdminTokenEnv)
	setString(lookup, "DBA_SHUTDOWN_TIMEOUT", &cfg.Server.ShutdownTimeout)
	setString(lookup, "DBA_DATA_DIR", &cfg.Server.DataDir)
	setString(lookup, "DBA_LOG_LEVEL", &cfg.Logging.Level)
	setString(lookup, "DBA_UPSTREAM_HOST", &cfg.Upstream.Host)
	setString(lookup, "DBA_UPSTREAM_USER", &cfg.Upstream.User)
	setString(lookup, "DBA_UPSTREAM_PASSWORD_ENV", &cfg.Upstream.PasswordEnv)
	setString(lookup, "DBA_UPSTREAM_DATABASE", &cfg.Upstream.Database)
	setString(lookup, "DBA_UPSTREAM_TLS_MODE", &cfg.Upstream.TLSMode)
	setString(lookup, "DBA_UPSTREAM_TLS_CA_FILE", &cfg.Upstream.TLSCAFile)
	setString(lookup, "DBA_UPSTREAM_TLS_SERVER_NAME", &cfg.Upstream.TLSServerName)
	setString(lookup, "DBA_UPSTREAM_CONNECT_TIMEOUT", &cfg.Upstream.ConnectTimeout)
	setString(lookup, "DBA_UPSTREAM_READ_TIMEOUT", &cfg.Upstream.ReadTimeout)
	setString(lookup, "DBA_UPSTREAM_WRITE_TIMEOUT", &cfg.Upstream.WriteTimeout)
	setString(lookup, "DBA_UPSTREAM_HEALTH_TIMEOUT", &cfg.Upstream.HealthTimeout)

	if err := setBool(lookup, "DBA_UPSTREAM_ENABLED", &cfg.Upstream.Enabled); err != nil {
		return err
	}
	if err := setBool(lookup, "DBA_UPSTREAM_ALLOW_EMPTY_PASSWORD", &cfg.Upstream.AllowEmptyPassword); err != nil {
		return err
	}
	for _, item := range []struct {
		name   string
		target *int
	}{
		{"DBA_UPSTREAM_PORT", &cfg.Upstream.Port},
		{"DBA_MAX_LOGICAL_CONNECTIONS", &cfg.Limits.MaxLogicalConnections},
		{"DBA_MAX_UPSTREAM_CONNECTIONS", &cfg.Limits.MaxUpstreamConnections},
		{"DBA_MAX_QUEUED_REQUESTS", &cfg.Limits.MaxQueuedRequests},
	} {
		if err := setInt(lookup, item.name, item.target); err != nil {
			return err
		}
	}
	if value, ok := lookup("DBA_MAX_QUEUED_BYTES"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("environment DBA_MAX_QUEUED_BYTES: %w", err)
		}
		cfg.Limits.MaxQueuedBytes = parsed
	}
	return nil
}

func setString(lookup func(string) (string, bool), name string, target *string) {
	if value, ok := lookup(name); ok {
		*target = value
	}
}

func setBool(lookup func(string) (string, bool), name string, target *bool) error {
	value, ok := lookup(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("environment %s: %w", name, err)
	}
	*target = parsed
	return nil
}

func setInt(lookup func(string) (string, bool), name string, target *int) error {
	value, ok := lookup(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("environment %s: %w", name, err)
	}
	*target = parsed
	return nil
}

// Validate rejects configuration before any listener opens.
func (c Config) Validate() error {
	var problems []string
	if c.Version != CurrentVersion {
		problems = append(problems, fmt.Sprintf("version must be %d", CurrentVersion))
	}
	if err := validateAddress(c.Server.MySQLListen); err != nil {
		problems = append(problems, "server.mysql_listen: "+err.Error())
	}
	if !oneOf(strings.ToLower(c.Server.MySQLMode), "transparent", "pooled") {
		problems = append(problems, "server.mysql_mode must be transparent or pooled")
	}
	if err := validateAddress(c.Server.AdminListen); err != nil {
		problems = append(problems, "server.admin_listen: "+err.Error())
	}
	if c.Server.MySQLListen == c.Server.AdminListen {
		problems = append(problems, "server mysql and admin listeners must differ")
	}
	if _, err := c.ShutdownDuration(); err != nil {
		problems = append(problems, "server.shutdown_timeout: "+err.Error())
	}
	if strings.TrimSpace(c.Server.DataDir) == "" {
		problems = append(problems, "server.data_dir must not be empty")
	}
	if c.Limits.MaxLogicalConnections <= 0 {
		problems = append(problems, "limits.max_logical_connections must be positive")
	}
	if c.Limits.MaxUpstreamConnections <= 0 {
		problems = append(problems, "limits.max_upstream_connections must be positive")
	}
	if c.Limits.MaxUpstreamConnections > c.Limits.MaxLogicalConnections {
		problems = append(problems, "limits.max_upstream_connections cannot exceed max_logical_connections")
	}
	if c.Limits.MaxQueuedRequests < 0 || c.Limits.MaxQueuedBytes < 0 {
		problems = append(problems, "queue limits cannot be negative")
	}
	if !oneOf(strings.ToLower(c.Logging.Level), "debug", "info", "warn", "error") {
		problems = append(problems, "logging.level must be debug, info, warn, or error")
	}
	if c.Upstream.Enabled {
		if strings.TrimSpace(c.Upstream.Host) == "" {
			problems = append(problems, "upstream.host must not be empty when enabled")
		}
		if c.Upstream.Port < 1 || c.Upstream.Port > 65535 {
			problems = append(problems, "upstream.port must be between 1 and 65535")
		}
		if strings.TrimSpace(c.Upstream.User) == "" {
			problems = append(problems, "upstream.user must not be empty when enabled")
		}
		if strings.TrimSpace(c.Upstream.PasswordEnv) == "" {
			problems = append(problems, "upstream.password_env must not be empty when enabled")
		}
		if !oneOf(strings.ToLower(c.Upstream.TLSMode), "disabled", "preferred", "required", "verify-ca", "verify-full") {
			problems = append(problems, "upstream.tls_mode is unsupported")
		}
		if oneOf(strings.ToLower(c.Upstream.TLSMode), "verify-ca", "verify-full") && strings.TrimSpace(c.Upstream.TLSCAFile) == "" {
			problems = append(problems, "upstream.tls_ca_file is required for verify-ca and verify-full")
		}
		if strings.EqualFold(c.Upstream.TLSMode, "verify-full") && strings.TrimSpace(c.Upstream.TLSServerName) == "" && net.ParseIP(c.Upstream.Host) != nil {
			problems = append(problems, "upstream.tls_server_name is required for verify-full when host is an IP address")
		}
		for name, value := range map[string]string{
			"connect_timeout": c.Upstream.ConnectTimeout,
			"read_timeout":    c.Upstream.ReadTimeout,
			"write_timeout":   c.Upstream.WriteTimeout,
			"health_timeout":  c.Upstream.HealthTimeout,
		} {
			if _, err := boundedDuration(value, 100*time.Millisecond, 10*time.Minute); err != nil {
				problems = append(problems, "upstream."+name+": "+err.Error())
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if host == "" {
		return errors.New("host must not be empty")
	}
	parsed, err := strconv.Atoi(port)
	if err != nil || parsed < 0 || parsed > 65535 {
		return errors.New("port must be between 0 and 65535")
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// ShutdownDuration parses the bounded graceful-shutdown duration.
func (c Config) ShutdownDuration() (time.Duration, error) {
	duration, err := time.ParseDuration(c.Server.ShutdownTimeout)
	if err != nil {
		return 0, err
	}
	if duration <= 0 || duration > 10*time.Minute {
		return 0, errors.New("must be greater than zero and at most 10m")
	}
	return duration, nil
}

func boundedDuration(value string, minimum, maximum time.Duration) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if duration < minimum || duration > maximum {
		return 0, fmt.Errorf("must be between %s and %s", minimum, maximum)
	}
	return duration, nil
}

// UpstreamDurations returns validated connector and probe limits.
func (c Config) UpstreamDurations() (connect, read, write, health time.Duration, err error) {
	values := []*time.Duration{&connect, &read, &write, &health}
	settings := []string{c.Upstream.ConnectTimeout, c.Upstream.ReadTimeout, c.Upstream.WriteTimeout, c.Upstream.HealthTimeout}
	for index := range values {
		*values[index], err = boundedDuration(settings[index], 100*time.Millisecond, 10*time.Minute)
		if err != nil {
			return 0, 0, 0, 0, err
		}
	}
	return connect, read, write, health, nil
}

// RedactedJSON returns printable configuration without resolved secret values.
func (c Config) RedactedJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// WriteDefault writes a non-secret starter configuration.
func WriteDefault(path string, force bool) error {
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, bytes.NewBufferString(exampleYAML))
	return err
}

const exampleYAML = `version: 1

server:
  mysql_listen: 127.0.0.1:3307
  mysql_mode: transparent
  admin_listen: 127.0.0.1:9090
  admin_token_env: DBA_ADMIN_TOKEN
  shutdown_timeout: 15s
  data_dir: ./data

upstream:
  enabled: false
  host: 127.0.0.1
  port: 3306
  user: accelerator
  password_env: DBA_UPSTREAM_PASSWORD
  allow_empty_password: false
  database: app
  tls_mode: preferred
  tls_ca_file: ""
  tls_server_name: ""
  connect_timeout: 5s
  read_timeout: 30s
  write_timeout: 30s
  health_timeout: 5s

limits:
  max_logical_connections: 10000
  max_upstream_connections: 200
  max_queued_requests: 5000
  max_queued_bytes: 67108864

logging:
  level: info
`
