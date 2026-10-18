CREATE TABLE media_asset_task_origins (
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  task_public_id TEXT NOT NULL,
  output_index INT NOT NULL,
  media_asset_id BIGINT NOT NULL REFERENCES media_assets(internal_id) ON DELETE CASCADE,
  submitter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  board_public_id TEXT NULL,
  CONSTRAINT media_asset_task_origins_pkey PRIMARY KEY (workspace_id, task_public_id, output_index),
  CONSTRAINT chk_media_asset_task_origins_task CHECK (task_public_id LIKE 'task\_%'),
  CONSTRAINT chk_media_asset_task_origins_output_index CHECK (output_index >= 0),
  CONSTRAINT chk_media_asset_task_origins_board CHECK (board_public_id IS NULL OR board_public_id LIKE 'board\_%')
);

CREATE INDEX idx_media_asset_task_origins_asset ON media_asset_task_origins (media_asset_id);

INSERT INTO media_asset_task_origins (workspace_id, task_public_id, output_index, media_asset_id, submitter_user_id, board_public_id)
SELECT t.workspace_id, t.public_id, o.output_index, o.media_asset_id, t.submitter_user_id,
       NULLIF(btrim(t.workflow_input ->> 'board_id'), '')
FROM task_outputs AS o
INNER JOIN tasks AS t ON t.internal_id = o.task_id
INNER JOIN media_assets AS m ON m.internal_id = o.media_asset_id AND m.workspace_id = t.workspace_id
WHERE o.output_kind = 'media_asset';
