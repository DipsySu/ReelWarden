package config

type RuntimeView struct {
	Server     RuntimeServerView     `json:"server"`
	Database   RuntimeDatabaseView   `json:"database"`
	Metadata   RuntimeMetadataView   `json:"metadata"`
	AI         RuntimeAIView         `json:"ai"`
	Compliance RuntimeComplianceView `json:"compliance"`
}

type RuntimeServerView struct {
	Listen   string `json:"listen"`
	DataDir  string `json:"data_dir"`
	LogLevel string `json:"log_level"`
}

type RuntimeDatabaseView struct {
	Driver       string `json:"driver"`
	Path         string `json:"path"`
	WAL          bool   `json:"wal"`
	MaxOpenConns int    `json:"max_open_conns"`
	BackupDir    string `json:"backup_dir"`
}

type RuntimeMetadataView struct {
	DefaultProvider string                      `json:"default_provider"`
	Providers       RuntimeMetadataProviderView `json:"providers"`
}

type RuntimeMetadataProviderView struct {
	Mock     RuntimeProviderToggleView `json:"mock"`
	LocalNFO RuntimeProviderToggleView `json:"local_nfo"`
	TMDB     RuntimeTMDBView           `json:"tmdb"`
}

type RuntimeProviderToggleView struct {
	Enabled bool `json:"enabled"`
}

type RuntimeTMDBView struct {
	Enabled              bool   `json:"enabled"`
	AuthType             string `json:"auth_type"`
	APIKeyConfigured     bool   `json:"api_key_configured"`
	TokenConfigured      bool   `json:"token_configured"`
	Language             string `json:"language"`
	FallbackLanguage     string `json:"fallback_language"`
	Region               string `json:"region"`
	OfficialEndpointOnly bool   `json:"official_endpoint_only"`
	APIBaseURL           string `json:"api_base_url"`
	ProxyConfigured      bool   `json:"proxy_configured"`
	TimeoutSeconds       int    `json:"timeout_seconds"`
	MaxRetries           int    `json:"max_retries"`
}

type RuntimeAIView struct {
	Enabled          bool                 `json:"enabled"`
	Provider         string               `json:"provider"`
	BaseURL          string               `json:"base_url"`
	APIKeyConfigured bool                 `json:"api_key_configured"`
	Model            string               `json:"model"`
	Protocol         string               `json:"protocol"`
	TimeoutSeconds   int                  `json:"timeout_seconds"`
	MaxRetries       int                  `json:"max_retries"`
	Capabilities     AICapabilitiesConfig `json:"capabilities"`
	Privacy          RuntimeAIPrivacyView `json:"privacy"`
}

type RuntimeAIPrivacyView struct {
	Mode                string `json:"mode"`
	SendAbsolutePaths   bool   `json:"send_absolute_paths"`
	SendProviderContent bool   `json:"send_provider_content"`
	SendNFOContent      bool   `json:"send_nfo_content"`
}

type RuntimeComplianceView struct {
	TMDBAI string `json:"tmdb_ai"`
}

func (c Config) RuntimeView() RuntimeView {
	return RuntimeView{
		Server:   RuntimeServerView{Listen: c.Server.Listen, DataDir: c.Server.DataDir, LogLevel: c.Server.LogLevel},
		Database: RuntimeDatabaseView{Driver: c.Database.Driver, Path: c.Database.Path, WAL: c.Database.WAL, MaxOpenConns: c.Database.MaxOpenConns, BackupDir: c.Database.BackupDir},
		Metadata: RuntimeMetadataView{
			DefaultProvider: c.Metadata.DefaultProvider,
			Providers: RuntimeMetadataProviderView{
				Mock:     RuntimeProviderToggleView{Enabled: c.Metadata.Providers.Mock.Enabled},
				LocalNFO: RuntimeProviderToggleView{Enabled: c.Metadata.Providers.LocalNFO.Enabled},
				TMDB: RuntimeTMDBView{
					Enabled:              c.Metadata.Providers.TMDB.Enabled,
					AuthType:             c.Metadata.Providers.TMDB.AuthType,
					APIKeyConfigured:     c.Metadata.Providers.TMDB.APIKey != "",
					TokenConfigured:      c.Metadata.Providers.TMDB.Token != "",
					Language:             c.Metadata.Providers.TMDB.Language,
					FallbackLanguage:     c.Metadata.Providers.TMDB.FallbackLanguage,
					Region:               c.Metadata.Providers.TMDB.Region,
					OfficialEndpointOnly: c.Metadata.Providers.TMDB.OfficialEndpointOnly,
					APIBaseURL:           c.Metadata.Providers.TMDB.APIBaseURL,
					ProxyConfigured:      c.Metadata.Providers.TMDB.ProxyURL != "",
					TimeoutSeconds:       c.Metadata.Providers.TMDB.TimeoutSeconds,
					MaxRetries:           c.Metadata.Providers.TMDB.MaxRetries,
				},
			},
		},
		AI: RuntimeAIView{
			Enabled:          c.AI.Enabled,
			Provider:         c.AI.Provider,
			BaseURL:          c.AI.BaseURL,
			APIKeyConfigured: c.AI.APIKey != "",
			Model:            c.AI.Model,
			Protocol:         c.AI.Protocol,
			TimeoutSeconds:   c.AI.TimeoutSeconds,
			MaxRetries:       c.AI.MaxRetries,
			Capabilities:     c.AI.Capabilities,
			Privacy: RuntimeAIPrivacyView{
				Mode:                c.AI.Privacy.Mode,
				SendAbsolutePaths:   c.AI.Privacy.SendAbsolutePaths,
				SendProviderContent: c.AI.Privacy.SendProviderContent,
				SendNFOContent:      c.AI.Privacy.SendNFOContent,
			},
		},
		Compliance: RuntimeComplianceView{TMDBAI: c.Compliance.TMDBAI},
	}
}
