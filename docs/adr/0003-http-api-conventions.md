# ADR 0003: HTTP API Conventions

## Status
Accepted.

## Context
`agent-memory` already exposes a small versioned HTTP API for:

- resource creation
- raw episode ingestion
- memory inspection
- semantic recall

Before the API surface grows further, we need a stable shape for:

- success responses
- error responses
- list responses
- pagination semantics
- versioning expectations

Without this, later auth, provenance, and operational endpoints will drift into inconsistent formats.

## Decision
Versioned HTTP endpoints under `/v1/` will use a **consistent JSON envelope**.

### 1. Success envelope
All `/v1/` success responses use:

```json
{
  "data": { ... }
}
```

or for list-style responses:

```json
{
  "data": {
    "items": [ ... ]
  },
  "meta": {
    "count": 1,
    "limit": 100
  }
}
```

Rules:

- single-resource responses return the resource in `data`
- list-style responses return an `items` array inside `data`
- `meta` is used for list/collection semantics and is optional for singleton responses

### 2. Error envelope
All `/v1/` error responses use:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "human-readable explanation"
  }
}
```

Rules:

- `code` is a stable machine-facing identifier
- `message` is a human-readable description
- the message may be shown to operators and users in early alpha, but clients should key on `code`

### 3. Status code expectations
The intended mapping is:

- `400` invalid request / malformed input
- `401` unauthenticated
- `403` unauthorized
- `404` not found
- `405` method not allowed
- `409` conflict / idempotency conflict
- `500` internal server error

Not every code is implemented yet, but future endpoints should align to this mapping.

### 4. Pagination strategy
For `v0.1.x`, list endpoints use **limit-only pagination metadata**.

That means:

- clients may send `limit`
- responses include:
  - `meta.count`
  - `meta.limit`

Cursor-based pagination is intentionally deferred, but the envelope must leave room to add fields later such as:

- `nextCursor`
- `hasMore`

### 5. Versioning posture
The `/v1/` prefix is a **stability anchor**, not a promise of production-level permanence today.

The more detailed compatibility and versioning rules are defined in `docs/adr/0004-api-compatibility-versioning.md`.

For `v0.1.x`:

- the API is still considered alpha
- backward-incompatible changes are allowed if clearly documented in release notes

For later milestones:

- breaking changes should trigger either:
  - explicit versioning
  - or a documented deprecation path

### 6. Scope of this ADR
This ADR applies to:

- `/v1/` HTTP endpoints

This ADR does **not** require the same envelope for:

- `/livez`
- `/readyz`
- `/buildz`
- `/metrics`

Those are operational endpoints and may keep simpler shapes.

## Consequences

### Positive
- Makes the API easier to consume consistently.
- Keeps future endpoints aligned with auth and pagination work.
- Makes MCP parity easier, because HTTP semantics are clearer.

### Trade-offs
- Existing tests and examples need to be updated.
- Some fields may look slightly more verbose than the earlier ad hoc shapes.
- Cursor pagination will need future follow-up work.

## Immediate follow-through
- Refactor existing `/v1/` handlers to use the success/error envelope.
- Add list metadata for memory and recall list responses.
- Update tests and docs to match the new response shape.
