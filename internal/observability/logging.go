package observability

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	charmlog "github.com/charmbracelet/log"

	"github.com/mordor-forge/agent-memory/internal/config"
)

// NewLogger builds the process logger from application config.
func NewLogger(cfg config.Config) (*slog.Logger, error) {
	return newLoggerWithWriter(cfg, os.Stdout)
}

func newLoggerWithWriter(cfg config.Config, w io.Writer) (*slog.Logger, error) {
	level, err := parseLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}

	formatter, err := parseFormatter(cfg.LogFormat)
	if err != nil {
		return nil, err
	}

	base := charmlog.NewWithOptions(w, charmlog.Options{
		Level:           level,
		Formatter:       formatter,
		ReportCaller:    cfg.LogReportCaller,
		ReportTimestamp: cfg.LogTimestamps,
		TimeFunction:    charmlog.NowUTC,
		TimeFormat:      time.RFC3339,
	})
	return slog.New(base), nil
}

func parseLevel(level string) (charmlog.Level, error) {
	switch level {
	case "debug":
		return charmlog.DebugLevel, nil
	case "info", "":
		return charmlog.InfoLevel, nil
	case "warn":
		return charmlog.WarnLevel, nil
	case "error":
		return charmlog.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
}

func parseFormatter(format string) (charmlog.Formatter, error) {
	switch format {
	case "", "text":
		return charmlog.TextFormatter, nil
	case "json":
		return charmlog.JSONFormatter, nil
	case "logfmt":
		return charmlog.LogfmtFormatter, nil
	default:
		return 0, fmt.Errorf("unsupported log format %q", format)
	}
}
