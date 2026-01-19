package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	TLS      TLSConfig      `yaml:"tls"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Logging  LoggingConfig  `yaml:"logging"`
	Tunnels  TunnelsConfig  `yaml:"tunnels"`
}

type ServerConfig struct {
	Domain      string `yaml:"domain"`
	ControlPort int    `yaml:"control_port"`
	HTTPPort    int    `yaml:"http_port"`
	HTTPSPort   int    `yaml:"https_port"`
}

type TLSConfig struct {
	Mode     string `yaml:"mode"`      // "auto", "manual", or "disabled"
	Email    string `yaml:"email"`     // For Let's Encrypt notifications
	CertPath string `yaml:"cert_path"` // For manual mode
	KeyPath  string `yaml:"key_path"`  // For manual mode
	CacheDir string `yaml:"cache_dir"` // Cache directory for autocert
	Staging  bool   `yaml:"staging"`   // Use Let's Encrypt staging for testing
}

type DatabaseConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

type AuthConfig struct {
	Required    bool `yaml:"required"`
	TokenLength int  `yaml:"token_length"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

type TunnelsConfig struct {
	SubdomainFormat         string `yaml:"subdomain_format"`
	TCPPortRange            string `yaml:"tcp_port_range"`
	MaxTunnelsPerClient     int    `yaml:"max_tunnels_per_client"`
	MaxConnectionsPerTunnel int    `yaml:"max_connections_per_tunnel"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

func (c *Config) validate() error {
	if c.Server.Domain == "" {
		return fmt.Errorf("server.domain is required")
	}
	if c.Server.ControlPort == 0 {
		c.Server.ControlPort = 4443
	}
	if c.Server.HTTPPort == 0 {
		c.Server.HTTPPort = 80
	}
	if c.Server.HTTPSPort == 0 {
		c.Server.HTTPSPort = 443
	}
	if c.Database.Type == "" {
		c.Database.Type = "sqlite"
	}
	if c.Database.Path == "" {
		c.Database.Path = "./tunnelab.db"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
	if c.Tunnels.MaxTunnelsPerClient == 0 {
		c.Tunnels.MaxTunnelsPerClient = 5
	}
	if c.TLS.Mode == "" {
		c.TLS.Mode = "disabled"
	}
	if c.TLS.CacheDir == "" {
		c.TLS.CacheDir = "./certs"
	}
	return nil
}
