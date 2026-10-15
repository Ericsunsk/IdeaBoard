-- Board Preview Scene cutover (issues #925-929, Phase B): persist the
-- content identity and the Gallery scene projection with the accepted
-- document, and denormalize the envelope schema version so the catalog
-- serves cards without reading (detoasting) full documents.
--
-- Both projection columns are NULL-able so pre-cutover rows stay
-- readable; the backfill command assigns them post-deploy and the
-- document endpoint derives them on read until then. schema_version is
-- backfilled here deterministically from the canonical stored bytes
-- (every accepted document is envelope-validated, so the key is always
-- present and positive) and is NOT NULL afterwards: the catalog
-- contract requires it on every card. No snapshot bytes, versions, or
-- revisions are touched, and board ordering never shifts.
ALTER TABLE board_documents
  ADD COLUMN content_version TEXT NULL,
  ADD COLUMN preview_scene JSONB NULL,
  ADD COLUMN schema_version INT NULL;
UPDATE board_documents
SET schema_version = (snapshot_data ->> 'schema_version')::INT
WHERE schema_version IS NULL;
ALTER TABLE board_documents
  ALTER COLUMN schema_version SET NOT NULL,
  ADD CONSTRAINT chk_board_documents_content_version CHECK (content_version IS NULL OR length(content_version) BETWEEN 8 AND 128),
  ADD CONSTRAINT chk_board_documents_schema_version_positive CHECK (schema_version > 0);
