-- Issue #931: Library managed/external save-identity precheck.
--
-- The canonical managed-save use case dedups on (workspace, media_asset_id)
-- and external saves on (workspace, provider, source_id); Task identity is
-- task provenance only, never a collection identity. Both identities are
-- enforced by the partial UNIQUE indexes uq_library_assets_workspace_media
-- and uq_library_assets_workspace_source, so conforming data has no
-- duplicates to merge. This migration prechecks the existing rows before
-- any consumer relies on the converged identity:
--
-- - Duplicate identity groups (managed or external) stop the migration
--   with desensitized COUNTS only (no workspace, media, provider, or
--   source identifiers are logged).
-- - Within duplicate managed groups, groups whose rows disagree on
--   folder, display metadata, or task provenance are reported as
--   unmergeable conflicts; a conflict count > 0 stops the migration so
--   no user fact is deleted or overwritten without confirmation.
-- - On clean data the migration is a no-op that reports desensitized
--   totals (managed rows, rows carrying task lineage, rows without a
--   server-side operator) via NOTICE.
--
-- No rows are written, updated, or deleted here. Lossless rows keep
-- their facts untouched; conflicting data stops the migration instead
-- of being merged.

DO $$
DECLARE
  managed_dupe_groups BIGINT;
  external_dupe_groups BIGINT;
  conflict_groups BIGINT;
  managed_total BIGINT;
  lineage_total BIGINT;
  unattributed_total BIGINT;
BEGIN
  SELECT count(*) INTO managed_dupe_groups FROM (
    SELECT workspace_id, media_asset_id
    FROM library_assets
    WHERE media_asset_id IS NOT NULL
    GROUP BY workspace_id, media_asset_id
    HAVING count(*) > 1
  ) d;
  SELECT count(*) INTO external_dupe_groups FROM (
    SELECT workspace_id, provider, source_id
    FROM library_assets
    WHERE media_asset_id IS NULL
    GROUP BY workspace_id, provider, source_id
    HAVING count(*) > 1
  ) d;
  SELECT count(*) INTO conflict_groups FROM (
    SELECT workspace_id, media_asset_id
    FROM library_assets
    WHERE media_asset_id IS NOT NULL
    GROUP BY workspace_id, media_asset_id
    HAVING count(*) > 1
       AND (count(DISTINCT folder_id) > 1
         OR count(DISTINCT title) > 1
         OR count(DISTINCT author) > 1
         OR count(DISTINCT COALESCE(task_id, '')) > 1
         OR count(DISTINCT COALESCE(task_result_index, -1)) > 1)
  ) d;
  IF conflict_groups > 0 THEN
    RAISE EXCEPTION 'library identity precheck: % unmergeable duplicate groups (folder/metadata/lineage conflict, counts only); refusing to convert without confirmation',
      conflict_groups;
  END IF;
  IF managed_dupe_groups > 0 OR external_dupe_groups > 0 THEN
    RAISE EXCEPTION 'library identity precheck: % duplicate managed groups, % duplicate external groups (counts only); refusing to convert without confirmation',
      managed_dupe_groups, external_dupe_groups;
  END IF;
  SELECT count(*) INTO managed_total FROM library_assets WHERE media_asset_id IS NOT NULL;
  SELECT count(*) INTO lineage_total FROM library_assets WHERE media_asset_id IS NOT NULL AND task_id IS NOT NULL;
  SELECT count(*) INTO unattributed_total FROM library_assets WHERE media_asset_id IS NOT NULL AND saved_by_user_id IS NULL;
  RAISE NOTICE 'library identity precheck: % managed rows, % with task lineage, % without server operator (counts only)',
    managed_total, lineage_total, unattributed_total;
END $$;
