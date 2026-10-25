import type {
  ListUserNotificationsParams,
  NotificationDTO,
} from '@/api/generated-query/public/model'

export type NotificationCenterTab = 'all' | 'unread' | 'invites' | 'billing' | 'archived'

const NOTIFICATION_INVITE_TYPES = [
  'workspace_invite',
  'workspace_member_joined',
  'workspace_invite_declined',
  'organization_invite',
  'organization_member_joined',
  'organization_invite_declined',
  'studio_handover',
] as const

const NOTIFICATION_BILLING_TYPES = ['billing_credit_low', 'billing_recharge_success'] as const

export function notificationFeedParams(tab: NotificationCenterTab): ListUserNotificationsParams {
  switch (tab) {
    case 'unread':
      return { status: 'unread' }
    case 'archived':
      return { status: 'archived' }
    case 'invites':
      return { category: 'invites' }
    case 'billing':
      return { category: 'billing' }
    default:
      return {}
  }
}

/** Keep optimistic transitions within the same feed contract as server facts. */
export function filterNotificationFeed(
  items: readonly NotificationDTO[],
  tab: NotificationCenterTab,
): NotificationDTO[] {
  return items.filter((item) => {
    if (tab === 'archived') return item.status === 'archived'
    if (item.status === 'archived') return false
    if (tab === 'unread') return item.status === 'unread'
    if (tab === 'invites')
      return (NOTIFICATION_INVITE_TYPES as readonly string[]).includes(item.type)
    if (tab === 'billing')
      return (NOTIFICATION_BILLING_TYPES as readonly string[]).includes(item.type)
    return true
  })
}
