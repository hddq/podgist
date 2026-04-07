package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	API      APIConfig      `yaml:"api"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type DatabaseConfig struct {
	DSN      string `yaml:"dsn"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode"`
}

type AuthConfig struct {
	Realm      string `yaml:"realm"`
	BcryptCost int    `yaml:"bcrypt_cost"`
}

type APIConfig struct {
	MaxEpisodeActions int `yaml:"max_episode_actions"`
	MaxRequestSize    int `yaml:"max_request_size"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func (d DatabaseConfig) GetDSN() string {
	if d.DSN != "" {
		return d.DSN
	}
	sslmode := d.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, sslmode)
}

func Load(path string) (*Config, error) {
	if envPath := os.Getenv("PODGIST_CONFIG"); envPath != "" {
		path = envPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(cfg)
	applyEnvOverrides(cfg)

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}
	if cfg.Auth.Realm == "" {
		cfg.Auth.Realm = "gpodder.net"
	}
	if cfg.Auth.BcryptCost == 0 {
		cfg.Auth.BcryptCost = 10
	}
	if cfg.API.MaxEpisodeActions == 0 {
		cfg.API.MaxEpisodeActions = 500
	}
	if cfg.API.MaxRequestSize == 0 {
		cfg.API.MaxRequestSize = 5 * 1024 * 1024
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "text"
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("PODGIST_DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("PODGIST_DB_DSN"); v != "" {
		cfg.Database.DSN = v
	}
}
