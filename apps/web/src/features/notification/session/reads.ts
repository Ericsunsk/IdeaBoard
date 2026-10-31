import { computed, watch, type Ref } from 'vue'
import { useInfiniteQuery, type QueryClient } from '@tanstack/vue-query'
import { useQueryPolling } from '@/core/query/useQueryPolling'
import {
  getGetUserNotificationsUnreadCountQueryKey,
  getListUserNotificationsQueryKey,
  listUserNotifications,
  useGetUserNotificationsUnreadCount,
} from '@/api/generated-query/public/notifications/notifications'
import {
  NOTIFICATION_EVENT_TYPES,
  subscribeUserNotificationEvents,
} from '@/api/realtime/workspaceEventStream'
import type { ListUserNotificationsParams } from '@/api/generated-query/public/model'

const NOTIFICATION_POLL_INTERVAL_MS = 30_000

/**
 * Notification pair invalidation: list + unread count. Authoritative for
 * read/archive paths and for realtime hints alike: a push frame only says
 * "something changed, GET again", it never writes payloads into the cache.
 * Queries also refetch every 30 seconds while active to close missed-hint gaps.
 */
export async function invalidateNotificationQueries(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: getListUserNotificationsQueryKey() }),
    queryClient.invalidateQueries({ queryKey: getGetUserNotificationsUnreadCountQueryKey() }),
  ])
}

/** Identity-scoped, bounded REST facts shared by the bell and open views. */
export function useNotificationReads(options: {
  queryClient: QueryClient
  identity: Ref<string | null>
  isEnabled: Ref<boolean>
  isVisible: Ref<boolean>
  params: Ref<ListUserNotificationsParams>
}) {
  const { queryClient, identity, isEnabled, isVisible, params } = options
  // Both queries pause in hidden tabs; panel visibility additionally gates the list.
  const polling = useQueryPolling({
    visibleInterval: NOTIFICATION_POLL_INTERVAL_MS,
    isEnabled: () => isEnabled.value,
  })
  const notificationsQuery = useInfiniteQuery(
    {
      queryKey: computed(() => [
        ...getListUserNotificationsQueryKey(),
        identity.value,
        params.value,
      ]),
      enabled: computed(() => isEnabled.value && isVisible.value),
      initialPageParam: 0,
      queryFn: ({ pageParam, signal }) =>
        listUserNotifications(
          {
            ...params.value,
            limit: 20,
            offset: pageParam,
          },
          { signal },
        ),
      getNextPageParam: (lastPage) => {
        const page = lastPage.data
        const nextOffset = page.offset + page.items.length
        return page.items.length > 0 && nextOffset < page.total ? nextOffset : undefined
      },
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: true,
    },
    queryClient,
  )
  const unreadCountQuery = useGetUserNotificationsUnreadCount(
    {
      query: {
        queryKey: computed(() => [...getGetUserNotificationsUnreadCountQueryKey(), identity.value]),
        refetchInterval: polling.refetchInterval,
        refetchIntervalInBackground: false,
        refetchOnWindowFocus: true,
        enabled: isEnabled,
      },
    },
    queryClient,
  )

  // Recipient hints do not require Workspace membership: pending invitees
  // receive the same invalidation through the user stream. REST remains the
  // fact source and polling closes gaps when realtime is unavailable.
  watch(
    [isEnabled, identity],
    ([isPushEnabled, id], _previous, onCleanup) => {
      if (!isPushEnabled || id == null) return
      const invalidate = (): void => {
        void invalidateNotificationQueries(queryClient)
      }
      onCleanup(
        subscribeUserNotificationEvents(invalidate, {
          types: [...NOTIFICATION_EVENT_TYPES],
          onGap: invalidate,
        }),
      )
      invalidate()
    },
    { immediate: true },
  )

  return { notificationsQuery, unreadCountQuery }
}
