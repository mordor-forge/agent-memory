# ADR 0005: Importer Framework

## Status
Accepted.

## Context
`agent-memory` already supports:

- append-only `episodes`
- projection workers that derive durable `memories`
- provenance, checkpoints, run inspection, and recall

The biggest remaining practical gap is getting existing context into the system.

Right now:

- tenant export exists
- tenant import does not
- memory formation works well for newly appended events
- there is no first-class way to ingest prior agent or conversation history

We need an import design that fits the current architecture instead of bypassing it.

## Decision

### 1. Importers write `episodes`, not `memories`
All imports will flow through the same core invariant as live ingestion:

- source data -> normalized import records
- normalized import records -> `episodes`
- worker projections -> `memories`

Importers must not write `memories`, `memory_embeddings`, or provenance rows directly.

This preserves:

- replayability
- projection flexibility
- provenance consistency
- one mental model for "live capture" and "historical import"

### 2. Phase 1 import is a local CLI workflow
The first importer surface will be a local administrative CLI path under `memoryd`, not HTTP or MCP.

The intended shape is:

```bash
go run ./cmd/memoryd import \
  --format=cursor-agent-jsonl \
  --file=/path/to/transcript.jsonl \
  --tenant-id=<tenant-uuid> \
  --agent-id=<agent-uuid> \
  --thread-id=<thread-uuid> \
  --source-id=<optional-stable-source-id>
```

Why CLI first:

- import is bulk and operator-oriented
- the first source is a local file format
- this avoids inventing remote trust and upload semantics too early

### 3. Internal structure
The importer subsystem should live under `internal/importer`.

The minimal phase-1 shape is:

- importer registry by format name
- per-format parser/normalizer
- runner that converts normalized records into `memory.AppendEpisodeRequest`
- import report with counts and failures

Suggested internal concepts:

- `Importer`
  - declares `Format() string`
  - reads a source and emits normalized records
- `Record`
  - normalized import unit before persistence
- `Sink`
  - writes imported records via `AppendEpisode`
- `Report`
  - summarizes read, appended, deduped, skipped, and failed rows

### 4. Deterministic idempotency is required
Imports must be safely repeatable.

Phase-1 rule:

- every imported record gets a deterministic `idempotency_key`
- rerunning the same source should not duplicate equivalent episode rows

For the first transcript importer, the idempotency key should be derived from:

- importer format
- stable `source_id`
- source ordinal, such as line number

Recommended pattern:

```text
import:cursor-agent-jsonl:<source-id>:<line-number>
```

If `--source-id` is not provided, the importer may derive it from the source file name or session identifier.

### 5. Source metadata is preserved in payload
Imported episodes should keep enough metadata to explain where they came from.

At minimum, imported payloads should record:

- importer format
- source identifier
- source file path or logical source name
- source line number or ordinal
- original role and content block metadata when available

The import metadata belongs in `payload`, not in new top-level episode columns.

### 6. First concrete importer: Cursor agent transcript JSONL
The first narrow importer will target Cursor/agent transcript JSONL files because:

- they exist in the current development workflow
- they are line-oriented and easy to fixture
- they already represent valuable long-form memory context

Phase-1 scope for this importer:

- one imported episode per JSONL line
- recognize top-level `role`
- concatenate text blocks into semantic `content`
- retain raw block metadata in `payload`
- derive useful content summaries for lines that contain tool blocks but little or no plain text

Suggested initial kind mapping:

- `user` -> `conversation.user_message`
- `assistant` -> `conversation.assistant_message`
- unknown or future roles -> `conversation.message`

The first importer should favor preserving data over over-interpreting it.

### 7. Projection hints remain optional
The framework may attach projection hints through payload when a source format truly supports them, but this is optional.

Phase 1 should not try to infer `state_snapshot` or `fact_merge` automatically from every transcript line.

The first importer should land useful raw episodes first, then let existing generic projections do their normal work.

### 8. Worker execution stays separate
Import and projection remain separate concerns.

The importer command should not silently mutate the projection model.

Acceptable phase-1 behavior:

- import only
- optionally document that the next step is `memoryd worker --once`

An optional convenience flag to run the worker later is acceptable, but not required for the first implementation.

## Consequences

### Positive
- Makes historical context ingestion fit the current architecture cleanly.
- Reuses existing idempotency, projection, provenance, and recall behavior.
- Avoids designing an unnecessary import-specific write path.
- Gives the project a concrete bridge from local transcripts to durable memory.

### Trade-offs
- The first importer will be intentionally narrow.
- Import remains a local/operator workflow before it becomes a service feature.
- The first transcript importer may preserve some raw structure in payload instead of fully normalizing every possible block type.

## Non-goals
- Building a full tenant re-import workflow immediately
- Automatic import of every transcript or chat export format
- Remote HTTP upload endpoints for bulk imports
- Inference-heavy automatic snapshot/fact tagging from transcript text

## Immediate Follow-through
- Add `internal/importer` and a first concrete importer implementation.
- Add a `memoryd import` CLI entry point.
- Add fixture-backed tests for one transcript file.
- Document the end-to-end import -> worker -> recall flow.
