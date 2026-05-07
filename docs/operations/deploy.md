# Deployment Guide

## Scope
This document describes the **currently supported** deployment shapes for `agent-memory`.

It is intentionally honest about what the project is good at today:

- local development
- local interactive use through Claude Code / Cursor
- single-service HTTP deployment
- local or remote CockroachDB backend

It does **not** claim that every possible topology is equally supported.

## Supported Runtime Roles
The project currently ships two binaries:

- `memoryd`
  - HTTP API
  - migrations
  - health checks
  - worker mode
- `agent-memory-mcp`
  - local-first stdio MCP server

## Supported Topologies

### 1. Local all-in-one development
Use:

- local Podman CockroachDB
- local `memoryd`
- local `agent-memory-mcp`

This is the default and best-tested setup.

See:

- `docs/operations/bootstrap.md`

### 2. Local app, remote CockroachDB
Use:

- local `memoryd`
- local `agent-memory-mcp`
- remote CockroachDB reachable over Tailscale or another trusted network path

This is a very good fit for interactive use when you want a more serious database backend but do not need to deploy the app itself.

### 3. Containerized `memoryd`, remote or local CockroachDB
Use:

- `Containerfile`
- one or more `memoryd` containers
- CockroachDB local or remote

This is useful for proving container behavior, structured JSON logs, and environment-driven configuration.

### 4. Single-site HA CockroachDB, local app
Recommended medium-term topology if you want real HA for the database but still interactive local use:

- 3 Cockroach nodes in one site
- one node per machine
- local `memoryd` / `agent-memory-mcp`

This is currently the most practical HA target for this project if the app remains primarily interactive.

## Not Yet a Supported Production Claim
The following are not rejected, but they are **not yet first-class documented deployment targets**:

- remote/public MCP exposure
- multi-site stretched CockroachDB clusters as the recommended default
- in-cluster `memoryd` + full GitOps application deployment
- auto-scaling `memoryd` with shared quotas/rate limiting

## HTTP API Deployment Notes
If you deploy `memoryd` as a long-running HTTP service:

- enable HTTP auth (`MEMORY_HTTP_AUTH_MODE=api_key`) outside local development
- prefer `MEMORY_LOG_FORMAT=json` in containers
- point the service at a reachable Cockroach SQL endpoint
- ensure migrations are run before serving production traffic

Recommended preflight sequence:

```bash
memoryd doctor
memoryd migrate
memoryd serve
```

## Worker Deployment Notes
Projection work can be run:

- interactively with `memoryd worker --once`
- continuously with `memoryd worker`

The worker uses:

- Cockroach-backed leases
- projection checkpoints

This means multiple worker processes are possible, but the current quota/rate-limit baseline is still process-local. For now, scale worker count conservatively and observe behavior before treating it as horizontally tuned.

## MCP Deployment Notes
`agent-memory-mcp` is designed first for:

- local stdio execution
- client-managed lifecycle

Recommended usage:

- Claude Code project-local MCP config
- Cursor project-local MCP config

Containerizing the MCP server is possible, but it is not the recommended default because stdio-based local execution is the best-supported mode today.

## Container Image Build
Build a local image with Podman:

```bash
podman build -t localhost/agent-memory:dev -f Containerfile .
```

The image contains:

- `/usr/local/bin/memoryd`
- `/usr/local/bin/agent-memory-mcp`

Default entrypoint:

```bash
memoryd serve
```

Override examples:

```bash
podman run --rm --network host localhost/agent-memory:dev doctor
podman run --rm --network host localhost/agent-memory:dev worker --once
podman run --rm --network host --entrypoint agent-memory-mcp localhost/agent-memory:dev
```

## Configuration Guidance
Minimum useful environment for local or remote HTTP service:

```bash
export MEMORY_DATABASE_URL='postgresql://...'
export MEMORY_HTTP_ADDR=':8080'
export MEMORY_HTTP_AUTH_MODE='api_key'
export MEMORY_HTTP_AUTH_HEADER='Authorization'
export MEMORY_HTTP_AUTH_SCHEME='Bearer'
export MEMORY_HTTP_AUTH_PRINCIPALS_JSON='[...]'
export MEMORY_EMBEDDER_PROVIDER='deterministic'
export MEMORY_LOG_FORMAT='json'
```

For OpenAI-compatible embeddings:

```bash
export MEMORY_EMBEDDER_PROVIDER='openai'
export MEMORY_EMBEDDER_MODEL='text-embedding-3-small'
export MEMORY_EMBEDDER_DIMENSIONS='1536'
export MEMORY_OPENAI_API_KEY='...'
```

## Operational Checklist
Before calling a deployment “serious”, verify:

- migrations applied
- auth enabled
- rate limiting configured deliberately
- embedder provider and dimensions confirmed
- log mode appropriate for the runtime
- backup/export plan decided
- health endpoints reachable
- worker projection flow validated with a real smoke test

## Smoke Test
Minimal service verification after deploy:

1. `GET /livez`
2. `GET /readyz`
3. `GET /buildz`
4. create tenant/agent/thread
5. append episode
6. run worker once
7. list memories
8. recall
9. inspect provenance
