package cockroach

import (
	"context"
	"errors"
	"fmt"

	crdbpgxv5 "github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgxv5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"github.com/mordor-forge/agent-memory/internal/config"
)

// ErrVectorIndexesDisabled is returned when Cockroach vector indexes are not enabled.
var ErrVectorIndexesDisabled = errors.New("CockroachDB vector indexes are disabled; run `SET CLUSTER SETTING feature.vector_index.enabled = true;`")

// Store owns the CockroachDB connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to CockroachDB and registers vector types on every new connection.
func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	if cfg.DatabaseURL == "" {
		return nil, errors.New("database URL is required")
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "agent-memory"
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	store := &Store{pool: pool}
	if err := store.Ping(ctx); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the database pool.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// Ping verifies basic connectivity.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return errors.New("store is not initialized")
	}
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping cockroach: %w", err)
	}
	return nil
}

// Exec runs a SQL statement directly against the pool.
func (s *Store) Exec(ctx context.Context, query string, args ...any) error {
	if s == nil || s.pool == nil {
		return errors.New("store is not initialized")
	}
	_, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("exec query: %w", err)
	}
	return nil
}

// WithTx executes the callback inside a CockroachDB retry-safe transaction.
func (s *Store) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if s == nil || s.pool == nil {
		return errors.New("store is not initialized")
	}
	return crdbpgxv5.ExecuteTx(ctx, s.pool, pgx.TxOptions{}, fn)
}

// CheckVectorIndexEnabled verifies the CockroachDB cluster setting required for vector indexes.
func (s *Store) CheckVectorIndexEnabled(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return errors.New("store is not initialized")
	}
	var value bool
	if err := s.pool.QueryRow(ctx, "SHOW CLUSTER SETTING feature.vector_index.enabled").Scan(&value); err != nil {
		return fmt.Errorf("check vector index setting: %w", err)
	}
	if value {
		return nil
	}
	return ErrVectorIndexesDisabled
}

// CurrentMigrationVersion returns the current goose migration version, or zero when migrations have not been applied.
func (s *Store) CurrentMigrationVersion(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, errors.New("store is not initialized")
	}

	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = 'goose_db_version'
		)
	`).Scan(&exists)
	if err != nil {
		return 0, fmt.Errorf("check goose table: %w", err)
	}
	if !exists {
		return 0, nil
	}

	var versionID int64
	if err := s.pool.QueryRow(ctx, `
		SELECT version_id
		FROM goose_db_version
		WHERE is_applied = true
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&versionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("query goose version: %w", err)
	}
	return versionID, nil
}
