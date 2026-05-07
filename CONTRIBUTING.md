# Contributing

Contributions are welcome once the repository is published.

For now, keep changes aligned with these principles:

- CockroachDB is the reference backend.
- Keep transactions short and retry-safe.
- Never place external API calls inside retryable DB transactions.
- Prefer deterministic tests and explicit operational docs over clever abstractions.
- Keep the public API small until the core behavior stabilizes.
- Treat the project as `v0.1.x` alpha unless release docs state otherwise.

Run before opening a change:

```bash
make fmt
make test
```

Additional release context:

- release process and alpha scope: `docs/release.md`
- deployment guidance: `docs/operations/deploy.md`
- backup/export/restore guidance: `docs/operations/backup-restore.md`
