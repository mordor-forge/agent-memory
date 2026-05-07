package cockroach

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TryAcquireLease acquires or renews a worker lease if it is currently free, expired, or already held by the caller.
func (s *Store) TryAcquireLease(ctx context.Context, leaseName string, holderID uuid.UUID, ttl time.Duration) (bool, error) {
	if leaseName == "" {
		return false, fmt.Errorf("try acquire lease: lease name must not be empty")
	}
	if holderID == uuid.Nil {
		return false, fmt.Errorf("try acquire lease: holder_id must not be nil")
	}
	if ttl <= 0 {
		return false, fmt.Errorf("try acquire lease: ttl must be > 0")
	}

	expiresAt := time.Now().UTC().Add(ttl)
	var acquiredHolder uuid.UUID
	err := s.pool.QueryRow(ctx,
		`INSERT INTO worker_leases (lease_name, holder_id, expires_at, renewed_at, metadata)
		 VALUES ($1, $2, $3, now(), '{}')
		 ON CONFLICT (lease_name)
		 DO UPDATE SET
			holder_id = EXCLUDED.holder_id,
			expires_at = EXCLUDED.expires_at,
			renewed_at = now()
		 WHERE worker_leases.expires_at <= now() OR worker_leases.holder_id = EXCLUDED.holder_id
		 RETURNING holder_id`,
		leaseName, holderID, expiresAt,
	).Scan(&acquiredHolder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("try acquire lease: %w", err)
	}
	return acquiredHolder == holderID, nil
}

// ReleaseLease releases a lease held by the supplied worker. Releasing a missing lease is a no-op.
func (s *Store) ReleaseLease(ctx context.Context, leaseName string, holderID uuid.UUID) error {
	if leaseName == "" {
		return fmt.Errorf("release lease: lease name must not be empty")
	}
	if holderID == uuid.Nil {
		return fmt.Errorf("release lease: holder_id must not be nil")
	}

	_, err := s.pool.Exec(ctx,
		`DELETE FROM worker_leases
		 WHERE lease_name = $1 AND holder_id = $2`,
		leaseName, holderID,
	)
	if err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	return nil
}
