package observability

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mordor-forge/agent-memory/internal/config"
)

func TestNewLoggerJSONFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger, err := newLoggerWithWriter(config.Config{
		LogLevel:        "info",
		LogFormat:       "json",
		LogTimestamps:   true,
		LogReportCaller: false,
	}, &buf)
	if err != nil {
		t.Fatalf("newLoggerWithWriter() error = %v", err)
	}

	logger.Info("hello", "key", "value")
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) || !strings.Contains(out, `"key":"value"`) {
		t.Fatalf("unexpected json log output: %s", out)
	}
}

func TestNewLoggerRejectsInvalidFormat(t *testing.T) {
	t.Parallel()

	if _, err := newLoggerWithWriter(config.Config{
		LogLevel:      "info",
		LogFormat:     "yaml",
		LogTimestamps: true,
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for invalid log format")
	}
}
