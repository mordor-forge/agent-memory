# ADR 0001: CockroachDB First

## Status
Accepted.

## Decision
The project is designed for CockroachDB first, using:

- `pgx/v5` for connectivity
- `crdbpgxv5` for retry-safe multi-statement transactions
- Cockroach-native `VECTOR` columns and `VECTOR INDEX`
- ordered progress using `(created_at, id)` `CheckpointCursor`s
- lease-based worker coordination in the database

## Rationale

- high availability is a core product goal, not a later portability layer
- transaction retries are expected in distributed SQL and should shape the design early
- vector search should live in the same operational database as memory state
- designing for Cockroach first avoids PostgreSQL-only assumptions that later become technical debt
