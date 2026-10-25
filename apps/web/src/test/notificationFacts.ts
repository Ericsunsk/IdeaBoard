import type { NotificationDTO } from '@/api/generated-query/public/model'
import { http, HttpResponse, server } from './msw'

/** A REST test adapter: UI fixtures cross the same interface as production facts. */
export function seedNotificationFacts(items: readonly NotificationDTO[], count?: number): void {
  server.use(
    http.get('*/api/v1/user/notifications', ({ request }) => {
      const url = new URL(request.url)
      const status = url.searchParams.get('status')
      const category = url.searchParams.get('category')
      const filtered = items.filter((item) => {
        if (status ? item.status !== status : item.status === 'archived') return false
        if (category === 'billing') return item.type.startsWith('billing_')
        if (category === 'invites') return !item.type.startsWith('billing_')
        return true
      })
      const offset = Number(url.searchParams.get('offset') ?? 0)
      const limit = Number(url.searchParams.get('limit') ?? 20)
      return HttpResponse.json({
        data: {
          items: filtered.slice(offset, offset + limit),
          total: filtered.length,
          offset,
          limit,
        },
      })
    }),
    http.get('*/api/v1/user/notifications/unread-count', () =>
      HttpResponse.json({
        data: { count: count ?? items.filter((item) => item.status === 'unread').length },
      }),
    ),
  )
}
