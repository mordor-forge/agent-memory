# Bootstrap Notes

## Local CockroachDB

```bash
make dev-up
```

## Export Local Environment

```bash
direnv allow
# or, without direnv:
eval "$(./scripts/dev-env.sh)"
```

If you use fish, make sure your shell config includes:

```fish
direnv hook fish | source
```

Local development intentionally uses:

```bash
export MEMORY_HTTP_AUTH_MODE='disabled'
```

This is a dev-only bypass for the HTTP API.

## Run Migrations

```bash
go run ./cmd/memoryd migrate
```

## Choose an Embedder

Deterministic local/dev mode:

```bash
export MEMORY_EMBEDDER_PROVIDER='deterministic'
export MEMORY_EMBEDDER_DIMENSIONS='1536'
```

## Logging

Local/dev:

```bash
export MEMORY_LOG_FORMAT='text'
```

Containers:

```bash
export MEMORY_LOG_FORMAT='json'
```

OpenAI-compatible mode:

```bash
export MEMORY_EMBEDDER_PROVIDER='openai'
export MEMORY_EMBEDDER_MODEL='text-embedding-3-small'
export MEMORY_EMBEDDER_DIMENSIONS='1536'
export MEMORY_OPENAI_API_KEY='<api-key>'
```

## Enable HTTP Auth

For non-local HTTP use, switch to API-key mode:

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

## Rate Limiting and Embed Batch Bounds

```bash
export MEMORY_HTTP_RATE_LIMIT_RPS='5'
export MEMORY_HTTP_RATE_LIMIT_BURST='10'
export MEMORY_EMBED_MAX_BATCH='32'
```

## Run a Worker Pass

```bash
go run ./cmd/memoryd worker --once
```

## Run MCP Server

```bash
go run ./cmd/agent-memory-mcp
```

## Stop or Reset Local CockroachDB

```bash
make dev-down
# or wipe the persistent volume too
make dev-reset
```

## Health Endpoints

- `/livez`: process liveness
- `/readyz`: dependency readiness
- `/metrics`: Prometheus metrics
- `/buildz`: build metadata
- `/v1/tenants`: create tenants with `POST`
- `/v1/agents`: create agents with `POST`
- `/v1/threads`: create threads with `POST`
- `/v1/episodes`: append raw episodes with `POST`
- `/v1/tenants/{id}/export`: export tenant-scoped data
- `/v1/tenants/{id}` with `DELETE`: delete tenant-scoped data
- `/v1/memories`: inspection-oriented durable memory listing
- `/v1/memories/{id}/provenance`: explain why a memory exists
- `/v1/projections/checkpoints`: projection checkpoint state for a tenant
- `/v1/projections/runs`: recent projection/consolidation runs for a tenant
- `/v1/leases`: current worker leases
- `/v1/recall`: semantic nearest-neighbor recall over `memory_embeddings`

For `/v1/` compatibility:

- JSON write requests may use `Content-Type: application/json`
- `/v1/` responses use `Content-Type: application/vnd.agent-memory.v1+json`
- `/v1/` responses also include:
  - `X-Agent-Memory-API-Version: v1`
  - `X-Agent-Memory-API-Stability: alpha`
- unsupported `Accept` headers return `406`
- unsupported JSON write `Content-Type` values return `415`

## Snapshot Projection Convention

To project a latest-state memory from a raw episode, include `snapshot_key` in the episode payload.

Example:

```json
{
  "snapshot_key": "resume_state",
  "snapshot_content": "resume on lesson 4 feedback step",
  "snapshot_summary": "latest resume point",
  "snapshot_attributes": {
    "phase": "reviewing"
  }
}
```

## Fact Merge Projection Convention

To collapse repeated identical observations into one durable fact memory, include either:

```json
{
  "fact_merge": true
}
```

or:

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
  "fact_attributes": {
    "source": "user_preference"
  }
}
```

## Secret Hygiene Notes

- Keep real `MEMORY_OPENAI_API_KEY` values out of committed files.
- Keep real `MEMORY_HTTP_AUTH_PRINCIPALS_JSON` values out of committed files.
- Prefer `.envrc.local` or another local secret source for developer credentials.
- Rotate secrets by updating the source of truth and restarting `memoryd` / `agent-memory-mcp`.
