// Package config resolves runtime configuration from environment variables
// with sensible local-first defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultPort  = 7777
	envDataDir   = "PLEUMCLOUD_DATA"
	envPort      = "PLEUMCLOUD_PORT"
	envBind      = "PLEUMCLOUD_BIND"
	envNoBrowser = "PLEUMCLOUD_NO_BROWSER"
	envServer    = "PLEUMCLOUD_SERVER"
	envPassword  = "PLEUMCLOUD_PASSWORD"
)

// Config holds resolved runtime settings.
type Config struct {
	// DataDir holds the SQLite database, thumbnail cache and secrets fallback.
	DataDir string
	Port    int
	// Bind is the listen host. Local-first default is loopback only;
	// server mode (post-MVP) will set 0.0.0.0 explicitly.
	Bind string
	// NoBrowser skips auto-opening the default browser on startup.
	NoBrowser bool
	// ServerMode binds on all interfaces and requires the auth password.
	ServerMode bool
	// Password guards the whole surface in server mode.
	Password string
}

func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}

	cfg := &Config{
		DataDir: filepath.Join(home, ".pleumcloud"),
		Port:    DefaultPort,
		Bind:    "127.0.0.1",
	}

	if v := os.Getenv(envDataDir); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv(envPort); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid %s=%q", envPort, v)
		}
		cfg.Port = p
	}
	if v := os.Getenv(envBind); v != "" {
		cfg.Bind = v
	}
	switch os.Getenv(envNoBrowser) {
	case "1", "true", "yes":
		cfg.NoBrowser = true
	}
	switch os.Getenv(envServer) {
	case "1", "true", "yes":
		cfg.ServerMode = true
		cfg.Bind = "0.0.0.0"
		cfg.NoBrowser = true // a headless server has no browser to open
		cfg.Password = os.Getenv(envPassword)
		if cfg.Password == "" {
			return nil, fmt.Errorf("PLEUMCLOUD_SERVER=1 requires PLEUMCLOUD_PASSWORD to be set")
		}
	}

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return cfg, nil
}

// DBPath returns the SQLite database location.
func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "pleumcloud.sqlite") }

// Addr returns the full listen address.
func (c *Config) BindAddr() string { return fmt.Sprintf("%s:%d", c.Bind, c.Port) }

// LocalURL returns the URL users open in a browser, normalizing wildcard
// binds to loopback.
func (c *Config) LocalURL() string {
	host := c.Bind
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", host, c.Port)
}
