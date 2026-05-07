# Fact Merge Primitive

`fact_merge` is the reusable projection for "many repeated observations should converge into one durable fact".

Use it when repeated episodes reinforce the same idea and you want memory growth to stay compact instead of creating one durable row per observation.

Common examples:

- stable user preferences
- repeatedly observed environment facts
- durable project decisions
- preferences or constraints that become more trustworthy over time

## Mental Model

Think of `fact_merge` as evidence accumulation.

- every raw observation still lands in `episodes`
- matching observations collapse into one logical memory
- the merged memory gains stronger provenance and higher confidence as more evidence arrives

## How To Opt In

You can opt in in either of two ways.

Use a boolean flag:

```json
{
  "fact_merge": true
}
```

Or provide an explicit logical key:

```json
{
  "fact_key": "preferred_database"
}
```

If no explicit `fact_key` is provided, the worker derives one from normalized content and episode kind.

## Optional Fields

Supported payload fields:

- `fact_merge`
  - optional boolean opt-in
- `fact_key`
  - optional explicit logical key
- `fact_content`
  - optional override for the merged memory content
  - defaults to the episode `content`
- `fact_summary`
  - optional override for the memory summary
- `fact_kind`
  - optional memory kind
  - defaults to `fact_merge`
- `fact_status`
  - optional memory status
  - defaults to `active`
- `fact_embed_text`
  - optional text used for embedding instead of derived default text
- `fact_importance`
  - optional numeric importance
  - defaults to `0.6`
- `fact_attributes`
  - optional object merged into memory attributes

## Matching Rules

`fact_merge` ignores snapshot-marked episodes and only considers opted-in fact episodes.

When deriving a key automatically, the worker:

- trims surrounding whitespace
- lowercases the content
- collapses repeated internal whitespace
- hashes the normalized content together with the episode kind

Like snapshots, the logical fact key is scoped:

- thread-scoped if the episode has a `thread_id`
- agent-scoped otherwise

That means the same normalized fact in two different threads produces two different merged memories.

## Resulting Memory Behavior

Matching observations are merged into one durable memory with:

- `merge_strategy: accumulate`
- deduplicated `source_episode_ids`
- `attributes.observation_count`
- `attributes.first_observed_at`
- `attributes.last_observed_at`
- increasing `confidence` as observations accumulate

The latest observed episode wins for the visible content and summary when newer evidence arrives.

## Example

Example episode body:

```json
{
  "tenant_id": "11111111-1111-1111-1111-111111111111",
  "agent_id": "22222222-2222-2222-2222-222222222222",
  "kind": "note",
  "content": "CockroachDB is the preferred HA database",
  "payload": {
    "fact_merge": true,
    "fact_summary": "preferred HA database choice",
    "fact_attributes": {
      "source": "user_preference"
    }
  }
}
```

After repeated matching observations and worker passes, the merged memory will look roughly like:

- `kind: fact_merge`
- `status: active`
- `content: "CockroachDB is the preferred HA database"`
- `summary: "preferred HA database choice"`
- `attributes.normalized_content: "cockroachdb is the preferred ha database"`
- `attributes.observation_count: 3`
- `attributes.first_observed_at: "..."`
- `attributes.last_observed_at: "..."`

## Good Uses

`fact_merge` is a good fit when:

- repeated observations should reinforce one durable memory
- you want compact memory growth instead of one durable row per repeat
- confidence should increase with repeated evidence

## Common Gotcha

Do not use `fact_merge` when the latest event should replace earlier state entirely.

If you are modelling "current status" or "latest resume point", use `state_snapshot` instead. `fact_merge` accumulates evidence; it does not model a single canonical latest state slot.
