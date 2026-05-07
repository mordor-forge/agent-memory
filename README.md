# agent-memory

`agent-memory` is a CockroachDB-first memory foundation for AI agents written in Go.

License: `Apache-2.0`.

For a guided overview of the current system shape, see `ARCHITECTURE.md`.

This repository is intentionally being built as a production-ready base layer from day one:

- CockroachDB-native schema and retry model
- native `VECTOR` support and `VECTOR INDEX`
- short retry-safe transactions with `pgx/v5` and `crdbpgxv5`
- health checks, metrics, structured logging, and tagged releases
- CI for linting, unit tests, integration smoke tests, and release packaging

The current alpha is already beyond a scaffold. Today it can:

- append immutable `episodes`
- project those episodes into `episode_digest`, `state_snapshot`, and `fact_merge` memories
- persist embeddings in CockroachDB and serve semantic recall
- explain memory provenance and inspect checkpoints, projection runs, and worker leases

The next major gap is not basic plumbing anymore. It is getting more real context into the system through import and capture workflows.

## Status

Current maturity: **`v0.1.x alpha`**

That means:

- the core write / project / recall loop is real
- local and operator-guided use is supported
- HTTP and MCP surfaces exist and are useful
- API and operational behavior can still evolve within the alpha line

It does **not** yet imply:

- stable long-term API compatibility guarantees
- mature remote MCP deployment support
- broad production topology support
- finished auth/RBAC depth beyond the current baseline

## Working Assumptions

- Working module path: `github.com/mordor-forge/agent-memory`
- CockroachDB is the first-class backend, not a PostgreSQL compatibility mode
- external embedding / LLM calls will happen outside retryable database transactions
- ordered projection progress will use `CheckpointCursor` semantics over `(created_at, id)`

## Commands

```bash
go run ./cmd/memoryd version
go run ./cmd/memoryd doctor
go run ./cmd/memoryd migrate
go run ./cmd/memoryd serve
go run ./cmd/memoryd worker --once
go run ./cmd/memoryd import --format=cursor-agent-jsonl --file=/path/to/transcript.jsonl --tenant-id=<tenant-uuid> --agent-id=<agent-uuid> [--thread-id=<thread-uuid>]
go run ./cmd/agent-memory-mcp
```

## Core Primitives

The current reusable memory primitives are:

- `episode_digest`: one durable digest per episode
- `state_snapshot`: keep the latest state for a logical key
- `fact_merge`: collapse repeated observations into one accumulating fact

Focused docs for the projection primitives:

- `docs/primitives/state-snapshot.md`
- `docs/primitives/fact-merge.md`

## Local Development

1. Start CockroachDB locally:

```bash
make dev-up
```

2. Load the local development environment:

```bash
# recommended with direnv:
direnv allow

# or manually:
eval "$(./scripts/dev-env.sh)"
```

If you use fish, make sure the direnv hook is enabled once in your shell config:

```fish
direnv hook fish | source
```

The tracked `.envrc` sets safe local defaults for deterministic development. Personal overrides can go in `.envrc.local` (gitignored).

Local development runs with:

- `MEMORY_HTTP_AUTH_MODE=disabled`

This is a deliberate **dev-only bypass** for the HTTP API. It should not be treated as a production deployment posture.

3. Run migrations and start the server:

```bash
make migrate
make run
```

4. Run one worker pass:

```bash
go run ./cmd/memoryd worker --once
```

5. Inspect projected memories and semantic recall:

```bash
curl "http://127.0.0.1:8080/v1/memories?tenant_id=<tenant-uuid>"
curl "http://127.0.0.1:8080/v1/recall?tenant_id=<tenant-uuid>&query=resume%20study&limit=5"
curl "http://127.0.0.1:8080/v1/projections/checkpoints?tenant_id=<tenant-uuid>"
curl "http://127.0.0.1:8080/v1/projections/runs?tenant_id=<tenant-uuid>"
curl "http://127.0.0.1:8080/v1/leases"
```

To stop or reset the local Cockroach container:

```bash
make dev-down
# or wipe container + volume
make dev-reset
```

## First HTTP API Calls

Create the core resources over HTTP:

```bash
curl -X POST http://127.0.0.1:8080/v1/tenants \
  -H 'Content-Type: application/json' \
  -d '{"slug":"demo-tenant"}'

curl -X POST http://127.0.0.1:8080/v1/agents \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"<tenant-uuid>","name":"demo-agent"}'

curl -X POST http://127.0.0.1:8080/v1/threads \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"<tenant-uuid>","agent_id":"<agent-uuid>"}'

curl -X POST http://127.0.0.1:8080/v1/episodes \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"<tenant-uuid>","agent_id":"<agent-uuid>","thread_id":"<thread-uuid>","kind":"note","content":"CockroachDB retries are normal in distributed SQL"}'

# export tenant-scoped data
curl -H 'Authorization: Bearer replace-me' \
  http://127.0.0.1:8080/v1/tenants/<tenant-uuid>/export

# delete a tenant (admin-only when auth is enabled)
curl -X DELETE -H 'Authorization: Bearer replace-me' \
  http://127.0.0.1:8080/v1/tenants/<tenant-uuid>
```

Versioned `/v1/` endpoints now use a consistent envelope:

- success: `{"data": ...}`
- list-style success: `{"data":{"items":[...]},"meta":{"count":N,"limit":M}}`
- error: `{"error":{"code":"...","message":"..."}}`

Compatibility details for `/v1/`:

- requests may use `Content-Type: application/json`
- JSON responses use `Content-Type: application/vnd.agent-memory.v1+json`
- responses advertise:
  - `X-Agent-Memory-API-Version: v1`
  - `X-Agent-Memory-API-Stability: alpha`
- clients should ignore unknown JSON fields so additive response growth stays compatible
- unsupported `Accept` headers return `406`
- unsupported JSON write `Content-Type` values return `415`

When HTTP auth is enabled, `/v1/` endpoints expect an auth header and distinguish:

- `401` unauthenticated
- `403` forbidden
- `400` invalid request

Recall results now include both:

- `distance`: raw vector distance from the candidate search
- `score`: the final reranked score after applying importance, confidence, recency, kind, and status heuristics

## Historical Import

The repo now includes a first narrow importer for Cursor agent transcript JSONL files.

Import flow:

1. create or choose the target tenant / agent / thread
2. run:

```bash
go run ./cmd/memoryd import \
  --format=cursor-agent-jsonl \
  --file="/path/to/transcript.jsonl" \
  --tenant-id="<tenant-uuid>" \
  --agent-id="<agent-uuid>" \
  --thread-id="<thread-uuid>" \
  --source-id="cursor-session-001"
```

3. project the imported episodes:

```bash
go run ./cmd/memoryd worker --once
```

See:

- `docs/adr/0005-importer-framework.md`
- `docs/recipes/cursor-transcript-import.md`

## Generic State Snapshot Convention

Episodes can opt into a reusable `state_snapshot` projection by including snapshot metadata in their payload.

See `docs/primitives/state-snapshot.md` for the detailed guide, field reference, and expected resulting memory shape.

Minimum payload:

```json
{
  "snapshot_key": "resume_state"
}
```

Useful optional fields:

```json
{
  "snapshot_key": "resume_state",
  "snapshot_content": "resume on lesson 4 feedback step",
  "snapshot_summary": "latest resume point",
  "snapshot_status": "active",
  "snapshot_kind": "state_snapshot",
  "snapshot_embed_text": "resume state lesson 4 feedback",
  "snapshot_attributes": {
    "phase": "reviewing",
    "pending_action": "revise implementation"
  }
}
```

The worker now runs both:

- `episode-digests`
- `state-snapshots`

This keeps the core generally useful without baking in a study-specific schema.

## Generic Fact Merge Convention

Episodes can opt into a reusable `fact_merge` projection when you want repeated observations to collapse into one durable memory instead of one row per event.

See `docs/primitives/fact-merge.md` for the detailed guide, merge behavior, and worked examples.

Opt in with either:

```json
{
  "fact_merge": true
}
```

or an explicit logical key:

```json
{
  "fact_key": "preferred_database"
}
```

Useful optional fields:

```json
{
  "fact_merge": true,
  "fact_content": "CockroachDB is the preferred HA database",
  "fact_summary": "preferred HA database choice",
  "fact_kind": "fact_merge",
  "fact_status": "active",
  "fact_embed_text": "preferred database cockroachdb high availability",
  "fact_importance": 0.7,
  "fact_attributes": {
    "source": "user_preference"
  }
}
```

The `fact_merge` projection:

- normalizes repeated content
- accumulates `observation_count`
- tracks `first_observed_at`
- updates `last_observed_at`
- refreshes the merged memory embedding

## Embedder Providers

The service now supports config-based embedder selection.

Default local/dev setup:

```bash
export MEMORY_EMBEDDER_PROVIDER='deterministic'
export MEMORY_EMBEDDER_DIMENSIONS='1536'
```

OpenAI-compatible setup:

```bash
export MEMORY_EMBEDDER_PROVIDER='openai'
export MEMORY_EMBEDDER_MODEL='text-embedding-3-small'
export MEMORY_EMBEDDER_DIMENSIONS='1536'
export MEMORY_OPENAI_API_KEY='<api-key>'
# optional
export MEMORY_OPENAI_BASE_URL='https://api.openai.com/v1'
```

`MEMORY_OPENAI_BASE_URL` can point at any OpenAI-compatible endpoint if you want to use a different provider later.

## HTTP Auth (Initial Slice)

The first auth implementation is **HTTP-only**. MCP remains local-first and is not treated as a remote production API.

Current modes:

- `MEMORY_HTTP_AUTH_MODE=disabled`
  - local/dev bypass
- `MEMORY_HTTP_AUTH_MODE=api_key`
  - API-key style auth with principal and tenant grants from config

Example:

```bash
export MEMORY_HTTP_AUTH_MODE='api_key'
export MEMORY_HTTP_AUTH_HEADER='Authorization'
export MEMORY_HTTP_AUTH_SCHEME='Bearer'
export MEMORY_HTTP_AUTH_PRINCIPALS_JSON='[
  {
    "token": "replace-me",
    "subject": "local-user",
    "role": "admin",
    "tenant_grants": ["*"]
  }
]'
```

Then call the HTTP API with:

```bash
curl -H 'Authorization: Bearer replace-me' \
  http://127.0.0.1:8080/v1/memories?tenant_id=<tenant-uuid>
```

## Quotas and Backpressure (Initial Slice)

The first quota/backpressure controls are intentionally simple:

- principal-scoped HTTP rate limiting
- bounded embedding provider batch sizes

HTTP rate limiting:

```bash
export MEMORY_HTTP_RATE_LIMIT_RPS='5'
export MEMORY_HTTP_RATE_LIMIT_BURST='10'
```

When exceeded, `/v1/` endpoints return:

```json
{
  "error": {
    "code": "rate_limited",
    "message": "request rate limit exceeded"
  }
}
```

Embedding batch limiting:

```bash
export MEMORY_EMBED_MAX_BATCH='32'
```

This bounds how many texts are sent to the underlying embedder in a single provider call. Larger batches are split deterministically and logged.

## Data Governance and Secret Hygiene (Initial Slice)

Current baseline:

- tenant-scoped export over HTTP and MCP
- tenant deletion over HTTP and MCP
- explicit local-dev auth bypass
- environment-driven secrets for:
  - HTTP auth principals
  - OpenAI-compatible provider keys

Current expectations:

- treat `.envrc.local` and any auth/provider env material as secrets
- do not commit `MEMORY_HTTP_AUTH_PRINCIPALS_JSON` or real provider keys
- rotate secrets by updating the environment source and restarting the affected process
- use export before destructive delete if you need a portable snapshot

The current delete path is explicit and destructive. It is meant for operator-controlled use, not silent lifecycle cleanup.

At runtime, configured embedders are wrapped with:

- provider request metrics
- optional Cockroach-backed `embedding_cache`

So repeated projection and recall queries can avoid recomputing the same vectors when the cache is warm.

## Logging

The service now uses a Charm-backed structured logger behind the `slog` API shape.

Recommended modes:

```bash
# local development
export MEMORY_LOG_FORMAT='text'

# containers / aggregators
export MEMORY_LOG_FORMAT='json'
```

Useful options:

```bash
export MEMORY_LOG_LEVEL='debug'
export MEMORY_LOG_REPORT_CALLER='false'
export MEMORY_LOG_TIMESTAMPS='true'
```

Structured request, worker, embedder/cache, and recall logs are now emitted with consistent fields so local troubleshooting and container log aggregation are both easier.

## MCP Server

This repo now also ships an MCP server binary using the same `github.com/modelcontextprotocol/go-sdk` pattern as other `mordor-forge` MCP servers.

Run it over stdio:

```bash
go run ./cmd/agent-memory-mcp
```

Current MCP tools:

- `create_tenant`
- `create_agent`
- `create_thread`
- `remember`
- `list_memories`
- `recall`
- `get_memory_provenance`
- `list_projection_checkpoints`
- `list_projection_runs`
- `list_leases`
- `get_config`

Example Claude Code MCP configuration:

```json
{
  "mcpServers": {
    "agent-memory": {
      "command": "agent-memory-mcp",
      "env": {
        "MEMORY_DATABASE_URL": "postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable",
        "MEMORY_EMBEDDER_PROVIDER": "deterministic",
        "MEMORY_EMBEDDER_DIMENSIONS": "1536"
      }
    }
  }
}
```

The MCP layer uses camelCase tool arguments, consistent with your other MCP servers. For example, `remember` expects fields like `tenantId`, `agentId`, and `threadId`.

## Repository Notes

- The architecture overview and Mermaid diagrams live in `ARCHITECTURE.md`.
- Primitive guides live in:
  - `docs/primitives/state-snapshot.md`
  - `docs/primitives/fact-merge.md`
- The compatibility/versioning policy lives in `docs/adr/0004-api-compatibility-versioning.md`.
- The importer design lives in `docs/adr/0005-importer-framework.md`.
- The first import recipe lives in `docs/recipes/cursor-transcript-import.md`.
- Release scope and process notes live in `docs/release.md`.
- Deployment guidance lives in `docs/operations/deploy.md`.
- Backup/export/restore guidance lives in `docs/operations/backup-restore.md`.
- The repository is licensed under `Apache-2.0`. Historical license notes live in `LICENSE-OPTIONS.md`.
