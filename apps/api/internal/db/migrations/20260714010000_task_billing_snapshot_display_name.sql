-- migrate:up
ALTER TABLE task_billing_snapshots
  ADD COLUMN display_name VARCHAR(120);

ALTER TABLE task_billing_snapshots
  DISABLE TRIGGER task_billing_snapshots_immutable;

UPDATE task_billing_snapshots snapshots
SET display_name = skus.display_name
FROM billing_skus skus
WHERE skus.sku = snapshots.sku;

ALTER TABLE task_billing_snapshots
  ENABLE TRIGGER task_billing_snapshots_immutable;

ALTER TABLE task_billing_snapshots
  ALTER COLUMN display_name SET NOT NULL,
  ADD CONSTRAINT task_billing_snapshot_display_name_check
    CHECK (char_length(btrim(display_name)) BETWEEN 1 AND 120);

-- migrate:down
ALTER TABLE task_billing_snapshots
  DROP COLUMN IF EXISTS display_name;
