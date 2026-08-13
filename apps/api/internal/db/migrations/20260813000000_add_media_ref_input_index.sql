-- migrate:up

ALTER TABLE media_refs
  ADD COLUMN input_index INTEGER NOT NULL DEFAULT 0,
  ADD CONSTRAINT ck_media_refs_input_index CHECK (input_index >= 0);

CREATE UNIQUE INDEX uq_media_refs_task_input
  ON media_refs (task_id, input_index, role)
  WHERE ref_type = 'prompt_reference' AND task_id <> '';

CREATE UNIQUE INDEX uq_media_refs_prepared_input
  ON media_refs (owner_user_id, ref_id, input_index, role)
  WHERE ref_type = 'prompt_reference' AND task_id = '';

-- migrate:down

DROP INDEX uq_media_refs_task_input;
DROP INDEX uq_media_refs_prepared_input;
ALTER TABLE media_refs
  DROP CONSTRAINT ck_media_refs_input_index,
  DROP COLUMN input_index;
