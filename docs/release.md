# Release Process

## Current Status
`agent-memory` should currently be treated as a **`v0.1.x alpha`** project.

That means:

- useful and test-backed
- suitable for local and operator-guided dogfooding
- not yet promising stable APIs or mature remote deployment support

## What an Alpha Release Means
An alpha release in this project implies:

- the basic write / project / recall loop works
- HTTP and MCP surfaces are documented
- operator introspection exists
- local and single-service deployment paths are documented
- known limitations are explicitly called out

It does **not** imply:

- stable API compatibility
- complete auth/RBAC maturity
- production SLA claims
- fully battle-tested multi-instance scaling

Even in alpha, compatibility behavior is not undefined. The current policy lives in `docs/adr/0004-api-compatibility-versioning.md`.

## Release Numbering
Use semantic version tags:

- `v0.1.0-alpha.1`
- `v0.1.0-alpha.2`
- `v0.1.0`

Suggested rule:

- use `-alpha.N` while the API and operational model are still moving quickly
- use plain `v0.1.0` only when the alpha scope is intentionally declared and documented

## Pre-Tag Checklist
Before creating a release tag, verify:

1. `make fmt`
2. `go test ./...`
3. `go test -tags=integration ./...`
4. `go mod tidy` produces no diff
5. `goreleaser check` passes
6. `podman build -t localhost/agent-memory:test -f Containerfile .` succeeds
7. README and operational docs describe the actual current behavior
8. known limitations for the release are written down

## Release Scope Template
Each release should say:

- what changed
- what is still alpha/experimental
- any API changes
- any migration/config changes
- whether local/MCP/HTTP workflows changed

Suggested headings:

```markdown
## Highlights

## Breaking or Notable Changes

## Operational Notes

## Known Limitations
```

For any intentionally breaking change to `/v1/` HTTP behavior or MCP tool contracts, release notes should say:

- what changed
- why it was necessary
- whether a compatibility alias or migration path exists

## Release Artifacts
Current release output should include:

- `memoryd`
- `agent-memory-mcp`
- checksums
- SBOMs

## Known Alpha Constraints To Repeat In Releases
- HTTP auth is still first-pass and API-key based
- MCP is local-first
- Cockroach is the only supported backend
- tenant export exists, tenant import does not
- quotas/backpressure are process-local baselines, not distributed controls
- some deployment topologies are documented as possible but not yet first-class supported

## Release Metadata
GitHub release drafts should be used to keep alpha notes structured.

CI should validate:

- formatting
- unit tests
- integration tests
- `go mod tidy`
- GoReleaser config

## Supported Audience For Alpha
Current alpha releases are best suited for:

- the maintainer
- close collaborators
- experienced early adopters comfortable with evolving APIs

They are not yet targeted at:

- broad production rollouts
- zero-guidance operators
- multi-team remote service deployments

## Recommended First Public Alpha Gate
Before calling a release “public alpha”, the repo should at minimum have:

- clear version tag
- release notes
- current README
- CI passing
- integration tests passing
- GoReleaser config passing
- security/reporting contact updated if publishing publicly
