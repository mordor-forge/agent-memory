# ADR 0002: Auth and Principal Model

## Status
Accepted.

## Context
`agent-memory` now exposes:

- a versioned HTTP API
- a local-first MCP server
- write and read operations across tenant-scoped memory data

The project currently assumes a trusted local environment. That is acceptable for development and interactive use, but not sufficient for a publishable service. We need an early decision on the trust model so API evolution, error semantics, and tenant boundaries do not drift into an unauthenticated shape that becomes hard to fix later.

The project also has two different access surfaces:

- **HTTP**, which may eventually be deployed remotely
- **MCP**, which is currently intended for local interactive use from tools like Claude Code and Cursor

These surfaces do not need identical trust models.

## Decision
`agent-memory` will use a **principal-based** security model, with different expectations for HTTP and MCP.

### 1. Principal model
Every authenticated request is associated with a principal:

- `subject`
  - stable identifier for the caller
- `tenant grants`
  - list of tenant IDs the principal may read and/or write
- optional `role`
  - used for future admin/operator operations

This is the authorization model the service is designed toward, even before all enforcement is implemented.

### 2. HTTP trust model
The HTTP API is treated as the surface that will eventually need real remote security.

The intended production direction is:

- authenticated requests by default
- explicit tenant authorization checks
- secure-by-default behavior

For early alpha:

- local development may use an explicit bypass mode
- this bypass must remain clearly documented as non-production behavior

This ADR does **not** lock in whether production auth will be:

- API key
- OIDC/JWT
- mTLS
- or a hybrid

It does lock in that:

- HTTP is the surface that carries the real authn/authz model
- tenant authorization is mandatory before production claims

### 3. MCP trust model
The MCP server is treated as **local-first**.

That means:

- the supported default usage is a local stdio child process launched by an MCP client
- it inherits trust from the local user session and host environment
- it is **not** treated as a remotely exposed production API in `v0.1.x`

If remote MCP support is ever introduced later, it must be treated as a new trust boundary and re-evaluated separately.

### 4. Authorization boundary
Tenant is the primary authorization boundary.

All externally meaningful operations should be shaped so that:

- tenant scope is explicit
- cross-tenant access is impossible without explicit permission

### 5. Error posture
Authn/authz failures must be distinguishable from validation failures.

The intended HTTP semantics are:

- `401` for unauthenticated
- `403` for authenticated but not authorized
- `400` for structurally invalid requests

## Consequences

### Positive
- Prevents the HTTP API from accidentally becoming “public and unauthenticated by design”.
- Keeps MCP ergonomic for local interactive use.
- Gives future implementation work a stable authorization target.
- Keeps tenant scoping central in API and projection design.

### Trade-offs
- Some early HTTP examples and tooling may stay “local/dev only” until auth is implemented.
- There is extra complexity in later adding principal extraction and tenant grant enforcement.
- MCP and HTTP will intentionally have different trust assumptions, which must be documented clearly.

### Non-goals of this ADR
- Choosing the final production auth mechanism
- Designing RBAC in detail
- Defining secret storage/rotation procedures
- Defining remote MCP deployment support

## Immediate follow-through
- Write an HTTP API conventions ADR that assumes auth failures will need their own error envelope semantics.
- Keep versioned API endpoints tenant-explicit.
- Add local-dev wording to docs rather than implying remote readiness.
- Reserve room in handlers/middleware for principal injection and tenant grant checks.
