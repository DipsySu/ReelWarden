package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Metadata   MetadataConfig   `yaml:"metadata"`
	AI         AIConfig         `yaml:"ai"`
	Compliance ComplianceConfig `yaml:"compliance"`
}

type ServerConfig struct {
	Listen   string `yaml:"listen"`
	DataDir  string `yaml:"data_dir"`
	LogLevel string `yaml:"log_level"`
}

type DatabaseConfig struct {
	Driver       string `yaml:"driver"`
	Path         string `yaml:"path"`
	WAL          bool   `yaml:"wal"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	BackupDir    string `yaml:"backup_dir"`
}

type MetadataConfig struct {
	Providers ProviderConfigs `yaml:"providers"`
}

type ProviderConfigs struct {
	Mock     ProviderToggle `yaml:"mock"`
	LocalNFO ProviderToggle `yaml:"local_nfo"`
	TMDB     ProviderToggle `yaml:"tmdb"`
}

type ProviderToggle struct {
	Enabled bool `yaml:"enabled"`
}

type AIConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ComplianceConfig struct {
	TMDBAI string `yaml:"tmdb_ai"`
}

func Default() Config {
	return Config{Server: ServerConfig{Listen: "127.0.0.1:8787", DataDir: "./data", LogLevel: "info"}, Database: DatabaseConfig{Driver: "modernc-sqlite", Path: "./data/reelwarden.db", WAL: true, MaxOpenConns: 1, BackupDir: "./data/backups"}, Compliance: ComplianceConfig{TMDBAI: "blocked"}}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("CFG_READ_FAILED: %w", err)
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("CFG_PARSE_FAILED: %w", err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("REELWARDEN_SERVER_LISTEN"); v != "" {
		cfg.Server.Listen = v
	}
	if v := os.Getenv("REELWARDEN_DATA_DIR"); v != "" {
		cfg.Server.DataDir = v
	}
	if v := os.Getenv("REELWARDEN_DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("REELWARDEN_AI_ENABLED"); v != "" {
		cfg.AI.Enabled, _ = strconv.ParseBool(v)
	}
	if v := os.Getenv("REELWARDEN_TMDB_ENABLED"); v != "" {
		cfg.Metadata.Providers.TMDB.Enabled, _ = strconv.ParseBool(v)
	}
}

func (c Config) Validate() error {
	if c.Server.Listen == "" {
		return errors.New("CFG_SERVER_LISTEN_REQUIRED")
	}
	if c.Database.Driver != "modernc-sqlite" {
		return fmt.Errorf("CFG_DATABASE_DRIVER_UNSUPPORTED: %s", c.Database.Driver)
	}
	if c.Database.Path == "" {
		return errors.New("CFG_DATABASE_PATH_REQUIRED")
	}
	if c.Database.MaxOpenConns != 1 {
		return errors.New("CFG_DATABASE_SINGLE_WRITER_REQUIRED")
	}
	if c.Compliance.TMDBAI == "" {
		return errors.New("CFG_COMPLIANCE_TMDB_AI_REQUIRED")
	}
	return nil
}
