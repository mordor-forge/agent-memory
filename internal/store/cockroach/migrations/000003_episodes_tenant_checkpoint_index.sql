-- +goose Up
CREATE INDEX episodes_by_tenant_ckpt
ON episodes (tenant_id, created_at, id);
