//go:build integration

package cockroach

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestMigrateUpgradeFromV4BackfillsConsolidationRuns(t *testing.T) {
	dbURL := os.Getenv("COCKROACH_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("COCKROACH_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	testDBName := "upgrade_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer adminPool.Close()

	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+testDBName); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), "DROP DATABASE IF EXISTS "+testDBName+" CASCADE")
	})

	testDBURL, err := databaseURLWithName(dbURL, testDBName)
	if err != nil {
		t.Fatalf("databaseURLWithName() error = %v", err)
	}
	cfg := config.Config{
		HTTPAddr:        ":0",
		DatabaseURL:     testDBURL,
		LogLevel:        "error",
		ShutdownTimeout: 5 * time.Second,
		HealthTimeout:   2 * time.Second,
		DBMaxConns:      4,
		DBMinConns:      1,
	}

	store, err := Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if err := store.Exec(ctx, "SET CLUSTER SETTING feature.vector_index.enabled = true"); err != nil {
		t.Fatalf("enable vector indexes: %v", err)
	}
	if err := migrateStoreToVersion(ctx, store, 4); err != nil {
		t.Fatalf("migrateStoreToVersion() error = %v", err)
	}

	versionID, err := store.CurrentMigrationVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentMigrationVersion() error = %v", err)
	}
	if versionID != 4 {
		t.Fatalf("CurrentMigrationVersion() = %d, want 4", versionID)
	}

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}

	runID := uuid.New()
	workerID := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.Exec(ctx,
		`INSERT INTO consolidation_runs (
			id,
			tenant_id,
			worker_id,
			batch_started_at,
			input_from_created_at,
			input_from_id,
			input_to_created_at,
			input_to_id,
			status,
			provider,
			model,
			error_text,
			created_at,
			completed_at
		)
		VALUES ($1, $2, $3, $4, NULL, NULL, NULL, NULL, 'completed', 'system', 'episode-digests', NULL, $5, $6)`,
		runID,
		tenant.ID,
		workerID,
		now,
		now,
		now,
	); err != nil {
		t.Fatalf("insert legacy consolidation run: %v", err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	versionID, err = store.CurrentMigrationVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentMigrationVersion() error = %v", err)
	}
	if versionID != 5 {
		t.Fatalf("CurrentMigrationVersion() = %d, want 5", versionID)
	}

	var (
		projectionName string
		provider       sql.NullString
		model          sql.NullString
	)
	if err := store.pool.QueryRow(ctx,
		`SELECT projection_name, provider, model
		 FROM consolidation_runs
		 WHERE id = $1`,
		runID,
	).Scan(&projectionName, &provider, &model); err != nil {
		t.Fatalf("query upgraded consolidation run: %v", err)
	}
	if projectionName != "episode-digests" {
		t.Fatalf("projection_name = %q, want %q", projectionName, "episode-digests")
	}
	if provider.Valid {
		t.Fatalf("provider should be NULL after backfill cleanup, got %q", provider.String)
	}
	if model.Valid {
		t.Fatalf("model should be NULL after backfill cleanup, got %q", model.String)
	}

	runs, err := store.ListConsolidationRuns(ctx, memory.ListConsolidationRunsRequest{
		TenantID:       tenant.ID,
		ProjectionName: "episode-digests",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListConsolidationRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	if runs[0].ProjectionName != "episode-digests" {
		t.Fatalf("ProjectionName = %q, want %q", runs[0].ProjectionName, "episode-digests")
	}
	if runs[0].Provider != "" || runs[0].Model != "" {
		t.Fatalf("expected empty provider/model after legacy backfill, got provider=%q model=%q", runs[0].Provider, runs[0].Model)
	}
}

func migrateStoreToVersion(ctx context.Context, store *Store, version int64) error {
	sqlDB := stdlib.OpenDBFromPool(store.pool)
	defer sqlDB.Close()

	goose.SetBaseFS(migrationFS)
	goose.SetLogger(discardLogger{})
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpToContext(ctx, sqlDB, "migrations", version)
}

func databaseURLWithName(rawURL, databaseName string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}
