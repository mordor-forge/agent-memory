-- +goose Up
CREATE TABLE memory_embeddings (
    memory_id UUID PRIMARY KEY REFERENCES memories (id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    embedding_model STRING NOT NULL,
    embedding VECTOR(1536) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE VECTOR INDEX memory_embeddings_ann
ON memory_embeddings (tenant_id, agent_id, embedding vector_cosine_ops);
