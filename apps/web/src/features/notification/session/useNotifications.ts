import { computed, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { useQueryClient, type QueryClient } from '@tanstack/vue-query'
import {
  getGetUserNotificationsUnreadCountQueryKey,
  getListUserNotificationsQueryKey,
  useArchiveUserNotification,
  useMarkAllUserNotificationsRead,
  useMarkUserNotificationRead,
} from '@/api/generated-query/public/notifications/notifications'
import {
  acceptUserOrganizationInvite,
  acceptUserWorkspaceInvite,
  declineUserOrganizationInvite,
  declineUserWorkspaceInvite,
  getListUserOrganizationInvitesQueryKey,
  getListUserWorkspaceInvitesQueryKey,
} from '@/api/generated-query/public/user/user'
import { getListOrganizationsQueryKey } from '@/api/generated-query/public/organizations/organizations'
import { getListWorkspacesQueryKey } from '@/api/generated-query/public/workspaces/workspaces'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import { useAppToast } from '@/core/composables/useAppToast'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '@/features/auth'
import {
  applyNotificationViewOverrides,
  type InviteLocalDecision,
  type NotificationStatusOverride,
  readPayloadInviteDecision,
} from './notificationDisplay'
import {
  filterNotificationFeed,
  notificationFeedParams,
  type NotificationCenterTab,
} from './readFilters'
import { useNotificationReads } from './reads'

export interface UseNotificationsOptions {
  enabled?: MaybeRefOrGetter<boolean>
  tab?: MaybeRefOrGetter<NotificationCenterTab>
  isVisible?: MaybeRefOrGetter<boolean>
}

/**
 * Notification query invalidation sets. Three sets exist and each flow
 * converges on exactly one of them (never a mix):
 *
 * - Notification pair (`invalidateNotificationQueries`): list + unread
 *   count. Authoritative for read/archive paths only.
 * - Invite-acceptance set (`syncInviteAcceptanceQueries`): membership
 *   scopes (workspaces/organizations) + user invite lists + notification
 *   pair. Authoritative for every invite-accept path (session accept flows
 *   and the workspace ClaimPage converge here).
 * - Invite-decline set (`syncInviteDeclineQueries`): user invite lists +
 *   notification pair. Authoritative for decline paths; membership scopes
 *   are untouched by a decline.
 *
 * Each business decision calls its matching sync helper once. The separate
 * read mutation performs its own notification sync and cannot gate membership
 * convergence or change the business decision outcome.
 */
async function invalidateNotificationQueries(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: getListUserNotificationsQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getGetUserNotificationsUnreadCountQueryKey() }),
  ])
}

/**
 * Shared invite-acceptance invalidation: membership scopes, invite lists,
 * and notification queries. Used by session accept flows and the workspace
 * ClaimPage so both converge on the same set.
 */
export async function syncInviteAcceptanceQueries(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: getListWorkspacesQueryKey(), exact: true }),
    queryClient.invalidateQueries({ queryKey: getListOrganizationsQueryKey(), exact: true }),
    queryClient.invalidateQueries({ queryKey: getListUserWorkspaceInvitesQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getListUserOrganizationInvitesQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getListUserNotificationsQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getGetUserNotificationsUnreadCountQueryKey() }),
  ])
}

/**
 * Shared invite-decline invalidation: invite lists and notification
 * queries (membership scopes are untouched by a decline).
 */
export async function syncInviteDeclineQueries(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: getListUserWorkspaceInvitesQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getListUserOrganizationInvitesQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getListUserNotificationsQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getGetUserNotificationsUnreadCountQueryKey() }),
  ])
}

export interface ArchiveReadResult {
  archivedIds: string[]
  failedIds: string[]
}

/** Split allSettled outcomes into per-item archived/failed id lists. */
function splitSettledArchiveIds(
  ids: readonly string[],
  settled: readonly PromiseSettledResult<unknown>[],
): ArchiveReadResult {
  const archivedIds: string[] = []
  const failedIds: string[] = []
  for (const [index, id] of ids.entries()) {
    if (settled[index]?.status === 'fulfilled') {
      archivedIds.push(id)
    } else {
      failedIds.push(id)
    }
  }
  return { archivedIds, failedIds }
}

/** Loading/error/fetching refs exposed by the generated list + unread-count queries. */
interface ListQueryFlags {
  isLoading: Ref<boolean>
  isError: Ref<boolean>
  isFetching: Ref<boolean>
}

/**
 * Combined flags for the list + unread-count pair. A standalone helper so
 * the session body stays under the function-length budget; both queries
 * drive the skeleton and inline-error states together.
 */
function combineListQueryFlags(
  listQuery: ListQueryFlags,
  countQuery: ListQueryFlags,
): ListQueryFlags {
  const isLoading = computed(() => listQuery.isLoading.value || countQuery.isLoading.value)
  const isError = computed(() => listQuery.isError.value || countQuery.isError.value)
  const isFetching = computed(() => listQuery.isFetching.value || countQuery.isFetching.value)
  return { isLoading, isError, isFetching }
}

export function useNotifications(options: UseNotificationsOptions = {}) {
  const queryClient = useQueryClient()
  const userStore = useUserStore()
  const toast = useAppToast()
  const { t } = useI18n()

  const isEnabled = computed(() => {
    if (options.enabled !== undefined) {
      return toValue(options.enabled)
    }
    return !!userStore.user
  })

  const identity = computed(() => userStore.user?.public_id ?? null)
  const isListVisible = computed(
    () => options.isVisible === undefined || toValue(options.isVisible),
  )
  const listParams = computed(() => notificationFeedParams(toValue(options.tab) ?? 'all'))
  const { notificationsQuery, unreadCountQuery } = useNotificationReads({
    queryClient,
    identity,
    isEnabled,
    isVisible: isListVisible,
    params: listParams,
  })

  const markReadMutation = useMarkUserNotificationRead({}, queryClient)
  const markAllReadMutation = useMarkAllUserNotificationsRead({}, queryClient)
  const archiveMutation = useArchiveUserNotification({}, queryClient)

  const facts = computed<NotificationDTO[]>(
    () => notificationsQuery.data.value?.pages.flatMap((page) => page.data.items) ?? [],
  )

  const { isLoading, isError, isFetching } = combineListQueryFlags(
    notificationsQuery,
    unreadCountQuery,
  )
  const isMarkingRead = computed(() => markReadMutation.isPending.value)
  const isMarkingAllRead = computed(() => markAllReadMutation.isPending.value)
  const isArchiving = computed(() => archiveMutation.isPending.value)

  // Session-owned temporary state is merged into the final projections;
  // callers never prepare facts or apply overrides themselves.
  // statusOverrides holds temporary read/archive views without rewriting cached facts,
  // inviteDecisions records the local accept/decline outcome for the pill.
  const statusOverrides = ref(new Map<string, NotificationStatusOverride>())
  const inviteDecisions = ref(new Map<string, InviteLocalDecision>())

  let operationSequence = 0
  const operationTokens = new Map<string, number>()

  watch(
    identity,
    () => {
      statusOverrides.value.clear()
      inviteDecisions.value.clear()
      operationTokens.clear()
    },
    { flush: 'sync' },
  )

  // A successful server read supersedes temporary view state. Cache data
  // remains authoritative; optimistic patches never replace server rows.
  watch(facts, (items) => {
    for (const item of items) {
      const override = statusOverrides.value.get(item.id)
      if (override?.status === item.status) statusOverrides.value.delete(item.id)
      if (inviteDecisions.value.get(item.id) === readPayloadInviteDecision(item)) {
        inviteDecisions.value.delete(item.id)
      }
    }
  })

  function rememberStatus(id: string, status: NotificationDTO['status'], readAt?: string | null) {
    const token = ++operationSequence
    operationTokens.set(id, token)
    statusOverrides.value.set(id, readAt === undefined ? { status } : { status, readAt })
    return token
  }

  function rollbackStatus(id: string, token: number): void {
    if (operationTokens.get(id) !== token) return
    statusOverrides.value.delete(id)
    operationTokens.delete(id)
  }

  const notifications = computed(() =>
    filterNotificationFeed(
      applyNotificationViewOverrides(facts.value, {
        statuses: statusOverrides.value,
        decisions: inviteDecisions.value,
      }),
      toValue(options.tab) ?? 'all',
    ),
  )

  // A page cannot determine the global count: subtract only known temporary
  // transitions from the authoritative count query.
  const unreadCount = computed(() => {
    const changed = facts.value.filter(
      (item) =>
        item.status === 'unread' &&
        statusOverrides.value.has(item.id) &&
        statusOverrides.value.get(item.id)?.status !== 'unread',
    ).length
    return Math.max(0, (unreadCountQuery.data.value?.data?.count ?? 0) - changed)
  })
  const hasUnread = computed(() => unreadCount.value > 0)

  watch(isListVisible, (isVisible) => {
    if (isVisible) void refetch()
  })

  async function updateStatus(
    id: string,
    override: NotificationStatusOverride,
    request: () => Promise<unknown>,
    errorKey: string,
  ): Promise<void> {
    const currentIdentity = identity.value
    const token = rememberStatus(id, override.status, override.readAt)
    try {
      await request()
    } catch (error) {
      if (identity.value === currentIdentity) {
        rollbackStatus(id, token)
        toast.error(t(errorKey))
      }
      throw error
    } finally {
      void invalidateNotificationQueries(queryClient)
    }
  }

  function markAsRead(id: string): Promise<void> {
    return updateStatus(
      id,
      { status: 'read', readAt: new Date().toISOString() },
      () => markReadMutation.mutateAsync({ id }),
      'notification.toasts.markReadFailed',
    )
  }

  async function markAllAsRead(): Promise<void> {
    const currentIdentity = identity.value
    const tokens = new Map<string, number>()
    const now = new Date().toISOString()
    for (const item of notifications.value) {
      if (item.status === 'unread')
        tokens.set(item.id, rememberStatus(item.id, 'read', item.read_at ?? now))
    }
    try {
      await markAllReadMutation.mutateAsync()
    } catch (error) {
      if (identity.value === currentIdentity) {
        for (const [id, token] of tokens) rollbackStatus(id, token)
        toast.error(t('notification.toasts.markReadFailed'))
      }
      throw error
    } finally {
      void invalidateNotificationQueries(queryClient)
    }
  }

  function archive(id: string): Promise<void> {
    return updateStatus(
      id,
      { status: 'archived' },
      () => archiveMutation.mutateAsync({ id }),
      'notification.toasts.archiveFailed',
    )
  }

  async function archiveRead(): Promise<ArchiveReadResult> {
    const currentIdentity = identity.value
    const items = notifications.value
    const ids = [...new Set(items.filter((item) => item.status === 'read').map((item) => item.id))]
    const tokens = new Map(ids.map((id) => [id, rememberStatus(id, 'archived')]))
    const settled = await Promise.allSettled(ids.map((id) => archiveMutation.mutateAsync({ id })))
    const result = splitSettledArchiveIds(ids, settled)
    if (identity.value === currentIdentity) {
      for (const id of result.failedIds) rollbackStatus(id, tokens.get(id)!)
      if (result.failedIds.length > 0) toast.error(t('notification.toasts.archiveFailed'))
    }
    void invalidateNotificationQueries(queryClient)
    return result
  }

  async function decideInvite(
    inviteId: string,
    notificationId: string | undefined,
    decision: InviteLocalDecision,
  ): Promise<boolean> {
    const currentIdentity = identity.value
    try {
      if (decision === 'joined') {
        await (
          inviteId.startsWith('osi_') ? acceptUserOrganizationInvite : acceptUserWorkspaceInvite
        )(inviteId)
      } else {
        await (
          inviteId.startsWith('osi_') ? declineUserOrganizationInvite : declineUserWorkspaceInvite
        )(inviteId)
      }
    } catch {
      if (identity.value === currentIdentity)
        toast.error(t('notification.toasts.inviteActionFailed'))
      return false
    }
    if (identity.value !== currentIdentity) return true
    if (notificationId) inviteDecisions.value.set(notificationId, decision)

    // The business decision already succeeded. Notification read status is
    // independent and cannot prevent membership/invite cache convergence.
    const sync = decision === 'joined' ? syncInviteAcceptanceQueries : syncInviteDeclineQueries
    await sync(queryClient)
    if (identity.value !== currentIdentity) return true
    if (notificationId) {
      try {
        await markAsRead(notificationId)
      } catch {
        // markAsRead reports its own failure; the business decision remains successful.
      }
    }
    if (identity.value === currentIdentity) {
      toast.success(
        t(
          decision === 'joined'
            ? 'notification.toasts.inviteAccepted'
            : 'notification.toasts.inviteDeclined',
        ),
      )
    }
    return true
  }

  function acceptInvite(inviteId: string, notificationId?: string): Promise<boolean> {
    return decideInvite(inviteId, notificationId, 'joined')
  }

  function declineInvite(inviteId: string, notificationId?: string): Promise<boolean> {
    return decideInvite(inviteId, notificationId, 'declined')
  }

  async function refetch(): Promise<void> {
    if (!isEnabled.value) return
    await Promise.all([notificationsQuery.refetch(), unreadCountQuery.refetch()])
  }

  return {
    notifications,
    hasNextPage: notificationsQuery.hasNextPage,
    isFetchingNextPage: notificationsQuery.isFetchingNextPage,
    isFetchNextPageError: notificationsQuery.isFetchNextPageError,
    fetchNextPage: () => notificationsQuery.fetchNextPage(),
    unreadCount,
    hasUnread,
    isLoading,
    isError,
    isFetching,
    isMarkingRead,
    isMarkingAllRead,
    isArchiving,
    markAsRead,
    markAllAsRead,
    archive,
    archiveRead,
    acceptInvite,
    declineInvite,
    refetch,
  }
}
