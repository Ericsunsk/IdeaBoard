-- Board media access (#936).
--
-- board_document_media_refs is the durable join fact behind asset
-- authorization for board readers: one row per (board, media asset)
-- accepted into the board's persisted document snapshot. Reconciled in
-- the same transaction as every document accept; board or asset removal
-- cascades; archive/restore change no row (authorization revalidates the
-- live board role on every read).
--
-- upload_sessions.target_board_id is the restricted upload target: the
-- board a Canvas/Library/Search upload is captured for, recorded at
-- session create (pre-complete) and verified by the handler against the
-- caller's board write access. NULL means no board binding yet (e.g. the
-- board is not autosaved): the resulting asset stays owner-only until a
-- board document accept, library save, or task output grants wider
-- access. Cleared when the board row goes away; the asset survives.
CREATE TABLE board_document_media_refs (
  board_id BIGINT NOT NULL REFERENCES boards (internal_id) ON DELETE CASCADE,
  media_asset_id BIGINT NOT NULL REFERENCES media_assets (internal_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (board_id, media_asset_id)
);
CREATE INDEX idx_board_document_media_refs_asset ON board_document_media_refs (media_asset_id, board_id);
ALTER TABLE upload_sessions ADD COLUMN target_board_id BIGINT NULL REFERENCES boards (internal_id) ON DELETE SET NULL;
CREATE INDEX idx_upload_sessions_target_board ON upload_sessions (target_board_id) WHERE target_board_id IS NOT NULL;

WITH documents AS (
  SELECT b.internal_id AS board_id, b.workspace_id,
         CASE WHEN d.snapshot_data ? 'payload' THEN d.snapshot_data -> 'payload'
              ELSE d.snapshot_data END AS payload
  FROM boards AS b
  INNER JOIN board_documents AS d ON d.board_id = b.internal_id
), media_references AS (
  SELECT d.board_id, d.workspace_id, btrim(ref.value #>> '{}') AS asset_public_id
  FROM documents AS d
  CROSS JOIN LATERAL (VALUES
    ('$.imageNodes[*].sourceMediaRef.assetId'::jsonpath),
    ('$.imageNodes[*].outputImages[*].mediaRef.assetId'::jsonpath),
    ('$.promptNodes[*].inputImages[*].mediaRef.assetId'::jsonpath),
    ('$.fusionNodes[*].inputImages[*].mediaRef.assetId'::jsonpath),
    ('$.fusionNodes[*].outputImages[*].mediaRef.assetId'::jsonpath)
  ) AS paths(path)
  CROSS JOIN LATERAL jsonb_path_query(d.payload, paths.path) AS ref(value)
  WHERE jsonb_typeof(ref.value) = 'string'
)
INSERT INTO board_document_media_refs (board_id, media_asset_id)
SELECT DISTINCT r.board_id, m.internal_id
FROM media_references AS r
INNER JOIN media_assets AS m ON m.public_id = r.asset_public_id AND m.workspace_id = r.workspace_id
WHERE r.asset_public_id ~ '^asset_[A-Za-z0-9_-]+$'
ON CONFLICT (board_id, media_asset_id) DO NOTHING;
