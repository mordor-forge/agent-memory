package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the process configuration loaded from environment variables.
type Config struct {
	HTTPAddr               string
	DatabaseURL            string
	LogLevel               string
	LogFormat              string
	LogReportCaller        bool
	LogTimestamps          bool
	ShutdownTimeout        time.Duration
	HealthTimeout          time.Duration
	WorkerPollInterval     time.Duration
	DBMaxConns             int32
	DBMinConns             int32
	HTTPAuthMode           string
	HTTPAuthHeader         string
	HTTPAuthScheme         string
	HTTPAuthPrincipalsJSON string
	HTTPRateLimitRPS       float64
	HTTPRateLimitBurst     int
	EmbeddingProvider      string
	EmbeddingModel         string
	EmbeddingDimensions    int
	EmbeddingMaxBatch      int
	OpenAIBaseURL          string
	OpenAIAPIKey           string
}

// LoadFromEnv loads configuration from environment variables with sensible defaults.
func LoadFromEnv() (Config, error) {
	maxConns, err := int32FromEnv("MEMORY_DB_MAX_CONNS", 8)
	if err != nil {
		return Config{}, err
	}
	minConns, err := int32FromEnv("MEMORY_DB_MIN_CONNS", 1)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromEnv("MEMORY_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	healthTimeout, err := durationFromEnv("MEMORY_HEALTH_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerPollInterval, err := durationFromEnv("MEMORY_WORKER_POLL_INTERVAL", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	logReportCaller, err := boolFromEnv("MEMORY_LOG_REPORT_CALLER", false)
	if err != nil {
		return Config{}, err
	}
	logTimestamps, err := boolFromEnv("MEMORY_LOG_TIMESTAMPS", true)
	if err != nil {
		return Config{}, err
	}
	embeddingDimensions, err := intFromEnv("MEMORY_EMBEDDER_DIMENSIONS", 1536)
	if err != nil {
		return Config{}, err
	}
	embeddingMaxBatch, err := intFromEnv("MEMORY_EMBED_MAX_BATCH", 32)
	if err != nil {
		return Config{}, err
	}
	httpRateLimitRPS, err := float64FromEnv("MEMORY_HTTP_RATE_LIMIT_RPS", 0)
	if err != nil {
		return Config{}, err
	}
	httpRateLimitBurst, err := intFromEnv("MEMORY_HTTP_RATE_LIMIT_BURST", 10)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:               stringFromEnv("MEMORY_HTTP_ADDR", ":8080"),
		DatabaseURL:            os.Getenv("MEMORY_DATABASE_URL"),
		LogLevel:               stringFromEnv("MEMORY_LOG_LEVEL", "info"),
		LogFormat:              stringFromEnv("MEMORY_LOG_FORMAT", "text"),
		LogReportCaller:        logReportCaller,
		LogTimestamps:          logTimestamps,
		ShutdownTimeout:        shutdownTimeout,
		HealthTimeout:          healthTimeout,
		WorkerPollInterval:     workerPollInterval,
		DBMaxConns:             maxConns,
		DBMinConns:             minConns,
		HTTPAuthMode:           stringFromEnv("MEMORY_HTTP_AUTH_MODE", "disabled"),
		HTTPAuthHeader:         stringFromEnv("MEMORY_HTTP_AUTH_HEADER", "Authorization"),
		HTTPAuthScheme:         stringFromEnv("MEMORY_HTTP_AUTH_SCHEME", "Bearer"),
		HTTPAuthPrincipalsJSON: os.Getenv("MEMORY_HTTP_AUTH_PRINCIPALS_JSON"),
		HTTPRateLimitRPS:       httpRateLimitRPS,
		HTTPRateLimitBurst:     httpRateLimitBurst,
		EmbeddingProvider:      stringFromEnv("MEMORY_EMBEDDER_PROVIDER", "deterministic"),
		EmbeddingModel:         os.Getenv("MEMORY_EMBEDDER_MODEL"),
		EmbeddingDimensions:    embeddingDimensions,
		EmbeddingMaxBatch:      embeddingMaxBatch,
		OpenAIBaseURL:          stringFromEnv("MEMORY_OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:           os.Getenv("MEMORY_OPENAI_API_KEY"),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks the internal consistency of the configuration.
func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("MEMORY_HTTP_ADDR must not be empty")
	}
	if c.LogLevel == "" {
		return fmt.Errorf("MEMORY_LOG_LEVEL must not be empty")
	}
	switch c.LogFormat {
	case "", "text", "json", "logfmt":
	default:
		return fmt.Errorf("MEMORY_LOG_FORMAT must be one of: text, json, logfmt")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("MEMORY_SHUTDOWN_TIMEOUT must be > 0")
	}
	if c.HealthTimeout <= 0 {
		return fmt.Errorf("MEMORY_HEALTH_TIMEOUT must be > 0")
	}
	if c.WorkerPollInterval <= 0 {
		return fmt.Errorf("MEMORY_WORKER_POLL_INTERVAL must be > 0")
	}
	if c.DBMaxConns <= 0 {
		return fmt.Errorf("MEMORY_DB_MAX_CONNS must be > 0")
	}
	if c.DBMinConns < 0 {
		return fmt.Errorf("MEMORY_DB_MIN_CONNS must be >= 0")
	}
	if c.DBMinConns > c.DBMaxConns {
		return fmt.Errorf("MEMORY_DB_MIN_CONNS must be <= MEMORY_DB_MAX_CONNS")
	}
	switch c.HTTPAuthMode {
	case "disabled", "api_key":
	default:
		return fmt.Errorf("MEMORY_HTTP_AUTH_MODE must be one of: disabled, api_key")
	}
	if c.HTTPAuthMode == "api_key" {
		if c.HTTPAuthHeader == "" {
			return fmt.Errorf("MEMORY_HTTP_AUTH_HEADER must not be empty when auth is enabled")
		}
		if c.HTTPAuthScheme == "" {
			return fmt.Errorf("MEMORY_HTTP_AUTH_SCHEME must not be empty when auth is enabled")
		}
		if c.HTTPAuthPrincipalsJSON == "" {
			return fmt.Errorf("MEMORY_HTTP_AUTH_PRINCIPALS_JSON must not be empty when MEMORY_HTTP_AUTH_MODE=api_key")
		}
	}
	if c.HTTPRateLimitRPS < 0 {
		return fmt.Errorf("MEMORY_HTTP_RATE_LIMIT_RPS must be >= 0")
	}
	if c.HTTPRateLimitBurst < 0 {
		return fmt.Errorf("MEMORY_HTTP_RATE_LIMIT_BURST must be >= 0")
	}
	if c.EmbeddingDimensions <= 0 {
		return fmt.Errorf("MEMORY_EMBEDDER_DIMENSIONS must be > 0")
	}
	if c.EmbeddingMaxBatch <= 0 {
		return fmt.Errorf("MEMORY_EMBED_MAX_BATCH must be > 0")
	}
	switch c.EmbeddingProvider {
	case "", "deterministic":
	case "openai":
		if c.EmbeddingModel == "" {
			return fmt.Errorf("MEMORY_EMBEDDER_MODEL must not be empty when MEMORY_EMBEDDER_PROVIDER=openai")
		}
		if c.OpenAIAPIKey == "" {
			return fmt.Errorf("MEMORY_OPENAI_API_KEY must not be empty when MEMORY_EMBEDDER_PROVIDER=openai")
		}
	default:
		return fmt.Errorf("MEMORY_EMBEDDER_PROVIDER must be one of: deterministic, openai")
	}
	return nil
}

func stringFromEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func int32FromEnv(key string, fallback int32) (int32, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return int32(parsed), nil
}

func intFromEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func boolFromEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func float64FromEnv(key string, fallback float64) (float64, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
