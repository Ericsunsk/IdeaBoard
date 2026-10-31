import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, reactive, ref } from 'vue'
import { QueryClient, VueQueryPlugin, focusManager } from '@tanstack/vue-query'
import { render, waitFor, fireEvent, screen, within } from '@/test/render'
import { mockAppToast } from '@/test/toast'
import { http, HttpResponse, server } from '@/test/msw'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import {
  NOTIFICATION_EVENT_TYPES,
  resetWorkspaceEventStreamsForTests,
} from '@/api/realtime/workspaceEventStream'
import { useNotifications } from './useNotifications'
import type { NotificationCenterTab } from './readFilters'
import NotificationPopover from '../ui/NotificationPopover.vue'

const auth = reactive({ user: { public_id: 'usr_a' } as { public_id: string } | null })
vi.mock('@/features/auth', () => ({ useUserStore: () => auth }))
vi.mock('@/core/composables/useAppToast')
const item = (id: string, overrides: Partial<NotificationDTO> = {}): NotificationDTO => ({
  id,
  type: 'workspace_invite',
  title: id,
  body: `Body ${id}`,
  status: 'unread',
  action_payload: { invite_id: `wsi_${id}`, invite_status: 'pending' },
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  ...overrides,
})
let items: NotificationDTO[]
let requests: URL[]
let session: ReturnType<typeof useNotifications>
let client: QueryClient
const isVisible = ref(true)
const tab = ref<NotificationCenterTab>('all')
function mountSession(staleTime = 0) {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime }, mutations: { retry: false } },
  })
  const Harness = defineComponent({
    setup() {
      session = useNotifications({ tab, isVisible })
      return () => h('div', session.notifications.value.map((n) => n.id).join(','))
    },
  })
  return render(Harness, { global: { plugins: [[VueQueryPlugin, { queryClient: client }]] } })
}
beforeEach(() => {
  mockAppToast()
  auth.user = { public_id: 'usr_a' }
  isVisible.value = true
  tab.value = 'all'
  items = [item('notif_first')]
  requests = []
  server.use(
    http.get('*/api/v1/user/notifications', ({ request }) => {
      const url = new URL(request.url)
      requests.push(url)
      const status = url.searchParams.get('status')
      const category = url.searchParams.get('category')
      const filtered = items.filter((n) => {
        if (status ? n.status !== status : n.status === 'archived') return false
        if (category === 'billing') return n.type.startsWith('billing_')
        if (category === 'invites') return n.type === 'workspace_invite'
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
      HttpResponse.json({ data: { count: items.filter((n) => n.status === 'unread').length } }),
    ),
  )
})
afterEach(() => {
  client?.clear()
  vi.useRealTimers()
})
describe('notification session facts and mutations', () => {
  it.each([true, false])(
    'resumes fresh polling after a hidden response (panel visible: %s)',
    async (isPanelVisible) => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
      const visibilityDescriptor = Object.getOwnPropertyDescriptor(document, 'visibilityState')
      function setVisibility(state: 'visible' | 'hidden') {
        Object.defineProperty(document, 'visibilityState', { configurable: true, value: state })
        document.dispatchEvent(new Event('visibilitychange'))
        focusManager.setFocused(state === 'visible')
      }
      let finishFirst!: () => void
      const firstResponse = new Promise<void>((resolve) => {
        finishFirst = resolve
      })
      let listRequests = 0
      let countRequests = 0
      server.use(
        http.get('*/api/v1/user/notifications', async () => {
          listRequests++
          if (listRequests === 1) await firstResponse
          return HttpResponse.json({
            data: { items: [item(`notif_poll_${listRequests}`)], total: 1, offset: 0, limit: 20 },
          })
        }),
        http.get('*/api/v1/user/notifications/unread-count', async () => {
          countRequests++
          if (countRequests === 1) await firstResponse
          return HttpResponse.json({ data: { count: countRequests } })
        }),
      )
      setVisibility('visible')
      isVisible.value = isPanelVisible
      const view = mountSession(5_000)
      try {
        await vi.waitFor(() => expect(countRequests).toBe(1))
        if (isPanelVisible) await vi.waitFor(() => expect(listRequests).toBe(1))
        setVisibility('hidden')
        finishFirst()
        await vi.waitFor(() => expect(session.unreadCount.value).toBe(1))
        if (isPanelVisible)
          await vi.waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_poll_1'))
        await vi.advanceTimersByTimeAsync(60_000)
        expect(countRequests).toBe(1)
        expect(listRequests).toBe(isPanelVisible ? 1 : 0)

        // Only interval timers advance; the responses remain inside the real 5s fresh window.
        setVisibility('visible')
        await vi.advanceTimersByTimeAsync(30_000)
        await vi.waitFor(() => expect(session.unreadCount.value).toBe(2))
        expect(listRequests).toBe(isPanelVisible ? 2 : 0)

        isVisible.value = false
        await vi.waitFor(() => expect(session.isFetching.value).toBe(false))
        await vi.advanceTimersByTimeAsync(30_000)
        await vi.waitFor(() => expect(session.unreadCount.value).toBe(3))
        expect(listRequests).toBe(isPanelVisible ? 2 : 0)
        auth.user = null
        await vi.waitFor(() => expect(session.notifications.value).toEqual([]))
        await vi.advanceTimersByTimeAsync(60_000)
        expect(countRequests).toBe(3)
      } finally {
        finishFirst()
        view.unmount()
        focusManager.setFocused(undefined)
        if (visibilityDescriptor)
          Object.defineProperty(document, 'visibilityState', visibilityDescriptor)
        else Reflect.deleteProperty(document, 'visibilityState')
      }
    },
  )
  it('refreshes on reopen with unchanged unread count', async () => {
    mountSession()
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_first'))
    isVisible.value = false
    await waitFor(() => expect(session.isFetching.value).toBe(false))
    items = [item('notif_new')]
    isVisible.value = true
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_new'))
    expect(session.unreadCount.value).toBe(1)
    expect(requests.length).toBeGreaterThan(1)
  })
  it('polls the visible list even when unread count stays constant', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    mountSession()
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_first'))
    items = [item('notif_poll')]
    await vi.advanceTimersByTimeAsync(30_000)
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_poll'))
    expect(session.unreadCount.value).toBe(1)
  })
  it('lets the center load additional server pages without a preview-props ceiling', async () => {
    items = Array.from({ length: 25 }, (_, i) => item(`notif_${i}`))
    items.push(item('notif_old_bill', { type: 'billing_credit_low' }))
    client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    render(NotificationPopover, {
      props: { isOpen: true },
      global: { plugins: [[VueQueryPlugin, { queryClient: client }]] },
    })
    await fireEvent.click(await screen.findByTestId('expand-notifications-btn'))
    expect(await screen.findByTestId('notification-load-more')).toBeInTheDocument()
    await fireEvent.click(screen.getByTestId('notification-load-more'))
    expect(
      await within(screen.getByTestId('notification-center-dialog')).findByText('notif_old_bill'),
    ).toBeInTheDocument()
    await fireEvent.click(screen.getByTestId('tab-billing'))
    await waitFor(() => expect(requests.at(-1)?.searchParams.get('category')).toBe('billing'))
    expect(
      await within(screen.getByTestId('notification-center-dialog')).findByText('notif_old_bill'),
    ).toBeInTheDocument()
  })
  it('paginates bounded pages and filters before pagination', async () => {
    items = Array.from({ length: 25 }, (_, i) => item(`notif_${i}`))
    items.push(item('notif_bill', { type: 'billing_credit_low' }))
    mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(20))
    expect(session.hasNextPage.value).toBe(true)
    await session.fetchNextPage()
    expect(session.notifications.value).toHaveLength(26)
    expect(session.hasNextPage.value).toBe(false)
    expect(requests.map((url) => url.searchParams.get('offset'))).toEqual(['0', '20'])
    tab.value = 'billing'
    await waitFor(() =>
      expect(session.notifications.value.map((n) => n.id)).toEqual(['notif_bill']),
    )
    await waitFor(() => expect(requests.at(-1)?.searchParams.get('category')).toBe('billing'))
  })
  it('rolls back failed archive status', async () => {
    server.use(
      http.post('*/api/v1/user/notifications/:id/archive', () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )
    mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(1))
    await expect(session.archive('notif_first')).rejects.toThrow()
    expect(session.notifications.value[0]?.status).toBe('unread')
  })
  it('restores unread view state when mark-all fails', async () => {
    server.use(
      http.post('*/api/v1/user/notifications/read-all', () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )
    mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(1))
    await expect(session.markAllAsRead()).rejects.toThrow()
    expect(session.notifications.value[0]?.status).toBe('unread')
  })
  it('rolls back only the failed bulk archive item', async () => {
    items = [item('notif_ok', { status: 'read' }), item('notif_fail', { status: 'read' })]
    server.use(
      http.post('*/api/v1/user/notifications/:id/archive', ({ params }) => {
        if (params.id === 'notif_fail') return HttpResponse.json({}, { status: 500 })
        const n = items.find((n) => n.id === params.id)!
        n.status = 'archived'
        return HttpResponse.json({ data: n })
      }),
    )
    mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(2))
    expect(await session.archiveRead()).toEqual({
      archivedIds: ['notif_ok'],
      failedIds: ['notif_fail'],
    })
    expect(session.notifications.value.map((n) => [n.id, n.status])).toEqual([
      ['notif_fail', 'read'],
    ])
  })
  it.each(['accept', 'decline'] as const)(
    'preserves %s success and cache sync when marking read fails',
    async (decision) => {
      server.use(
        http.post(`*/api/v1/user/invites/:id/${decision}`, () => HttpResponse.json({ data: {} })),
        http.post('*/api/v1/user/notifications/:id/read', () =>
          HttpResponse.json({}, { status: 500 }),
        ),
      )
      mountSession()
      await waitFor(() => expect(session.notifications.value).toHaveLength(1))
      const invalidate = vi.spyOn(client, 'invalidateQueries')
      const isSuccessful =
        decision === 'accept'
          ? await session.acceptInvite('wsi_first', 'notif_first')
          : await session.declineInvite('wsi_first', 'notif_first')
      expect(isSuccessful).toBe(true)
      expect(invalidate).toHaveBeenCalledWith({ queryKey: ['user', 'invites'] })
      if (decision === 'accept')
        expect(invalidate).toHaveBeenCalledWith(
          expect.objectContaining({ queryKey: ['workspaces'] }),
        )
      expect(session.notifications.value[0]?.action_payload?.invite_status).toBe(
        decision === 'accept' ? 'accepted' : 'declined',
      )
      expect(session.notifications.value[0]?.status).toBe('unread')
    },
  )
  it('leaves failed invitation decisions actionable', async () => {
    server.use(
      http.post('*/api/v1/user/invites/:id/accept', () => HttpResponse.json({}, { status: 409 })),
    )
    mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(1))
    expect(await session.acceptInvite('wsi_first', 'notif_first')).toBe(false)
    expect(session.notifications.value[0]?.status).toBe('unread')
  })
  it('isolates pages and clears state across identities and logout', async () => {
    items = Array.from({ length: 21 }, (_, i) => item(`notif_${i}`))
    mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(20))
    await session.fetchNextPage()
    server.use(
      http.post('*/api/v1/user/invites/:id/accept', () => HttpResponse.json({ data: {} })),
      http.post('*/api/v1/user/notifications/:id/read', () =>
        HttpResponse.json({}, { status: 500 }),
      ),
    )
    await session.acceptInvite('wsi_notif_0', 'notif_0')
    expect(session.notifications.value[0]?.action_payload?.invite_status).toBe('accepted')
    items = [item('notif_other')]
    auth.user = { public_id: 'usr_b' }
    await waitFor(() =>
      expect(session.notifications.value.map((n) => n.id)).toEqual(['notif_other']),
    )
    expect(requests.at(-1)?.searchParams.get('offset')).toBe('0')
    auth.user = null
    await waitFor(() => expect(session.notifications.value).toEqual([]))
  })
  it('adjusts a global count only by known transitions, never by page length', async () => {
    let finish: (() => void) | undefined
    server.use(
      http.post('*/api/v1/user/notifications/:id/read', async () => {
        await new Promise<void>((resolve) => {
          finish = resolve
        })
        return HttpResponse.json({ data: { ...items[0], status: 'read' } })
      }),
    )
    server.use(
      http.get('*/api/v1/user/notifications/unread-count', () =>
        HttpResponse.json({ data: { count: 200 } }),
      ),
    )
    mountSession()
    await waitFor(() => expect(session.unreadCount.value).toBe(200))
    const pending = session.markAsRead('notif_first')
    await waitFor(() => expect(finish).toBeDefined())
    expect(session.unreadCount.value).toBe(199)
    finish!()
    await pending
  })
})

/**
 * Realtime push: hint frames and gap notices travel the recipient user
 * SSE transport and converge on the same invalidate path the 30s poll uses.
 * Frames are only a "refetch now" prompt: the suite asserts new server rows
 * appear after a refetch, never that payloads write the cache.
 */
describe('notification realtime push', () => {
  class MockEventSource {
    static instances: MockEventSource[] = []

    readonly url: string
    private listeners: Record<string, EventListener[]> = {}

    constructor(url: string) {
      this.url = url
      MockEventSource.instances.push(this)
    }

    addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
      const eventListener: EventListener =
        typeof listener === 'function' ? listener : listener.handleEvent.bind(listener)
      this.listeners[type] = [...(this.listeners[type] ?? []), eventListener]
    }

    removeEventListener() {}

    close = vi.fn()

    emit(type: string, data: unknown) {
      const event = new MessageEvent(type, {
        data: typeof data === 'string' ? data : JSON.stringify(data),
      })
      ;(this.listeners[type] ?? []).forEach((listener) => listener(event))
    }
  }

  function hintFrame(overrides: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      id: 'evt_push111',
      type: 'notification.created',
      version: 1,
      occurred_at: new Date().toISOString(),
      actor: { type: 'user', id: 'usr_a' },
      resource: { type: 'invite', id: 'wsi_push123' },
      workspace_id: 'ws_push123',
      data: { invite_id: 'wsi_push123', notification_type: 'workspace_invite' },
      ...overrides,
    }
  }

  function userStream() {
    const found = MockEventSource.instances.find((instance) =>
      instance.url.includes('/api/v1/user/notifications/events'),
    )
    if (!found) throw new Error('user notification stream was not opened')
    return found
  }

  beforeEach(() => {
    vi.stubGlobal('EventSource', MockEventSource)
  })

  afterEach(() => {
    resetWorkspaceEventStreamsForTests()
    vi.unstubAllGlobals()
    MockEventSource.instances = []
  })

  it('refetches the inbox when a hint frame arrives on the user stream', async () => {
    mountSession()
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_first'))
    const fetched = requests.length
    items = [item('notif_push')]
    userStream().emit('notification.created', hintFrame())
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_push'))
    expect(requests.length).toBeGreaterThan(fetched)
    expect(session.unreadCount.value).toBe(1)
  })

  it('refetches on gap notices with the same read path', async () => {
    mountSession()
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_first'))
    const fetched = requests.length
    items = [item('notif_gap')]
    userStream().emit('gap', {})
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_gap'))
    expect(requests.length).toBeGreaterThan(fetched)
  })

  it('uses only the user stream for pending invites and historical workspace facts', async () => {
    items = [
      item('notif_pending', {
        action_payload: {
          invite_id: 'wsi_pending',
          invite_status: 'pending',
          workspace_id: 'ws_notjoined',
        },
      }),
      item('notif_history', {
        action_payload: {
          invite_id: 'wsi_history',
          invite_status: 'accepted',
          workspace_id: 'ws_history',
        },
      }),
    ]
    mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(2))
    expect(MockEventSource.instances).toHaveLength(1)
    const source = userStream()
    const streamUrl = new URL(source.url, 'https://example.test')
    expect(streamUrl.searchParams.get('types')).toBe([...NOTIFICATION_EVENT_TYPES].sort().join(','))
    isVisible.value = false
    items = [...items, item('notif_hidden')]
    const fetched = requests.length
    source.emit('notification.updated', hintFrame({ type: 'notification.updated' }))
    await waitFor(() => expect(session.unreadCount.value).toBe(3))
    expect(source.close).not.toHaveBeenCalled()
    isVisible.value = true
    items = [item('notif_new_workspace', { action_payload: { workspace_id: 'ws_new' } })]
    source.emit('notification.created', hintFrame())
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_new_workspace'))
    expect(requests.length).toBeGreaterThan(fetched)
    expect(MockEventSource.instances).toHaveLength(1)
  })

  it('releases the old stream on identity changes, logout and scope disposal', async () => {
    const view = mountSession()
    await waitFor(() => expect(session.notifications.value).toHaveLength(1))
    const first = userStream()
    auth.user = { public_id: 'usr_b' }
    await waitFor(() => expect(first.close).toHaveBeenCalledOnce())
    expect(MockEventSource.instances).toHaveLength(2)
    const second = MockEventSource.instances[1]!
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    first.emit('notification.created', hintFrame())
    expect(invalidate).not.toHaveBeenCalled()
    auth.user = null
    await waitFor(() => expect(second.close).toHaveBeenCalledOnce())
    expect(MockEventSource.instances).toHaveLength(2)
    auth.user = { public_id: 'usr_c' }
    await waitFor(() => expect(MockEventSource.instances).toHaveLength(3))
    const third = MockEventSource.instances[2]!
    view.unmount()
    expect(third.close).toHaveBeenCalledOnce()
  })

  it('drops malformed frames without refetching', async () => {
    mountSession()
    await waitFor(() => expect(session.notifications.value[0]?.id).toBe('notif_first'))
    const fetched = requests.length
    userStream().emit('notification.created', 'not-json{')
    await new Promise((resolve) => setTimeout(resolve, 300))
    expect(requests.length).toBe(fetched)
    expect(session.notifications.value[0]?.id).toBe('notif_first')
  })
})
