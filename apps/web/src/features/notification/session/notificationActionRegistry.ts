import type { NotificationDTO } from '@/api/generated-query/public/model'

type NotificationActionVariant = 'primary' | 'outline' | 'secondary'

export interface NotificationActionItem {
  id: string
  labelKey: string
  variant?: NotificationActionVariant
  testId?: string
  execute: (context: NotificationActionContext) => Promise<boolean>
}

export interface NotificationStatusPill {
  statusKey: string
  labelKey: string
  testId?: string
}

interface NotificationActionContext {
  notification: NotificationDTO
  payload: Record<string, unknown>
}

export interface NotificationActionResolution {
  status?: NotificationStatusPill
  actions: NotificationActionItem[]
}

/**
 * Terminal invite outcome shared by the registry and the display layer.
 * `joined` maps to payload `invite_status: 'accepted'`, `declined` to
 * `'declined'` (the write-back direction lives in `notificationDisplay`).
 */
export type InviteTerminalDecision = 'joined' | 'declined'

/**
 * Single owner of the terminal invite_status mapping. Both the action
 * resolver below and `notificationDisplay.readPayloadInviteDecision`
 * delegate here so the pill can never diverge between layers.
 */
export function classifyInviteStatus(inviteStatus: string): InviteTerminalDecision | null {
  if (inviteStatus === 'accepted') return 'joined'
  if (inviteStatus === 'declined' || inviteStatus === 'cancelled' || inviteStatus === 'expired') {
    return 'declined'
  }
  return null
}

/**
 * Typed invite-family payload (backend `notifications.InviteActionPayload`).
 * Wire keys must stay identical on both sides: invite_id / invite_status /
 * role / workspace_id / workspace_name / organization_id /
 * organization_name / member_user_id / member_email / invitee_email.
 */
interface InviteActionPayload {
  invite_id: string
  invite_status?: string
  role?: string
  workspace_id?: string
  workspace_name?: string
  organization_id?: string
  organization_name?: string
  member_user_id?: string
  member_email?: string
  invitee_email?: string
}

type NotificationActionResolver = (
  notification: NotificationDTO,
  handlers?: {
    onAcceptInvite?: ((inviteId: string, notificationId: string) => Promise<boolean>) | undefined
    onDeclineInvite?: ((inviteId: string, notificationId: string) => Promise<boolean>) | undefined
  },
) => NotificationActionResolution | null

const resolvers = new Map<string, NotificationActionResolver>()

/**
 * Registers an action resolver for a notification type.
 */
export function registerNotificationActionResolver(
  type: string,
  resolver: NotificationActionResolver,
): void {
  resolvers.set(type, resolver)
}

function readInvitePayload(payload: Record<string, unknown>): InviteActionPayload {
  const out: InviteActionPayload = { invite_id: String(payload.invite_id ?? '') }
  if (payload.invite_status != null) out.invite_status = String(payload.invite_status)
  if (payload.role != null) out.role = String(payload.role)
  if (payload.workspace_id != null) out.workspace_id = String(payload.workspace_id)
  if (payload.workspace_name != null) out.workspace_name = String(payload.workspace_name)
  if (payload.organization_id != null) out.organization_id = String(payload.organization_id)
  if (payload.organization_name != null) out.organization_name = String(payload.organization_name)
  if (payload.member_user_id != null) out.member_user_id = String(payload.member_user_id)
  if (payload.member_email != null) out.member_email = String(payload.member_email)
  if (payload.invitee_email != null) out.invitee_email = String(payload.invitee_email)
  return out
}

function createInviteResolver(): NotificationActionResolver {
  return (notification, handlers) => {
    const payload = (notification.action_payload as Record<string, unknown> | undefined) ?? {}
    const invite = readInvitePayload(payload)
    const inviteId = invite.invite_id
    const decision = classifyInviteStatus(invite.invite_status ?? '')

    if (decision === 'joined') {
      return {
        status: {
          statusKey: 'joined',
          labelKey: 'notification.actions.joined',
          testId: 'invite-status-joined',
        },
        actions: [],
      }
    }

    if (decision === 'declined') {
      return {
        status: {
          statusKey: 'declined',
          labelKey: 'notification.actions.declined',
          testId: 'invite-status-declined',
        },
        actions: [],
      }
    }

    return {
      actions: [
        {
          id: 'accept',
          labelKey: 'notification.actions.accept',
          variant: 'primary',
          testId: 'accept-invite-btn',
          execute: async () => {
            if (handlers?.onAcceptInvite && inviteId) {
              return await handlers.onAcceptInvite(inviteId, notification.id)
            }
            return false
          },
        },
        {
          id: 'decline',
          labelKey: 'notification.actions.decline',
          variant: 'outline',
          testId: 'decline-invite-btn',
          execute: async () => {
            if (handlers?.onDeclineInvite && inviteId) {
              return await handlers.onDeclineInvite(inviteId, notification.id)
            }
            return false
          },
        },
      ],
    }
  }
}

/** Explicit no-action resolver for informational notices. */
function createNoActionResolver(): NotificationActionResolver {
  return () => null
}

// Exhaustive built-ins: every backend NotificationType maps to exactly one
// resolver. Invite-family types share the invite resolver; inviter-side
// joined/declined notices, studio handover, and billing notices explicitly
// resolve to no actions (handover intentionally reads transfer_invite_id,
// never invite_id).
registerNotificationActionResolver('workspace_invite', createInviteResolver())
registerNotificationActionResolver('organization_invite', createInviteResolver())
registerNotificationActionResolver('workspace_member_joined', createNoActionResolver())
registerNotificationActionResolver('workspace_invite_declined', createNoActionResolver())
registerNotificationActionResolver('organization_member_joined', createNoActionResolver())
registerNotificationActionResolver('organization_invite_declined', createNoActionResolver())
registerNotificationActionResolver('studio_handover', createNoActionResolver())
registerNotificationActionResolver('billing_credit_low', createNoActionResolver())
registerNotificationActionResolver('billing_recharge_success', createNoActionResolver())

/**
 * Resolves actions and status pills for a given notification.
 */
export function resolveNotificationActions(
  notification: NotificationDTO,
  handlers?: {
    onAcceptInvite?: ((inviteId: string, notificationId: string) => Promise<boolean>) | undefined
    onDeclineInvite?: ((inviteId: string, notificationId: string) => Promise<boolean>) | undefined
  },
): NotificationActionResolution | null {
  const resolver = resolvers.get(notification.type)
  if (!resolver) return null
  return resolver(notification, handlers)
}
