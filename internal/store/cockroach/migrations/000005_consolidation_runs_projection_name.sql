-- +goose Up
ALTER TABLE consolidation_runs
    ADD COLUMN projection_name STRING NULL;

UPDATE consolidation_runs
SET projection_name = model
WHERE projection_name IS NULL
  AND model IS NOT NULL;

UPDATE consolidation_runs
SET model = NULL
WHERE projection_name IS NOT NULL
  AND provider = 'system'
  AND model = projection_name;

UPDATE consolidation_runs
SET provider = NULL
WHERE projection_name IS NOT NULL
  AND provider = 'system'
  AND model IS NULL;

ALTER TABLE consolidation_runs
    ALTER COLUMN projection_name SET NOT NULL;
