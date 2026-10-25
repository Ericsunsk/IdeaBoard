-- User notifications queries (#958, ADR-0040).
--
-- The user_notifications table is the sole source of truth for user in-app alerts.
-- All user queries are strictly scoped by user_id for tenant isolation.

-- name: CreateNotification :one
INSERT INTO user_notifications (
  public_id, user_id, type, title, body, action_payload, deduplication_key
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(user_id), sqlc.arg(type),
  sqlc.arg(title), sqlc.arg(body), sqlc.arg(action_payload), sqlc.arg(deduplication_key)
)
ON CONFLICT (user_id, deduplication_key) WHERE deduplication_key <> ''
DO UPDATE SET updated_at = user_notifications.updated_at
RETURNING internal_id, public_id, user_id, type, title, body, action_payload, deduplication_key,
  status, read_at, created_at, updated_at;

-- name: GetNotificationByPublicID :one
SELECT internal_id, public_id, user_id, type, title, body, action_payload, deduplication_key,
  status, read_at, created_at, updated_at
FROM user_notifications
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: ListNotificationsByUserID :many
SELECT internal_id, public_id, user_id, type, title, body, action_payload, deduplication_key,
  status, read_at, created_at, updated_at
FROM user_notifications
WHERE user_id = sqlc.arg(user_id)
  AND ((sqlc.arg(status)::TEXT = '' AND status <> 'archived') OR status = sqlc.arg(status))
  AND (sqlc.arg(category)::TEXT IN ('', 'all')
    OR (sqlc.arg(category) = 'invites' AND type IN (
      'workspace_invite', 'workspace_member_joined', 'workspace_invite_declined',
      'organization_invite', 'organization_member_joined', 'organization_invite_declined',
      'studio_handover'))
    OR (sqlc.arg(category) = 'billing' AND type IN ('billing_credit_low', 'billing_recharge_success')))
ORDER BY
  CASE WHEN status = 'unread' THEN 0 ELSE 1 END,
  created_at DESC,
  internal_id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountUnreadNotifications :one
SELECT COUNT(*)
FROM user_notifications
WHERE user_id = sqlc.arg(user_id)
  AND status = 'unread';

-- name: CountNotificationsByUserID :one
SELECT COUNT(*)
FROM user_notifications
WHERE user_id = sqlc.arg(user_id)
  AND ((sqlc.arg(status)::TEXT = '' AND status <> 'archived') OR status = sqlc.arg(status))
  AND (sqlc.arg(category)::TEXT IN ('', 'all')
    OR (sqlc.arg(category) = 'invites' AND type IN (
      'workspace_invite', 'workspace_member_joined', 'workspace_invite_declined',
      'organization_invite', 'organization_member_joined', 'organization_invite_declined',
      'studio_handover'))
    OR (sqlc.arg(category) = 'billing' AND type IN ('billing_credit_low', 'billing_recharge_success')));

-- name: MarkNotificationAsRead :one
UPDATE user_notifications
SET status = 'read',
    read_at = COALESCE(read_at, now()),
    updated_at = now()
WHERE public_id = sqlc.arg(public_id)
  AND user_id = sqlc.arg(user_id)
RETURNING internal_id, public_id, user_id, type, title, body, action_payload, deduplication_key,
  status, read_at, created_at, updated_at;

-- name: MarkAllNotificationsAsRead :execrows
UPDATE user_notifications
SET status = 'read',
    read_at = COALESCE(read_at, now()),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND status = 'unread';

-- name: ArchiveNotification :one
UPDATE user_notifications
SET status = 'archived',
    updated_at = now()
WHERE public_id = sqlc.arg(public_id)
  AND user_id = sqlc.arg(user_id)
RETURNING internal_id, public_id, user_id, type, title, body, action_payload, deduplication_key,
  status, read_at, created_at, updated_at;

-- name: MarkInviteNotificationResolved :execrows
-- Keys 'invite_status'/'invite_id' must match
-- notifications.PayloadKeyInviteStatus/PayloadKeyInviteID and the frontend
-- InviteActionPayload interface; see notifications.PayloadKeyTransferInviteID
-- for why handover rows never match this merge.
UPDATE user_notifications
SET action_payload = COALESCE(action_payload, '{}'::jsonb) || jsonb_build_object('invite_status', sqlc.arg(invite_status)::TEXT),
    updated_at = now()
WHERE action_payload ->> 'invite_id' = sqlc.arg(invite_id)::TEXT
  AND type IN ('workspace_invite', 'organization_invite');

-- name: DeleteOldNotifications :execrows
DELETE FROM user_notifications
WHERE created_at < sqlc.arg(cutoff)::TIMESTAMPTZ;
