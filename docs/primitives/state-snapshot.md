# State Snapshot Primitive

`state_snapshot` is the reusable projection for "keep the latest known state for this logical key".

Use it when you want later recall to find the newest state instead of every historical update as separate durable memories.

Common examples:

- current study or workflow checkpoint
- active task state
- latest operator note for a named slot
- current preference or mode for a thread

## Mental Model

Think of `episodes` as the event stream and `state_snapshot` as the current bookmark.

- every qualifying episode is still stored immutably in `episodes`
- the worker collapses those updates into one latest-state memory per logical key
- provenance still points back to the source episodes that contributed to that state

## How To Opt In

Add `snapshot_key` to the episode payload.

Minimal payload:

```json
{
  "snapshot_key": "resume_state"
}
```

That is enough to tell the worker:

- this episode participates in the snapshot projection
- the logical snapshot key is `resume_state`

## Optional Fields

Supported payload fields:

- `snapshot_key`
  - required
  - logical key for the state slot
- `snapshot_content`
  - optional override for the memory content
  - defaults to the episode `content`
- `snapshot_summary`
  - optional override for the memory summary
- `snapshot_kind`
  - optional memory kind
  - defaults to `state_snapshot`
- `snapshot_status`
  - optional memory status
  - defaults to `active`
- `snapshot_embed_text`
  - optional text used for embedding instead of derived default text
- `snapshot_importance`
  - optional numeric importance
  - defaults to `0.7`
- `snapshot_confidence`
  - optional numeric confidence
  - defaults to `1.0`
- `snapshot_attributes`
  - optional object merged into memory attributes

## Scope Rules

The logical key is namespaced by scope:

- thread-scoped if the episode has a `thread_id`
- agent-scoped otherwise

So these are different snapshots:

- `thread:<thread-id>:resume_state`
- `agent:<agent-id>:resume_state`

That keeps snapshots from unrelated threads from overwriting each other accidentally.

## Resulting Memory Behavior

For a given logical key:

- the latest episode wins for content, summary, status, and embed text
- `source_episode_ids` are accumulated and deduplicated
- `last_observed_at` tracks the newest relevant event time
- the resulting memory remains recallable like any other durable memory

The stored memory attributes always include at least:

- `snapshot_key`
- `source_kind`

## Example

Example episode body:

```json
{
  "tenant_id": "11111111-1111-1111-1111-111111111111",
  "agent_id": "22222222-2222-2222-2222-222222222222",
  "thread_id": "33333333-3333-3333-3333-333333333333",
  "kind": "state.update",
  "content": "resume on lesson 4 feedback step",
  "payload": {
    "snapshot_key": "resume_state",
    "snapshot_summary": "latest resume point",
    "snapshot_attributes": {
      "phase": "reviewing",
      "pending_action": "revise implementation"
    }
  }
}
```

After a worker pass, the corresponding durable memory will look roughly like:

- `kind: state_snapshot`
- `status: active`
- `content: "resume on lesson 4 feedback step"`
- `summary: "latest resume point"`
- `attributes.snapshot_key: "resume_state"`
- `attributes.phase: "reviewing"`

If a later episode arrives with the same scoped key, that later episode replaces the visible state while preserving provenance links.

## Good Uses

`state_snapshot` is a good fit when:

- only the latest state should be surfaced by default
- stale state should be overwritten naturally
- a client wants a compact "where should I resume?" style memory

## Common Gotcha

Do not use `state_snapshot` when each repeated observation should remain independently visible over time.

If you want repeated observations to accumulate into one durable fact instead of replacing each other, use `fact_merge` instead.
