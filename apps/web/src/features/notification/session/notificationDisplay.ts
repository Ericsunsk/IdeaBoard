import type { NotificationDTO } from '@/api/generated-query/public/model'
import {
  classifyInviteStatus,
  resolveNotificationActions,
  type InviteTerminalDecision,
  type NotificationStatusPill,
} from './notificationActionRegistry'

/**
 * Local invite outcome decided in this client session.
 * `joined` maps to payload `invite_status: 'accepted'`, `declined` to `'declined'`.
 */
export type InviteLocalDecision = InviteTerminalDecision

/** Session-owned optimistic status patch for one notification. */
export interface NotificationStatusOverride {
  status: NotificationDTO['status']
  readAt?: string | null
}

/**
 * Single derived view of a notification. Session (`useNotifications`) is the
 * sole owner of the inputs (server `invite_status` + read/archived status +
 * local decision); UI layers only render `pill` and never keep their own
 * optimistic copies.
 */
export interface NotificationDerivedState {
  status: NotificationDTO['status']
  inviteDecision: InviteLocalDecision | null
  pill: NotificationStatusPill | undefined
}

/** Read the invite outcome carried by the server payload, if any. Delegates
 * to the registry's `classifyInviteStatus` (single source); this module
 * owns only the local-decision write-back below. */
export function readPayloadInviteDecision(
  notification: NotificationDTO,
): InviteLocalDecision | null {
  const payload = (notification.action_payload as Record<string, unknown> | undefined) ?? {}
  const inviteStatus = typeof payload.invite_status === 'string' ? payload.invite_status : ''
  return classifyInviteStatus(inviteStatus)
}

function withInviteStatus(
  notification: NotificationDTO,
  decision: InviteLocalDecision,
): NotificationDTO {
  return {
    ...notification,
    action_payload: {
      ...((notification.action_payload as Record<string, unknown> | undefined) ?? {}),
      // Write-back stays canonical: a local 'joined' decision lands as
      // 'accepted' (the only accepted value the server emits), so the
      // optimistic pill matches the post-round-trip payload exactly.
      invite_status: decision === 'joined' ? 'accepted' : 'declined',
    },
  }
}

/**
 * `invite_status` + read/archived + 本地决策 → pill/行动的唯一派生入口.
 * A local decision wins over the payload so the pill renders identically
 * before the server round-trip settles; pill copy itself stays in
 * `notificationActionRegistry` (single source).
 */
export function resolveNotificationDerivedState(
  notification: NotificationDTO,
  localDecision?: InviteLocalDecision | null,
): NotificationDerivedState {
  const inviteDecision = localDecision ?? readPayloadInviteDecision(notification)
  const effective =
    localDecision != null ? withInviteStatus(notification, localDecision) : notification
  return {
    status: notification.status,
    inviteDecision,
    pill: resolveNotificationActions(effective)?.status,
  }
}

/**
 * Merge session-owned overrides onto the current fact snapshot. Returns the input order; items without overrides keep identity.
 */
export function applyNotificationViewOverrides(
  items: readonly NotificationDTO[],
  overrides: {
    statuses: ReadonlyMap<string, NotificationStatusOverride>
    decisions: ReadonlyMap<string, InviteLocalDecision>
  },
): NotificationDTO[] {
  if (overrides.statuses.size === 0 && overrides.decisions.size === 0) return [...items]
  return items.map((item) => {
    const status = overrides.statuses.get(item.id)
    const decision = overrides.decisions.get(item.id)
    if (!status && decision == null) return item
    const next: NotificationDTO = { ...item }
    if (status) {
      next.status = status.status
      if (status.readAt !== undefined) next.read_at = status.readAt
    }
    if (decision != null) {
      return withInviteStatus(next, decision)
    }
    return next
  })
}
