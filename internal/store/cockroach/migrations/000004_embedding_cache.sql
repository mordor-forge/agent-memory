-- +goose Up
CREATE TABLE embedding_cache (
    provider STRING NOT NULL,
    model STRING NOT NULL,
    text_hash STRING NOT NULL,
    text STRING NOT NULL,
    embedding VECTOR(1536) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, model, text_hash)
);
