-- destructive: approved
-- Drop media_asset_derivatives: display-tier pre-generation is deleted (ADR-0007 pull model); the workspacemedia record surface is gone, so no writer or reader remains.
DROP TABLE "media_asset_derivatives";
