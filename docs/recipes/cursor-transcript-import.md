# Cursor Transcript Import Recipe

This recipe shows the smallest useful historical bootstrap flow:

1. import a Cursor agent transcript JSONL file as `episodes`
2. run the worker once
3. inspect projected memories and recall

## When To Use This

Use this when you already have a useful agent session on disk and want to turn it into searchable memory instead of waiting to capture future events only.

## Prerequisites

- local CockroachDB running
- environment loaded with `direnv allow` or `eval "$(./scripts/dev-env.sh)"`
- migrations applied with `go run ./cmd/memoryd migrate`

## 1. Create Scope

Create a tenant, agent, and thread first.

```bash
curl -X POST http://127.0.0.1:8080/v1/tenants \
  -H 'Content-Type: application/json' \
  -d '{"slug":"import-demo"}'

curl -X POST http://127.0.0.1:8080/v1/agents \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"<tenant-uuid>","name":"import-agent"}'

curl -X POST http://127.0.0.1:8080/v1/threads \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"<tenant-uuid>","agent_id":"<agent-uuid>"}'
```

## 2. Import The Transcript

```bash
go run ./cmd/memoryd import \
  --format=cursor-agent-jsonl \
  --file="/path/to/transcript.jsonl" \
  --tenant-id="<tenant-uuid>" \
  --agent-id="<agent-uuid>" \
  --thread-id="<thread-uuid>" \
  --source-id="cursor-session-001"
```

What this does:

- reads the JSONL file line by line
- normalizes each line into one imported episode
- uses deterministic idempotency keys so rerunning the same source is safe
- stores import metadata in episode payload under `payload.import`

## 3. Project Imported Episodes

```bash
go run ./cmd/memoryd worker --once
```

For the current alpha, imported transcript episodes are most naturally surfaced through the existing `episode_digest` projection.

## 4. Inspect The Result

List durable memories:

```bash
curl "http://127.0.0.1:8080/v1/memories?tenant_id=<tenant-uuid>&thread_id=<thread-uuid>"
```

Run semantic recall:

```bash
curl "http://127.0.0.1:8080/v1/recall?tenant_id=<tenant-uuid>&thread_id=<thread-uuid>&query=CockroachDB&limit=5"
```

Inspect projection state:

```bash
curl "http://127.0.0.1:8080/v1/projections/checkpoints?tenant_id=<tenant-uuid>"
curl "http://127.0.0.1:8080/v1/projections/runs?tenant_id=<tenant-uuid>&projection_name=episode-digests"
```

## Notes

- The first importer preserves transcript structure in episode payload instead of trying to infer higher-level snapshot or fact semantics automatically.
- Import writes only `episodes`; projections still own memory creation.
- The first supported format is intentionally narrow: Cursor agent transcript JSONL.
