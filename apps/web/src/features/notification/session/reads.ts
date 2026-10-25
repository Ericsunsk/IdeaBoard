import { computed, type Ref } from 'vue'
import { useInfiniteQuery, type QueryClient } from '@tanstack/vue-query'
import {
  getGetUserNotificationsUnreadCountQueryKey,
  getListUserNotificationsQueryKey,
  listUserNotifications,
  useGetUserNotificationsUnreadCount,
} from '@/api/generated-query/public/notifications/notifications'
import type { ListUserNotificationsParams } from '@/api/generated-query/public/model'

const NOTIFICATION_POLL_INTERVAL_MS = 30_000

/** Identity-scoped, bounded REST facts shared by the bell and open views. */
export function useNotificationReads(options: {
  queryClient: QueryClient
  identity: Ref<string | null>
  isEnabled: Ref<boolean>
  isVisible: Ref<boolean>
  params: Ref<ListUserNotificationsParams>
}) {
  const { queryClient, identity, isEnabled, isVisible, params } = options
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
      refetchInterval: computed(() => (isVisible.value ? NOTIFICATION_POLL_INTERVAL_MS : false)),
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: true,
    },
    queryClient,
  )
  const unreadCountQuery = useGetUserNotificationsUnreadCount(
    {
      query: {
        queryKey: computed(() => [...getGetUserNotificationsUnreadCountQueryKey(), identity.value]),
        refetchInterval: NOTIFICATION_POLL_INTERVAL_MS,
        refetchIntervalInBackground: false,
        refetchOnWindowFocus: true,
        enabled: isEnabled,
      },
    },
    queryClient,
  )
  return { notificationsQuery, unreadCountQuery }
}
