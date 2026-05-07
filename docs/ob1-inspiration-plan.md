# OB1-Inspired Action Plan for `agent-memory`

## Purpose
This document turns the useful conceptual lessons from [`OB1`](https://github.com/NateBJones-Projects/OB1) into an implementation plan for `agent-memory`.

The goal is **not** to copy OB1’s implementation stack or data model.

The goal **is** to adopt the parts that fit `agent-memory` well:

- stronger product framing
- ecosystem structure around the core
- import/capture workflows
- compatibility shims for weaker clients
- setup/troubleshooting quality as a first-class feature

This plan is intentionally scoped to **our architecture**:

- CockroachDB-first
- Go service + MCP
- `episodes` + projected `memories`
- provenance, checkpoints, leases
- multiple projection types

## What to Adopt

### 1. Sharper product framing
OB1 is very good at expressing a simple idea:

> one memory system, shared across many AI tools

That fits `agent-memory` well.

#### What to implement
- tighten README and docs messaging around:
  - shared memory infrastructure
  - one backend, many AI clients
  - memory as a system, not a notes app
- keep the language anchored in our architecture:
  - not “thoughts table”
  - but “raw episodes + projected memories”

#### Why it fits
This is a messaging/documentation improvement, not a code fork.

#### Suggested deliverables
- README positioning pass
- one “what this is / what this is not” page
- one “how this differs from chat memory and note apps” page

---

### 2. Ecosystem structure around the core
OB1’s repo structure makes it easy to grow use cases without bloating the core.

We should eventually introduce similar top-level categories, but adapted to our project:

- `recipes/`
- `integrations/`
- `primitives/`
- maybe `examples/`

#### What to implement
- `recipes/`
  - workflow-oriented implementations
  - example: session auto-capture
  - example: conversation import
- `integrations/`
  - client/system-specific adapters
  - example: `study-skill`
  - example: NotebookLM
  - example: Slack / Discord / email capture
- `primitives/`
  - reusable projection or ingestion patterns
  - example: `state_snapshot`
  - example: `fact_merge`
  - example: future dedup primitives
- `examples/`
  - minimal end-to-end usage
  - example: local HTTP + MCP smoke flow

#### Why it fits
This lets us keep the core generic while making the repo more approachable.

#### Suggested sequencing
- do **not** add all directories immediately
- add them when the first real content for each exists

---

### 3. Importers and migration workflows
This is the strongest practical lesson from OB1.

A memory system gets much more useful when it can ingest existing context instead of waiting for users to slowly rebuild it.

For `agent-memory`, importers should become one of the major next-stage capabilities.

#### What to implement
Build a generic importer framework that:

- reads source data
- normalizes it into `episodes`
- assigns source metadata
- uses idempotency and/or dedup
- optionally emits projection hints

#### Importer candidates
Priority order:

1. Claude / Cursor / MCP session import
2. ChatGPT export import
3. markdown / notes import
4. NotebookLM or study-workspace import
5. email / calendar import

#### Architecture fit
Importers should write **episodes**, not memories directly.

The core rule remains:
- raw import -> `episodes`
- projection workers -> `memories`

That preserves:
- provenance
- replayability
- projection flexibility

#### Suggested deliverables
- importer interface
- import job runner
- one simple importer first:
  - markdown or structured JSON conversation import
- one recipe doc demonstrating the flow

---

### 4. Compatibility shims
OB1’s “search/fetch aliases” idea is useful.

Some clients or environments cannot use the richest possible tool surface.

`agent-memory` should remain expressive, but we can add a compatibility layer for simpler clients.

#### What to implement
Potential compatibility MCP tools:

- `search`
  - alias or reduced-form recall
- `fetch`
  - fetch a specific memory by ID
- maybe `capture`
  - simplified wrapper over `remember`

These should not replace the richer tools; they should sit beside them.

#### Architecture fit
This is a surface-layer concern only.

Do not change the storage model for this.

#### Suggested sequencing
Implement only after:
- current MCP tools stabilize
- we know which clients actually benefit from aliases

---

### 5. Better capture workflows
OB1 is very focused on real-life capture.

We should not clone that exact UX, but we should take the principle seriously:

good memory systems need low-friction capture.

#### What to implement
Short-term:
- “auto-capture” style recipe patterns
- example end-of-session summaries
- example structured note capture

Longer-term:
- wrappers over `remember` that encode useful conventions
  - `remember_state`
  - maybe `remember_fact`
  - maybe `remember_context`

#### Architecture fit
This works very well with our projection system:
- capture raw episodes
- use payload conventions to trigger generic projections

That is already how:
- `state_snapshot`
- `fact_merge`

work today.

---

## What Not to Adopt

### 1. A flat “one thoughts table” model
This is the biggest thing **not** to import.

Our architecture is already stronger:

- `episodes`
- projected `memories`
- provenance
- checkpoints
- leases

Flattening back to one table would throw away important guarantees.

### 2. Tight coupling to one SaaS stack
OB1 is very oriented around:

- Supabase
- Edge Functions
- OpenRouter

That is fine for OB1, but not for us.

We should stay:
- backend-first
- self-hostable
- Cockroach-native
- provider-pluggable

### 3. Remote-MCP-first assumptions
OB1 leans into remote MCP flows.

We should stay local-first for MCP unless there is a strong reason to widen the trust surface.

### 4. Copying implementation or docs verbatim
OB1’s license is not a simple permissive one.

So the rule is:
- inspiration yes
- reimplementation yes
- direct code/docs transplant no

---

## Proposed Workstreams

### Workstream A: Positioning and documentation
#### Goal
Make the repo easier to understand as “shared memory infrastructure”.

#### Tasks
- update README framing
- add one “concepts” doc
- add one “what this is not” doc

#### Effort
Small

---

### Workstream B: Repo ecosystem scaffolding
#### Goal
Create a sustainable structure for future contributions and use cases.

#### Tasks
- add `recipes/` when first recipe exists
- add `integrations/` when first integration exists
- add `primitives/` for reusable projection conventions

#### Effort
Small to medium

---

### Workstream C: Importer framework
#### Goal
Let users bring existing context into the system.

#### Tasks
- define importer interface
- define import job execution pattern
- write first importer
- document import recipe

#### First candidate
Simple conversation JSON importer

#### Effort
Medium

---

### Workstream D: Capture recipes
#### Goal
Show opinionated but optional ways to capture useful memory events.

#### Tasks
- write “session auto-capture” recipe
- write “state snapshot capture” recipe
- write “fact merge capture” recipe

#### Effort
Small to medium

---

### Workstream E: Compatibility MCP tools
#### Goal
Support weaker or simpler MCP clients without reducing core expressiveness.

#### Tasks
- evaluate need for aliases
- add `search`
- add `fetch`
- maybe add simplified `capture`

#### Effort
Small

---

### Workstream F: Integration layer
#### Goal
Connect the core to real external workflows without making the core itself domain-specific.

#### Candidate integrations
- `study-skill`
- NotebookLM
- Slack / Discord
- markdown / notes vaults

#### Rule
Integrations live outside the core and rely on:
- HTTP
- MCP
- documented projection conventions

#### Effort
Medium to large

---

## Recommended Implementation Order

### Phase 1: Documentation and structure
1. improve positioning docs
2. add `primitives/` docs for current conventions
3. add first `recipe` doc

### Phase 2: Import path
1. importer interface
2. one concrete importer
3. import recipe

### Phase 3: Capture ergonomics
1. helper wrappers / convenience operations
2. auto-capture recipe
3. session summary recipe

### Phase 4: Integrations
1. `study-skill` integration plan
2. first implementation
3. additional adapters

### Phase 5: Compatibility shims
Only after we know they are worth having.

---

## Immediate Next Actions
If we want to act on this in a future session, the best next concrete tasks are:

1. Create `docs/primitives/` and write short docs for:
   - `state_snapshot`
   - `fact_merge`
2. Create `recipes/` and add:
   - session auto-capture recipe
3. Design an importer interface and implement:
   - first conversation import path

That is the shortest route from “interesting inspiration” to “real project value”.

## Summary
The right lesson from OB1 is not:

> “make `agent-memory` look like OB1”

It is:

> “make `agent-memory` easier to adopt, easier to extend, and easier to feed with real context”

That means:

- better framing
- ecosystem structure
- importers
- capture workflows
- compatibility shims only where they help

All of that fits our architecture well, as long as we keep the core centered on:

- episodes
- projected memories
- provenance
- replayability
