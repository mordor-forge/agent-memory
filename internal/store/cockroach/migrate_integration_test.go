//go:build integration

package cockroach_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mordor-forge/agent-memory/internal/config"
	storepkg "github.com/mordor-forge/agent-memory/internal/store/cockroach"
)

func TestMigrateAgainstCockroach(t *testing.T) {
	dbURL := os.Getenv("COCKROACH_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("COCKROACH_TEST_DATABASE_URL is not set")
	}

	cfg := config.Config{
		HTTPAddr:        ":0",
		DatabaseURL:     dbURL,
		LogLevel:        "error",
		ShutdownTimeout: 5 * time.Second,
		HealthTimeout:   2 * time.Second,
		DBMaxConns:      4,
		DBMinConns:      1,
	}

	ctx := context.Background()
	store, err := storepkg.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(store.Close)

	if err := store.Exec(ctx, "SET CLUSTER SETTING feature.vector_index.enabled = true"); err != nil {
		t.Fatalf("enable vector indexes: %v", err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	versionID, err := store.CurrentMigrationVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentMigrationVersion() error = %v", err)
	}
	if versionID == 0 {
		t.Fatal("expected non-zero migration version")
	}
}
