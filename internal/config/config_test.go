package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"REELWARDEN_SERVER_LISTEN",
		"REELWARDEN_DATA_DIR",
		"REELWARDEN_LOG_LEVEL",
		"REELWARDEN_DATABASE_PATH",
		"REELWARDEN_AI_ENABLED",
		"REELWARDEN_AI_PROVIDER",
		"REELWARDEN_AI_BASE_URL",
		"REELWARDEN_AI_API_KEY",
		"REELWARDEN_AI_MODEL",
		"REELWARDEN_AI_PROTOCOL",
		"REELWARDEN_TMDB_ENABLED",
		"REELWARDEN_TMDB_AUTH_TYPE",
		"REELWARDEN_TMDB_API_KEY",
		"REELWARDEN_TMDB_TOKEN",
		"REELWARDEN_TMDB_LANGUAGE",
		"REELWARDEN_TMDB_REGION",
		"REELWARDEN_TMDB_PROXY_URL",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadReadsAIAndTMDBSettings(t *testing.T) {
	clearConfigEnv(t)
	path := writeConfig(t, `
server:
  listen: 127.0.0.1:9999

metadata:
  default_provider: tmdb
  providers:
    tmdb:
      enabled: true
      auth_type: api_key
      api_key: yaml-tmdb-key
      language: ja-JP
      fallback_language: en-US
      region: JP
      official_endpoint_only: true
      api_base_url: https://api.themoviedb.org/3
      proxy_url: socks5://127.0.0.1:1080
      timeout_seconds: 9
      max_retries: 4

ai:
  enabled: true
  provider: openai-compatible
  base_url: http://localhost:11434/v1/
  api_key: yaml-ai-key
  model: qwen3
  protocol: chat-completions
  timeout_seconds: 60
  max_retries: 3
  capabilities:
    streaming: auto
    tool_calling: auto
    structured_output: off
  privacy:
    mode: minimal
    send_absolute_paths: false
    send_provider_content: false
    send_nfo_content: false

compliance:
  tmdb_ai: accepted
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Listen != "127.0.0.1:9999" {
		t.Fatalf("server listen not loaded: %s", cfg.Server.Listen)
	}
	if !cfg.Metadata.Providers.TMDB.Enabled || cfg.Metadata.Providers.TMDB.APIKey != "yaml-tmdb-key" {
		t.Fatalf("TMDB settings not loaded: %+v", cfg.Metadata.Providers.TMDB)
	}
	if cfg.Metadata.Providers.TMDB.Language != "ja-JP" || cfg.Metadata.Providers.TMDB.TimeoutSeconds != 9 {
		t.Fatalf("TMDB locale/retry settings not loaded: %+v", cfg.Metadata.Providers.TMDB)
	}
	if !cfg.AI.Enabled || cfg.AI.BaseURL != "http://localhost:11434/v1" || cfg.AI.APIKey != "yaml-ai-key" {
		t.Fatalf("AI settings not loaded: %+v", cfg.AI)
	}
	if cfg.AI.Capabilities.StructuredOutput != "off" {
		t.Fatalf("AI capabilities not loaded: %+v", cfg.AI.Capabilities)
	}
}

func TestLoadEnvOverridesSecretsAndRuntimeViewIsRedacted(t *testing.T) {
	clearConfigEnv(t)
	path := writeConfig(t, `
metadata:
  providers:
    tmdb:
      enabled: false

ai:
  enabled: false

compliance:
  tmdb_ai: blocked
`)
	t.Setenv("REELWARDEN_AI_ENABLED", "true")
	t.Setenv("REELWARDEN_AI_BASE_URL", "https://models.example/v1/")
	t.Setenv("REELWARDEN_AI_API_KEY", "env-ai-secret")
	t.Setenv("REELWARDEN_AI_MODEL", "gpt-test")
	t.Setenv("REELWARDEN_TMDB_ENABLED", "true")
	t.Setenv("REELWARDEN_TMDB_TOKEN", "env-tmdb-secret")
	t.Setenv("REELWARDEN_TMDB_LANGUAGE", "zh-CN")
	t.Setenv("REELWARDEN_TMDB_REGION", "CN")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	view := cfg.RuntimeView()
	if !view.AI.Enabled || view.AI.BaseURL != "https://models.example/v1" || view.AI.Model != "gpt-test" {
		t.Fatalf("runtime AI view mismatch: %+v", view.AI)
	}
	if !view.AI.APIKeyConfigured || !view.Metadata.Providers.TMDB.TokenConfigured {
		t.Fatalf("secret configured flags missing: %+v %+v", view.AI, view.Metadata.Providers.TMDB)
	}
	rendered := strings.Join([]string{view.AI.BaseURL, view.AI.Model, view.Metadata.Providers.TMDB.Language}, "|")
	if strings.Contains(rendered, "env-ai-secret") || strings.Contains(rendered, "env-tmdb-secret") {
		t.Fatal("runtime view leaked secrets")
	}
}

func TestLoadRejectsEnabledTMDBWithoutCredential(t *testing.T) {
	clearConfigEnv(t)
	path := writeConfig(t, `
metadata:
  providers:
    tmdb:
      enabled: true

compliance:
  tmdb_ai: blocked
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected TMDB credential validation error")
	}
	if !strings.Contains(err.Error(), "CFG_TMDB_CREDENTIAL_REQUIRED") {
		t.Fatalf("unexpected error: %v", err)
	}
}
