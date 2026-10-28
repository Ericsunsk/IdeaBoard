import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RenderOptions } from '@testing-library/vue'
import { seedNotificationFacts } from '@/test/notificationFacts'
import { createDeferred } from '@/test/deferred'
import { fireEvent, render, screen } from '@/test/render'
import { http, HttpResponse, server } from '@/test/msw'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import NotificationPopover from './NotificationPopover.vue'

vi.mock('@/features/auth', () => ({
  useUserStore: () => ({ user: { public_id: 'usr_notifications' } }),
}))

let client: QueryClient
function renderView(options: RenderOptions<typeof NotificationPopover>) {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(NotificationPopover, {
    ...options,
    global: { ...options.global, plugins: [[VueQueryPlugin, { queryClient: client }]] },
  })
}
afterEach(() => client?.clear())

const sampleNotifications: NotificationDTO[] = [
  {
    id: 'notif_invite',
    type: 'workspace_invite',
    title: 'Workspace Invitation',
    body: 'You have been invited to join Team Alpha',
    action_payload: { invite_id: 'inv_alpha_1', workspace_id: 'ws_alpha' },
    status: 'unread',
    read_at: null,
    created_at: new Date(Date.now() - 10 * 60 * 1000).toISOString(),
    updated_at: new Date().toISOString(),
  },
  {
    id: 'notif_handover',
    type: 'studio_handover',
    title: 'Studio Handover',
    body: 'Studio ownership transferred',
    action_payload: {},
    status: 'read',
    read_at: new Date().toISOString(),
    created_at: new Date(Date.now() - 3 * 3600 * 1000).toISOString(),
    updated_at: new Date().toISOString(),
  },
  {
    id: 'notif_billing',
    type: 'billing_credit_low',
    title: 'Low Balance',
    body: 'Credits running low',
    action_payload: {},
    status: 'unread',
    read_at: null,
    created_at: new Date(Date.now() - 24 * 3600 * 1000).toISOString(),
    updated_at: new Date().toISOString(),
  },
]

describe('NotificationPopover', () => {
  it('renders header with title', async () => {
    seedNotificationFacts([])
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByText('Notifications')).toBeInTheDocument()
    expect(screen.queryByTestId('mark-all-read-btn')).toBeNull()
  })

  it('renders notifications list with type icons, titles, bodies, and unread dots', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(3)

    expect(screen.getByText('Workspace Invitation')).toBeInTheDocument()
    expect(screen.getByText('You have been invited to join Team Alpha')).toBeInTheDocument()
    expect(screen.getByText('Studio Handover')).toBeInTheDocument()
    expect(screen.getByText('Low Balance')).toBeInTheDocument()

    // 2 unread items have unread dots
    const unreadDots = screen.getAllByTestId('notification-unread-dot')
    expect(unreadDots).toHaveLength(2)

    // workspace_invite notification uses the primary outline token styling without circular fill container
    const icons = screen.getAllByTestId('notification-item-icon')
    expect(icons[0]!.querySelector('svg')?.getAttribute('class')).toContain('text-primary')
    expect(icons[0]!.getAttribute('class') ?? '').not.toContain('rounded-pill')
  })

  it('lists all notifications unified with unread items prioritized at the top', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(3)

    // Unread items appear first, ordered by time
    expect(items[0]).toHaveTextContent('Workspace Invitation')
    expect(items[1]).toHaveTextContent('Low Balance')
    // Read item appears after unread items
    expect(items[2]).toHaveTextContent('Studio Handover')

    // Unread dots are rendered on the unread items
    const unreadDots = screen.getAllByTestId('notification-unread-dot')
    expect(unreadDots).toHaveLength(2)
  })

  it('renders expand button in popover header without number count', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(screen.queryByTestId('popover-unread-count')).toBeNull()
    const expandBtn = await screen.findByTestId('expand-notifications-btn')
    expect(expandBtn).toBeInTheDocument()
    expect(expandBtn).toHaveAttribute('aria-label', 'View all notifications')
  })

  it('renders empty state when there are no notifications at all', async () => {
    seedNotificationFacts([])
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByTestId('notification-empty')).toHaveTextContent('No notifications')
  })

  it('renders inline Accept and Decline buttons for workspace_invite notifications', async () => {
    seedNotificationFacts([sampleNotifications[0]!])
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByTestId('accept-invite-btn')).toBeInTheDocument()
    expect(screen.getByTestId('decline-invite-btn')).toBeInTheDocument()
  })

  it('renders Joined and Declined pills when invite was already accepted or declined', async () => {
    const resolvedNotifications: NotificationDTO[] = [
      {
        id: 'notif_accepted',
        type: 'workspace_invite',
        title: 'Workspace Invitation',
        body: 'You joined Design Team',
        action_payload: { invite_id: 'inv_1', invite_status: 'accepted' },
        status: 'read',
        read_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      {
        id: 'notif_declined',
        type: 'workspace_invite',
        title: 'Workspace Invitation',
        body: 'You declined Product Team',
        action_payload: { invite_id: 'inv_2', invite_status: 'declined' },
        status: 'read',
        read_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    ]

    seedNotificationFacts(resolvedNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByTestId('invite-status-joined')).toHaveTextContent('Joined')
    expect(screen.getByTestId('invite-status-declined')).toHaveTextContent('Declined')
    expect(screen.queryByTestId('accept-invite-btn')).toBeNull()
    expect(screen.queryByTestId('decline-invite-btn')).toBeNull()
  })

  it('renders expand button in header, has no footer button, and opens NotificationCenterDialog on click', async () => {
    seedNotificationFacts([sampleNotifications[0]!])
    renderView({
      props: {
        isOpen: true,
      },
    })

    // Footer button is removed
    expect(screen.queryByTestId('view-all-notifications-btn')).toBeNull()

    const expandBtn = await screen.findByTestId('expand-notifications-btn')
    expect(expandBtn).toBeInTheDocument()

    // Dialog is not open yet
    expect(screen.queryByTestId('notification-center-dialog')).toBeNull()

    // Click expand button
    await fireEvent.click(expandBtn)

    // Dialog opens
    expect(await screen.findByTestId('notification-center-dialog')).toBeInTheDocument()
  })

  it('renders at most 5 items in the preview', async () => {
    const manyNotifications: NotificationDTO[] = Array.from({ length: 7 }, (_, i) => ({
      id: `notif_${i + 1}`,
      type: 'workspace_invite',
      title: `Notification ${i + 1}`,
      body: `Body ${i + 1}`,
      action_payload: {},
      status: 'unread',
      read_at: null,
      created_at: new Date(Date.now() - i * 60 * 1000).toISOString(),
      updated_at: new Date().toISOString(),
    }))

    seedNotificationFacts(manyNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(5)
    expect(screen.getByText('Notification 1')).toBeInTheDocument()
    expect(screen.getByText('Notification 5')).toBeInTheDocument()
    expect(screen.queryByText('Notification 6')).toBeNull()
    expect(screen.queryByText('Notification 7')).toBeNull()
  })

  it('bubbles the next unread notification into the top 5 when an unread item is marked as read', async () => {
    server.use(
      http.post('*/api/v1/user/notifications/:id/read', ({ params }) => {
        return HttpResponse.json({
          success: true,
          data: {
            id: String(params.id),
            type: 'studio_handover',
            title: 'Handover 1',
            body: 'Body 1',
            action_payload: {},
            status: 'read',
            read_at: new Date().toISOString(),
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
        })
      }),
    )

    const sixNotifications: NotificationDTO[] = Array.from({ length: 6 }, (_, i) => ({
      id: `bubble_notif_${i + 1}`,
      type: 'studio_handover',
      title: `Handover ${i + 1}`,
      body: `Body ${i + 1}`,
      action_payload: {},
      status: 'unread',
      read_at: null,
      created_at: new Date(Date.now() - i * 60 * 1000).toISOString(),
      updated_at: new Date().toISOString(),
    }))

    seedNotificationFacts(sixNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    // Initially top 5: Handover 1 to 5. Handover 6 is hidden.
    expect(await screen.findByText('Handover 1')).toBeInTheDocument()
    expect(screen.getByText('Handover 5')).toBeInTheDocument()
    expect(screen.queryByText('Handover 6')).toBeNull()

    const items = screen.getAllByTestId('notification-item')
    // Click item 1 to mark it as read
    await fireEvent.click(items[0]!)

    // Handover 6 has now bubbled up into the visible 5 items
    expect(await screen.findByText('Handover 6')).toBeInTheDocument()
    // The top 5 items are Handover 2, 3, 4, 5, 6
    const updatedItems = screen.getAllByTestId('notification-item')
    expect(updatedItems).toHaveLength(5)
    expect(updatedItems[0]).toHaveTextContent('Handover 2')
    expect(updatedItems[4]).toHaveTextContent('Handover 6')
    expect(screen.queryByText('Handover 1')).toBeNull()
  })

  describe('loading and error states', () => {
    it('renders skeleton rows that preserve row height while loading', async () => {
      seedNotificationFacts([])
      const response = createDeferred<Response>()
      server.use(http.get('*/api/v1/user/notifications', () => response.promise))
      renderView({ props: { isOpen: true } })

      const skeleton = await screen.findByTestId('notification-skeleton')
      expect(skeleton).toHaveAttribute('role', 'status')
      expect(screen.queryByTestId('notification-empty')).toBeNull()
      expect(screen.queryByTestId('notification-item')).toBeNull()
      response.resolve(HttpResponse.json({ data: { items: [], total: 0, limit: 20, offset: 0 } }))
      expect(await screen.findByTestId('notification-empty')).toBeInTheDocument()
    })

    it('retries the notification facts after a failed request', async () => {
      seedNotificationFacts([])
      let requests = 0
      server.use(
        http.get('*/api/v1/user/notifications', () => {
          requests += 1
          return requests === 1
            ? HttpResponse.json({}, { status: 500 })
            : HttpResponse.json({ data: { items: [], total: 0, limit: 20, offset: 0 } })
        }),
      )
      renderView({ props: { isOpen: true } })

      expect(await screen.findByTestId('notification-list-error')).toHaveTextContent(
        'Could not load notifications',
      )
      expect(screen.queryByTestId('notification-empty')).toBeNull()
      await fireEvent.click(screen.getByTestId('notification-retry-btn'))
      expect(await screen.findByTestId('notification-empty')).toBeInTheDocument()
      expect(requests).toBe(2)
    })
  })
})
