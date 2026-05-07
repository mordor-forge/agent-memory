# ADR 0004: API Compatibility and Versioning

## Status
Accepted.

## Context
`agent-memory` now has enough public surface area that accidental drift is a bigger risk than missing one more endpoint.

The project already exposes:

- versioned HTTP endpoints under `/v1/`
- a local-first MCP tool surface
- documented release automation and alpha positioning

Without an explicit compatibility policy, small implementation changes can silently become breaking changes for:

- HTTP clients
- MCP clients
- operators following examples and release notes

This ADR does not attempt to solve full storage migration compatibility yet. It defines the public contract rules that current code and tests should enforce.

## Decision

### 1. `/v1/` is the current HTTP compatibility line
The `/v1/` path prefix is the major-version boundary for the HTTP API.

That means:

- additive behavior is allowed within `/v1/`
- incompatible HTTP changes require a new major path or an explicitly documented alpha exception
- the path version, not the binary version, is the compatibility anchor for clients

### 2. `/v1/` uses explicit JSON media-type negotiation
Versioned HTTP responses advertise:

- `Content-Type: application/vnd.agent-memory.v1+json`
- `X-Agent-Memory-API-Version: v1`
- `X-Agent-Memory-API-Stability: alpha`

`/v1/` requests must follow these rules:

- `Accept` must allow either:
  - `application/json`
  - `application/vnd.agent-memory.v1+json`
  - a generic wildcard such as `*/*` or `application/*`
- JSON write endpoints must send either:
  - `application/json`
  - `application/vnd.agent-memory.v1+json`

If the client does not accept the versioned JSON response, the server returns:

- `406 not acceptable`

If the client sends an unsupported request media type for a JSON body, the server returns:

- `415 unsupported media type`

### 3. Backward-compatible response rules inside one API line
Within one HTTP API line:

- existing top-level envelope fields (`data`, `meta`, `error`) must remain stable
- existing error `code` values should remain stable
- additive JSON fields are allowed
- clients are expected to ignore unknown fields
- removing or renaming an existing field is a breaking change

### 4. Unknown `/v1/` paths still return the versioned JSON error shape
Unknown versioned endpoints must not fall back to ad hoc plain-text responses.

They should continue to return the `/v1/` error envelope so clients can rely on one error shape across the versioned surface.

### 5. MCP compatibility posture
The MCP server remains local-first, but it still has a public contract for tool-calling clients.

Within the current MCP surface:

- existing tool names are compatibility-sensitive
- existing required argument names are compatibility-sensitive
- additive optional arguments are allowed
- additive structured result fields are allowed
- removing or renaming a tool or required argument is a breaking change

The `get_config` tool should expose the current HTTP API line and stability level so MCP clients can reason about the paired HTTP surface without scraping docs.

### 6. Alpha posture
During `v0.1.x`:

- the project is still alpha
- compatibility discipline applies, but absolute permanence is not promised yet
- any intentional breaking change must be called out in release notes

This is stricter than “anything goes”, but still honest about alpha maturity.

## Consequences

### Positive
- Makes HTTP behavior machine-discussable rather than only path-documented.
- Gives tests a concrete contract to enforce.
- Makes MCP and HTTP compatibility expectations easier to explain together.
- Reduces the chance of accidental content-type, envelope, or error-shape drift.

### Trade-offs
- Some generic clients will now see `406` or `415` if they send the wrong headers.
- The project has to preserve version headers and media types once documented.
- Future major-version work will need deliberate duplication instead of silent mutation.

## Immediate follow-through
- Enforce `/v1/` media-type negotiation and compatibility headers in the HTTP server.
- Add contract tests for negotiation failures and unknown-path behavior.
- Expose HTTP API version metadata through MCP `get_config`.
- Reference this ADR from the main docs and release guidance.
