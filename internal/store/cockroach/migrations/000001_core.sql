-- +goose Up
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug STRING NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    external_ref STRING NULL,
    name STRING NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    INDEX agents_by_tenant (tenant_id, created_at, id)
);

CREATE TABLE threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    agent_id UUID NOT NULL REFERENCES agents (id),
    external_ref STRING NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, agent_id, external_ref),
    INDEX threads_by_agent (tenant_id, agent_id, created_at, id)
);

CREATE TABLE episodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    agent_id UUID NOT NULL REFERENCES agents (id),
    thread_id UUID NULL REFERENCES threads (id),
    idempotency_key STRING NULL,
    kind STRING NOT NULL,
    content STRING NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key),
    INDEX episodes_by_agent_ckpt (tenant_id, agent_id, created_at, id),
    INDEX episodes_by_thread_ckpt (tenant_id, thread_id, created_at, id)
);

CREATE TABLE consolidation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    worker_id UUID NOT NULL,
    batch_started_at TIMESTAMPTZ NOT NULL,
    input_from_created_at TIMESTAMPTZ NULL,
    input_from_id UUID NULL,
    input_to_created_at TIMESTAMPTZ NULL,
    input_to_id UUID NULL,
    status STRING NOT NULL,
    provider STRING NULL,
    model STRING NULL,
    error_text STRING NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    INDEX consolidation_runs_by_tenant (tenant_id, created_at, id)
);

CREATE TABLE memories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    agent_id UUID NOT NULL REFERENCES agents (id),
    thread_id UUID NULL REFERENCES threads (id),
    kind STRING NOT NULL,
    status STRING NOT NULL,
    content STRING NOT NULL,
    summary STRING NULL,
    attributes JSONB NOT NULL DEFAULT '{}',
    importance FLOAT8 NOT NULL DEFAULT 0.5,
    confidence FLOAT8 NOT NULL DEFAULT 0.5,
    supersedes_memory_id UUID NULL REFERENCES memories (id),
    last_observed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    INDEX memories_by_agent (tenant_id, agent_id, status, created_at, id),
    INDEX memories_by_thread (tenant_id, thread_id, status, created_at, id)
);

CREATE TABLE memory_provenance (
    memory_id UUID NOT NULL REFERENCES memories (id) ON DELETE CASCADE,
    episode_id UUID NOT NULL REFERENCES episodes (id),
    consolidation_run_id UUID NOT NULL REFERENCES consolidation_runs (id),
    role STRING NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (memory_id, episode_id)
);

CREATE TABLE projection_checkpoints (
    projection_name STRING NOT NULL,
    tenant_id UUID NOT NULL,
    shard_id INT8 NOT NULL,
    last_created_at TIMESTAMPTZ NULL,
    last_id UUID NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (projection_name, tenant_id, shard_id)
);

CREATE TABLE worker_leases (
    lease_name STRING PRIMARY KEY,
    holder_id UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    renewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB NOT NULL DEFAULT '{}'
);
