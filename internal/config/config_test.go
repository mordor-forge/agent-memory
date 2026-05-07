package config

import (
	"testing"
	"time"
)

func TestLoadFromEnvDefaults(t *testing.T) {
	t.Setenv("MEMORY_HTTP_ADDR", "")
	t.Setenv("MEMORY_DATABASE_URL", "")
	t.Setenv("MEMORY_LOG_LEVEL", "")
	t.Setenv("MEMORY_LOG_FORMAT", "")
	t.Setenv("MEMORY_LOG_REPORT_CALLER", "")
	t.Setenv("MEMORY_LOG_TIMESTAMPS", "")
	t.Setenv("MEMORY_SHUTDOWN_TIMEOUT", "")
	t.Setenv("MEMORY_HEALTH_TIMEOUT", "")
	t.Setenv("MEMORY_WORKER_POLL_INTERVAL", "")
	t.Setenv("MEMORY_DB_MAX_CONNS", "")
	t.Setenv("MEMORY_DB_MIN_CONNS", "")
	t.Setenv("MEMORY_HTTP_AUTH_MODE", "")
	t.Setenv("MEMORY_HTTP_AUTH_HEADER", "")
	t.Setenv("MEMORY_HTTP_AUTH_SCHEME", "")
	t.Setenv("MEMORY_HTTP_AUTH_PRINCIPALS_JSON", "")
	t.Setenv("MEMORY_HTTP_RATE_LIMIT_RPS", "")
	t.Setenv("MEMORY_HTTP_RATE_LIMIT_BURST", "")
	t.Setenv("MEMORY_EMBEDDER_PROVIDER", "")
	t.Setenv("MEMORY_EMBEDDER_MODEL", "")
	t.Setenv("MEMORY_EMBEDDER_DIMENSIONS", "")
	t.Setenv("MEMORY_EMBED_MAX_BATCH", "")
	t.Setenv("MEMORY_OPENAI_BASE_URL", "")
	t.Setenv("MEMORY_OPENAI_API_KEY", "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
	if cfg.LogReportCaller {
		t.Fatal("LogReportCaller should default to false")
	}
	if !cfg.LogTimestamps {
		t.Fatal("LogTimestamps should default to true")
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 10*time.Second)
	}
	if cfg.WorkerPollInterval != 10*time.Second {
		t.Fatalf("WorkerPollInterval = %v, want %v", cfg.WorkerPollInterval, 10*time.Second)
	}
	if cfg.EmbeddingProvider != "deterministic" {
		t.Fatalf("EmbeddingProvider = %q, want %q", cfg.EmbeddingProvider, "deterministic")
	}
	if cfg.HTTPAuthMode != "disabled" {
		t.Fatalf("HTTPAuthMode = %q, want %q", cfg.HTTPAuthMode, "disabled")
	}
	if cfg.HTTPRateLimitRPS != 0 {
		t.Fatalf("HTTPRateLimitRPS = %f, want 0", cfg.HTTPRateLimitRPS)
	}
	if cfg.HTTPRateLimitBurst != 10 {
		t.Fatalf("HTTPRateLimitBurst = %d, want 10", cfg.HTTPRateLimitBurst)
	}
	if cfg.EmbeddingDimensions != 1536 {
		t.Fatalf("EmbeddingDimensions = %d, want %d", cfg.EmbeddingDimensions, 1536)
	}
	if cfg.EmbeddingMaxBatch != 32 {
		t.Fatalf("EmbeddingMaxBatch = %d, want %d", cfg.EmbeddingMaxBatch, 32)
	}
}

func TestLoadFromEnvRejectsInvalidPoolSizes(t *testing.T) {
	t.Setenv("MEMORY_DB_MAX_CONNS", "1")
	t.Setenv("MEMORY_DB_MIN_CONNS", "2")

	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error for invalid pool sizes")
	}
}

func TestLoadFromEnvRejectsIncompleteOpenAIEmbedderConfig(t *testing.T) {
	t.Setenv("MEMORY_EMBEDDER_PROVIDER", "openai")
	t.Setenv("MEMORY_EMBEDDER_MODEL", "text-embedding-3-small")
	t.Setenv("MEMORY_OPENAI_API_KEY", "")

	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error for incomplete openai config")
	}
}

func TestLoadFromEnvRejectsMissingHTTPPrincipalsWhenAuthEnabled(t *testing.T) {
	t.Setenv("MEMORY_HTTP_AUTH_MODE", "api_key")
	t.Setenv("MEMORY_HTTP_AUTH_PRINCIPALS_JSON", "")

	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error for missing HTTP auth principals")
	}
}

func TestLoadFromEnvRejectsInvalidLogFormat(t *testing.T) {
	t.Setenv("MEMORY_LOG_FORMAT", "yaml")

	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error for invalid log format")
	}
}

func TestLoadFromEnvRejectsInvalidQuotaSettings(t *testing.T) {
	t.Setenv("MEMORY_HTTP_RATE_LIMIT_RPS", "-1")

	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected error for invalid rate limit")
	}
}
