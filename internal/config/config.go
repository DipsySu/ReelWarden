package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server     ServerConfig     `json:"server"`
	Database   DatabaseConfig   `json:"database"`
	Metadata   MetadataConfig   `json:"metadata"`
	AI         AIConfig         `json:"ai"`
	Compliance ComplianceConfig `json:"compliance"`
}

type ServerConfig struct{ Listen, DataDir, LogLevel string }
type DatabaseConfig struct {
	Driver, Path, BackupDir string
	WAL                     bool
	MaxOpenConns            int
}
type MetadataConfig struct {
	DefaultProvider string
	Providers       ProviderConfigs
}
type ProviderConfigs struct{ Mock, LocalNFO, TMDB ProviderToggle }
type ProviderToggle struct{ Enabled bool }
type AIConfig struct{ Enabled bool }
type ComplianceConfig struct{ TMDBAI string }

func Default() Config {
	return Config{Server: ServerConfig{Listen: "127.0.0.1:8787", DataDir: "./data", LogLevel: "info"}, Database: DatabaseConfig{Driver: "modernc-sqlite", Path: "./data/reelwarden.db", WAL: true, MaxOpenConns: 1, BackupDir: "./data/backups"}, Metadata: MetadataConfig{DefaultProvider: "mock", Providers: ProviderConfigs{Mock: ProviderToggle{Enabled: true}, LocalNFO: ProviderToggle{Enabled: true}}}, Compliance: ComplianceConfig{TMDBAI: "blocked"}}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		if err := applyYAMLFile(&cfg, path); err != nil {
			return cfg, err
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func applyYAMLFile(cfg *Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("CFG_READ_FAILED: %w", err)
	}
	defer f.Close()
	var stack []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		indent := (len(raw) - len(strings.TrimLeft(raw, " "))) / 2
		if indent < 0 {
			indent = 0
		}
		if strings.HasSuffix(line, ":") {
			key := strings.TrimSuffix(line, ":")
			if indent >= len(stack) {
				stack = append(stack, key)
			} else {
				stack = append(stack[:indent], key)
			}
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		pathParts := append(append([]string{}, stack[:min(indent, len(stack))]...), key)
		set(cfg, pathParts, val)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("CFG_PARSE_FAILED: %w", err)
	}
	return nil
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func set(c *Config, p []string, v string) {
	joined := strings.Join(p, ".")
	switch joined {
	case "server.listen":
		c.Server.Listen = v
	case "server.data_dir":
		c.Server.DataDir = v
	case "server.log_level":
		c.Server.LogLevel = v
	case "database.driver":
		c.Database.Driver = v
	case "database.path":
		c.Database.Path = v
	case "database.wal":
		c.Database.WAL = parseBool(v)
	case "database.max_open_conns":
		c.Database.MaxOpenConns = parseInt(v, c.Database.MaxOpenConns)
	case "database.backup_dir":
		c.Database.BackupDir = v
	case "metadata.default_provider":
		c.Metadata.DefaultProvider = v
	case "metadata.providers.mock.enabled":
		c.Metadata.Providers.Mock.Enabled = parseBool(v)
	case "metadata.providers.local_nfo.enabled":
		c.Metadata.Providers.LocalNFO.Enabled = parseBool(v)
	case "metadata.providers.tmdb.enabled":
		c.Metadata.Providers.TMDB.Enabled = parseBool(v)
	case "ai.enabled":
		c.AI.Enabled = parseBool(v)
	case "compliance.tmdb_ai":
		c.Compliance.TMDBAI = v
	}
}
func parseBool(v string) bool { b, _ := strconv.ParseBool(v); return b }
func parseInt(v string, d int) int {
	i, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return i
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
		cfg.AI.Enabled = parseBool(v)
	}
	if v := os.Getenv("REELWARDEN_TMDB_ENABLED"); v != "" {
		cfg.Metadata.Providers.TMDB.Enabled = parseBool(v)
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
