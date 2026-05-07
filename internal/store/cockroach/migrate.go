package cockroach

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type discardLogger struct{}

func (discardLogger) Printf(string, ...any) {}
func (discardLogger) Fatalf(string, ...any) {}

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies embedded SQL migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if err := s.CheckVectorIndexEnabled(ctx); err != nil {
		return err
	}

	sqlDB := stdlib.OpenDBFromPool(s.pool)
	defer sqlDB.Close()

	goose.SetBaseFS(migrationFS)
	goose.SetLogger(discardLogger{})
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
