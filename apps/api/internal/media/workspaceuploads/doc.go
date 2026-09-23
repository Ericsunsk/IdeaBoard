// Package workspaceuploads owns workspace-scoped upload sessions and the
// workspace binding of media assets for the Public Data Plane v1
// (issue #803).
//
// Protocol split (Baseline 6.3):
//
//   - Upload sessions are EPHEMERAL transfer coordination only. They live in
//     the process-local SessionStore, never in PostgreSQL: abandoned
//     sessions create no rows and therefore need no reclamation. Only a
//     successful complete creates the durable Media Asset row.
//   - Media assets reuse the existing media_assets table with NO schema
//     change: the workspace binding travels in metadata.workspace_id and
//     the archive lifecycle marker in metadata.archived_at. Storage keys
//     never leave the server; content URLs carry asset_... only.
//
// Session state machine: pending -> transferring -> completed,
// transferring -> failed, pending/transferring -> expired. Transfer start
// is signaled by the first session read after create (the client fetched
// the transfer instructions); expiry is LAZY on read (no background
// sweeper in this slice: sessions hold no durable resources, so a sweeper
// would only bound memory, which the TTL + capped store already does).
//
// Retention TTLs (see also the report):
//
//   - UploadPendingTTL (30m): pending -> expired without transfer start.
//   - UploadTransferTTL (2h): transferring -> expired without complete.
//   - WorkspaceAssetAbandonedTTL (7d): reclaim_after stamped on assets
//     whose bytes were never verified; the existing media reclamation
//     scanner reclaims them once due unless Board/Task/Library refs
//     protect them (#805/#806 own those refs).
//   - WorkspaceArchivedAssetRetentionTTL (30d): reclaim_after stamped on
//     archive; archived assets stay metadata-readable until reclaimed.
//
// Byte transfer itself is out of scope: create returns an opaque
// same-origin slot (no bucket/secret/path), and byte PUT + presigned
// variants land with #805. Complete validates declarations only
// (ownership via workspace membership, MIME allowlist, size bounds,
// checksum match, declared media kind); storage-level integrity
// verification runs when the byte backend lands.
package workspaceuploads
