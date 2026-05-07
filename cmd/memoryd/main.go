package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mordor-forge/agent-memory/internal/auth"
	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/internal/health"
	"github.com/mordor-forge/agent-memory/internal/importer"
	"github.com/mordor-forge/agent-memory/internal/observability"
	"github.com/mordor-forge/agent-memory/internal/projection"
	"github.com/mordor-forge/agent-memory/internal/quota"
	"github.com/mordor-forge/agent-memory/internal/recall"
	"github.com/mordor-forge/agent-memory/internal/server"
	"github.com/mordor-forge/agent-memory/internal/store/cockroach"
	"github.com/mordor-forge/agent-memory/internal/version"
	"github.com/mordor-forge/agent-memory/internal/worker"

	"github.com/google/uuid"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return flag.ErrHelp
	}

	switch args[0] {
	case "serve":
		return runServe(ctx, args[1:])
	case "worker":
		return runWorker(ctx, args[1:])
	case "import":
		return runImport(ctx, args[1:])
	case "doctor":
		return runDoctor(ctx, args[1:])
	case "migrate":
		return runMigrate(ctx, args[1:])
	case "version":
		fmt.Println(version.Current().String())
		return nil
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("MEMORY_DATABASE_URL is required for serve")
	}

	logger, err := observability.NewLogger(cfg)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	observability.RegisterBuildInfo(version.Current())

	store, err := cockroach.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	checkers := []health.Checker{
		health.PingChecker{NameValue: "cockroach", PingFunc: store.Ping},
		health.PingChecker{NameValue: "vector_index_setting", PingFunc: store.CheckVectorIndexEnabled},
	}

	embedder, err := embed.NewRuntimeEmbedder(cfg, store)
	if err != nil {
		return err
	}
	recallService, err := recall.NewService(embedder, store)
	if err != nil {
		return err
	}
	authorizer, err := auth.NewFromConfig(cfg)
	if err != nil {
		return err
	}
	limiter := quota.NewHTTPRateLimiter(cfg)

	srv := server.New(cfg, logger, checkers, authorizer, limiter, store, store, store, recallService)
	errCh := make(chan error, 1)

	go func() {
		logger.Info("http server starting", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		logger.Info("shutting down")
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("MEMORY_DATABASE_URL is required for doctor")
	}

	store, err := cockroach.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	checks := []struct {
		name string
		fn   func(context.Context) error
	}{
		{name: "cockroach ping", fn: store.Ping},
		{name: "vector index setting", fn: store.CheckVectorIndexEnabled},
	}

	var failed bool
	for _, check := range checks {
		if err := check.fn(ctx); err != nil {
			fmt.Printf("[fail] %s: %v\n", check.name, err)
			failed = true
			continue
		}
		fmt.Printf("[ ok ] %s\n", check.name)
	}

	versionID, err := store.CurrentMigrationVersion(ctx)
	if err != nil {
		fmt.Printf("[warn] migration version: %v\n", err)
	} else {
		fmt.Printf("[info] current migration version: %d\n", versionID)
	}
	authorizer, err := auth.NewFromConfig(cfg)
	if err != nil {
		fmt.Printf("[fail] http auth config: %v\n", err)
		failed = true
	} else if authorizer.Enabled() {
		fmt.Printf("[info] http auth mode: %s\n", cfg.HTTPAuthMode)
	} else {
		fmt.Printf("[warn] http auth mode: disabled (dev-only)\n")
	}
	limiter := quota.NewHTTPRateLimiter(cfg)
	if limiter.Enabled() {
		fmt.Printf("[info] http rate limiting: enabled (rps=%.2f burst=%d)\n", cfg.HTTPRateLimitRPS, cfg.HTTPRateLimitBurst)
	} else {
		fmt.Printf("[warn] http rate limiting: disabled\n")
	}
	embedder, err := embed.NewFromConfig(cfg)
	if err != nil {
		fmt.Printf("[fail] embedder config: %v\n", err)
		failed = true
	} else {
		fmt.Printf("[info] embedder provider: %s model=%s dims=%d\n", cfg.EmbeddingProvider, embedder.Model(), embedder.Dimensions())
	}

	if failed {
		return errors.New("doctor found failing checks")
	}
	return nil
}

func runWorker(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	once := fs.Bool("once", false, "process the current tenant set once and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("MEMORY_DATABASE_URL is required for worker")
	}

	logger, err := observability.NewLogger(cfg)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	store, err := cockroach.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	embedder, err := embed.NewRuntimeEmbedder(cfg, store)
	if err != nil {
		return err
	}
	digestProjector, err := projection.NewEpisodeDigestProjector("episode-digests", store, embedder)
	if err != nil {
		return err
	}
	digestEngine, err := projection.NewEngine("episode-digests", store, digestProjector, 100, cfg.WorkerPollInterval)
	if err != nil {
		return err
	}
	snapshotProjector, err := projection.NewMemoryProjector("state-snapshots", store, embedder, projection.StateSnapshotTransformer{})
	if err != nil {
		return err
	}
	snapshotEngine, err := projection.NewEngine("state-snapshots", store, snapshotProjector, 100, cfg.WorkerPollInterval)
	if err != nil {
		return err
	}
	factMergeProjector, err := projection.NewMemoryProjector("fact-merges", store, embedder, projection.FactMergeTransformer{})
	if err != nil {
		return err
	}
	factMergeEngine, err := projection.NewEngine("fact-merges", store, factMergeProjector, 100, cfg.WorkerPollInterval)
	if err != nil {
		return err
	}
	runner, err := worker.NewRunner(uuid.New(), cfg.WorkerPollInterval, 100, logger, store, digestEngine, snapshotEngine, factMergeEngine)
	if err != nil {
		return err
	}

	logger.Info("worker starting", slog.Bool("once", *once), slog.Duration("poll_interval", cfg.WorkerPollInterval))
	if *once {
		return runner.RunOnce(ctx)
	}
	return runner.RunLoop(ctx)
}

func runMigrate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("MEMORY_DATABASE_URL is required for migrate")
	}

	store, err := cockroach.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		return err
	}

	versionID, err := store.CurrentMigrationVersion(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("migrations applied, current version=%d\n", versionID)
	return nil
}

func runImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	format := fs.String("format", "", "import format name")
	filePath := fs.String("file", "", "path to the source file")
	sourceID := fs.String("source-id", "", "stable source identifier used for idempotent re-runs")
	tenantIDValue := fs.String("tenant-id", "", "target tenant UUID")
	agentIDValue := fs.String("agent-id", "", "target agent UUID")
	threadIDValue := fs.String("thread-id", "", "optional target thread UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("MEMORY_DATABASE_URL is required for import")
	}

	tenantID, err := parseRequiredUUIDFlag("tenant-id", *tenantIDValue)
	if err != nil {
		return err
	}
	agentID, err := parseRequiredUUIDFlag("agent-id", *agentIDValue)
	if err != nil {
		return err
	}
	threadID, err := parseOptionalUUIDFlag(*threadIDValue)
	if err != nil {
		return fmt.Errorf("parse thread-id: %w", err)
	}

	store, err := cockroach.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	runner, err := importer.NewRunner(store, importer.CursorAgentJSONLImporter{})
	if err != nil {
		return err
	}
	report, err := runner.ImportFile(ctx, importer.ImportRequest{
		Format:   *format,
		FilePath: *filePath,
		SourceID: *sourceID,
		TenantID: tenantID,
		AgentID:  agentID,
		ThreadID: threadID,
	})
	if err != nil {
		return err
	}

	fmt.Printf(
		"import complete: format=%s source_id=%s records_read=%d episodes_imported=%d\n",
		report.Format,
		report.SourceID,
		report.RecordsRead,
		report.EpisodesImported,
	)
	return nil
}

func parseRequiredUUIDFlag(name, value string) (uuid.UUID, error) {
	if value == "" {
		return uuid.UUID{}, fmt.Errorf("%s is required", name)
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("parse %s: %w", name, err)
	}
	return id, nil
}

func parseOptionalUUIDFlag(value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func printUsage(w *os.File) {
	writeUsageLine(w, "agent-memory")
	writeUsageLine(w, "")
	writeUsageLine(w, "Usage:")
	writeUsageLine(w, "  memoryd serve")
	writeUsageLine(w, "  memoryd worker [--once]")
	writeUsageLine(w, "  memoryd import --format=<name> --file=<path> --tenant-id=<uuid> --agent-id=<uuid> [--thread-id=<uuid>] [--source-id=<value>]")
	writeUsageLine(w, "  memoryd doctor")
	writeUsageLine(w, "  memoryd migrate")
	writeUsageLine(w, "  memoryd version")
	writeUsageLine(w, "")
	writeUsageLine(w, "Environment:")
	writeUsageLine(w, "  MEMORY_DATABASE_URL      CockroachDB connection string")
	writeUsageLine(w, "  MEMORY_HTTP_ADDR         HTTP listen address (default :8080)")
	writeUsageLine(w, "  MEMORY_HTTP_AUTH_MODE    disabled|api_key (default disabled)")
	writeUsageLine(w, "  MEMORY_HTTP_AUTH_HEADER  auth header name (default Authorization)")
	writeUsageLine(w, "  MEMORY_HTTP_AUTH_SCHEME  auth scheme prefix (default Bearer)")
	writeUsageLine(w, "  MEMORY_HTTP_AUTH_PRINCIPALS_JSON JSON array of principals when api_key auth is enabled")
	writeUsageLine(w, "  MEMORY_HTTP_RATE_LIMIT_RPS requests per second per principal, 0 disables rate limiting")
	writeUsageLine(w, "  MEMORY_HTTP_RATE_LIMIT_BURST token bucket burst size when rate limiting is enabled")
	writeUsageLine(w, "  MEMORY_LOG_LEVEL         debug|info|warn|error (default info)")
	writeUsageLine(w, "  MEMORY_LOG_FORMAT        text|json|logfmt (default text)")
	writeUsageLine(w, "  MEMORY_LOG_REPORT_CALLER true|false (default false)")
	writeUsageLine(w, "  MEMORY_LOG_TIMESTAMPS    true|false (default true)")
	writeUsageLine(w, "  MEMORY_SHUTDOWN_TIMEOUT  graceful shutdown timeout (default 10s)")
	writeUsageLine(w, "  MEMORY_HEALTH_TIMEOUT    readiness timeout (default 3s)")
	writeUsageLine(w, "  MEMORY_WORKER_POLL_INTERVAL worker poll interval (default 10s)")
	writeUsageLine(w, "  MEMORY_EMBEDDER_PROVIDER deterministic|openai (default deterministic)")
	writeUsageLine(w, "  MEMORY_EMBEDDER_MODEL    required when provider=openai")
	writeUsageLine(w, "  MEMORY_EMBEDDER_DIMENSIONS embedding vector dimensions (default 1536)")
	writeUsageLine(w, "  MEMORY_EMBED_MAX_BATCH   max texts per provider embedding call (default 32)")
	writeUsageLine(w, "  MEMORY_OPENAI_API_KEY    required when provider=openai")
	writeUsageLine(w, "  MEMORY_OPENAI_BASE_URL   optional OpenAI-compatible base URL")
}

func writeUsageLine(w *os.File, line string) {
	_, _ = fmt.Fprintln(w, line)
}
