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
type ProviderConfigs struct {
	Mock     ProviderToggle
	LocalNFO ProviderToggle
	TMDB     TMDBConfig
}
type ProviderToggle struct{ Enabled bool }
type TMDBConfig struct {
	Enabled              bool
	AuthType             string
	APIKey               string
	Token                string
	Language             string
	FallbackLanguage     string
	Region               string
	OfficialEndpointOnly bool
	APIBaseURL           string
	ProxyURL             string
	TimeoutSeconds       int
	MaxRetries           int
}
type AIConfig struct {
	Enabled        bool
	Provider       string
	BaseURL        string
	APIKey         string
	Model          string
	Protocol       string
	TimeoutSeconds int
	MaxRetries     int
	Capabilities   AICapabilitiesConfig
	Privacy        AIPrivacyConfig
}
type AICapabilitiesConfig struct {
	Streaming        string `json:"streaming"`
	ToolCalling      string `json:"tool_calling"`
	StructuredOutput string `json:"structured_output"`
}
type AIPrivacyConfig struct {
	Mode                string
	SendAbsolutePaths   bool
	SendProviderContent bool
	SendNFOContent      bool
}
type ComplianceConfig struct{ TMDBAI string }

func Default() Config {
	return Config{Server: ServerConfig{Listen: "127.0.0.1:8787", DataDir: "./data", LogLevel: "info"}, Database: DatabaseConfig{Driver: "modernc-sqlite", Path: "./data/reelwarden.db", WAL: true, MaxOpenConns: 1, BackupDir: "./data/backups"}, Metadata: MetadataConfig{DefaultProvider: "mock", Providers: ProviderConfigs{Mock: ProviderToggle{Enabled: true}, LocalNFO: ProviderToggle{Enabled: true}, TMDB: TMDBConfig{AuthType: "bearer_token", Language: "zh-CN", FallbackLanguage: "en-US", Region: "CN", OfficialEndpointOnly: true, APIBaseURL: "https://api.themoviedb.org/3", TimeoutSeconds: 15, MaxRetries: 2}}}, AI: AIConfig{Provider: "openai-compatible", BaseURL: "http://localhost:11434/v1", Model: "qwen3", Protocol: "chat-completions", TimeoutSeconds: 120, MaxRetries: 2, Capabilities: AICapabilitiesConfig{Streaming: "auto", ToolCalling: "auto", StructuredOutput: "auto"}, Privacy: AIPrivacyConfig{Mode: "minimal"}}, Compliance: ComplianceConfig{TMDBAI: "blocked"}}
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
	case "metadata.providers.tmdb.auth_type":
		c.Metadata.Providers.TMDB.AuthType = v
	case "metadata.providers.tmdb.api_key":
		c.Metadata.Providers.TMDB.APIKey = v
	case "metadata.providers.tmdb.token":
		c.Metadata.Providers.TMDB.Token = v
	case "metadata.providers.tmdb.language":
		c.Metadata.Providers.TMDB.Language = v
	case "metadata.providers.tmdb.fallback_language":
		c.Metadata.Providers.TMDB.FallbackLanguage = v
	case "metadata.providers.tmdb.region":
		c.Metadata.Providers.TMDB.Region = v
	case "metadata.providers.tmdb.official_endpoint_only":
		c.Metadata.Providers.TMDB.OfficialEndpointOnly = parseBool(v)
	case "metadata.providers.tmdb.api_base_url":
		c.Metadata.Providers.TMDB.APIBaseURL = v
	case "metadata.providers.tmdb.proxy_url":
		c.Metadata.Providers.TMDB.ProxyURL = v
	case "metadata.providers.tmdb.timeout_seconds":
		c.Metadata.Providers.TMDB.TimeoutSeconds = parseInt(v, c.Metadata.Providers.TMDB.TimeoutSeconds)
	case "metadata.providers.tmdb.max_retries":
		c.Metadata.Providers.TMDB.MaxRetries = parseInt(v, c.Metadata.Providers.TMDB.MaxRetries)
	case "ai.enabled":
		c.AI.Enabled = parseBool(v)
	case "ai.provider":
		c.AI.Provider = v
	case "ai.base_url":
		c.AI.BaseURL = strings.TrimRight(v, "/")
	case "ai.api_key":
		c.AI.APIKey = v
	case "ai.model":
		c.AI.Model = v
	case "ai.protocol":
		c.AI.Protocol = v
	case "ai.timeout_seconds":
		c.AI.TimeoutSeconds = parseInt(v, c.AI.TimeoutSeconds)
	case "ai.max_retries":
		c.AI.MaxRetries = parseInt(v, c.AI.MaxRetries)
	case "ai.capabilities.streaming":
		c.AI.Capabilities.Streaming = v
	case "ai.capabilities.tool_calling":
		c.AI.Capabilities.ToolCalling = v
	case "ai.capabilities.structured_output":
		c.AI.Capabilities.StructuredOutput = v
	case "ai.privacy.mode":
		c.AI.Privacy.Mode = v
	case "ai.privacy.send_absolute_paths":
		c.AI.Privacy.SendAbsolutePaths = parseBool(v)
	case "ai.privacy.send_provider_content":
		c.AI.Privacy.SendProviderContent = parseBool(v)
	case "ai.privacy.send_nfo_content":
		c.AI.Privacy.SendNFOContent = parseBool(v)
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
	if v := os.Getenv("REELWARDEN_LOG_LEVEL"); v != "" {
		cfg.Server.LogLevel = v
	}
	if v := os.Getenv("REELWARDEN_DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("REELWARDEN_AI_ENABLED"); v != "" {
		cfg.AI.Enabled = parseBool(v)
	}
	if v := os.Getenv("REELWARDEN_AI_PROVIDER"); v != "" {
		cfg.AI.Provider = v
	}
	if v := os.Getenv("REELWARDEN_AI_BASE_URL"); v != "" {
		cfg.AI.BaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("REELWARDEN_AI_API_KEY"); v != "" {
		cfg.AI.APIKey = v
	}
	if v := os.Getenv("REELWARDEN_AI_MODEL"); v != "" {
		cfg.AI.Model = v
	}
	if v := os.Getenv("REELWARDEN_AI_PROTOCOL"); v != "" {
		cfg.AI.Protocol = v
	}
	if v := os.Getenv("REELWARDEN_TMDB_ENABLED"); v != "" {
		cfg.Metadata.Providers.TMDB.Enabled = parseBool(v)
	}
	if v := os.Getenv("REELWARDEN_TMDB_AUTH_TYPE"); v != "" {
		cfg.Metadata.Providers.TMDB.AuthType = v
	}
	if v := os.Getenv("REELWARDEN_TMDB_API_KEY"); v != "" {
		cfg.Metadata.Providers.TMDB.APIKey = v
	}
	if v := os.Getenv("REELWARDEN_TMDB_TOKEN"); v != "" {
		cfg.Metadata.Providers.TMDB.Token = v
	}
	if v := os.Getenv("REELWARDEN_TMDB_LANGUAGE"); v != "" {
		cfg.Metadata.Providers.TMDB.Language = v
	}
	if v := os.Getenv("REELWARDEN_TMDB_REGION"); v != "" {
		cfg.Metadata.Providers.TMDB.Region = v
	}
	if v := os.Getenv("REELWARDEN_TMDB_PROXY_URL"); v != "" {
		cfg.Metadata.Providers.TMDB.ProxyURL = v
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
	if c.AI.Enabled {
		if c.AI.Provider == "" {
			return errors.New("CFG_AI_PROVIDER_REQUIRED")
		}
		if c.AI.BaseURL == "" {
			return errors.New("CFG_AI_BASE_URL_REQUIRED")
		}
		if c.AI.Model == "" {
			return errors.New("CFG_AI_MODEL_REQUIRED")
		}
	}
	if c.Metadata.Providers.TMDB.Enabled {
		if c.Metadata.Providers.TMDB.AuthType == "" {
			return errors.New("CFG_TMDB_AUTH_TYPE_REQUIRED")
		}
		if c.Metadata.Providers.TMDB.APIKey == "" && c.Metadata.Providers.TMDB.Token == "" {
			return errors.New("CFG_TMDB_CREDENTIAL_REQUIRED")
		}
		if c.Metadata.Providers.TMDB.Language == "" {
			return errors.New("CFG_TMDB_LANGUAGE_REQUIRED")
		}
	}
	return nil
}
