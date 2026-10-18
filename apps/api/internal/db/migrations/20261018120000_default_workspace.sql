-- destructive: approved
-- Default workspace product semantics (Bug2 root fix).
--
-- Every user owns exactly one live Default personal workspace (is_default).
-- Plain personal workspaces are unlimited, so the legacy single-personal
-- cap ux_workspaces_personal_owner is dropped (reviewed destructive step:
-- fresh-DB release, no backfill; see task Default workspace resolution).
-- Organization workspaces never carry the marker (chk_workspaces_default_scope).

ALTER TABLE workspaces ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE workspaces ADD CONSTRAINT chk_workspaces_default_scope CHECK (
  NOT is_default OR (organization_id IS NULL AND owner_user_id IS NOT NULL)
);

DROP INDEX ux_workspaces_personal_owner;

CREATE UNIQUE INDEX ux_workspaces_default_owner ON workspaces(owner_user_id) WHERE organization_id IS NULL AND is_default AND archived_at IS NULL;
