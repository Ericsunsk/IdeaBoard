import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RenderOptions } from '@testing-library/vue'
import { seedNotificationFacts } from '@/test/notificationFacts'
import { createDeferred } from '@/test/deferred'
import { fireEvent, render, screen, waitFor, within } from '@/test/render'
import { http, HttpResponse, server } from '@/test/msw'
import { expectToastFeedback, mockAppToast } from '@/test/toast'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import NotificationCenterDialog from './NotificationCenterDialog.vue'

vi.mock('../../../core/composables/useAppToast')

vi.mock('@/features/auth', () => ({
  useUserStore: () => ({ user: { public_id: 'usr_notifications' } }),
}))

let client: QueryClient
function renderView(options: RenderOptions<typeof NotificationCenterDialog>) {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(NotificationCenterDialog, {
    ...options,
    global: { ...options.global, plugins: [[VueQueryPlugin, { queryClient: client }]] },
  })
}
afterEach(() => client?.clear())

const now = new Date()
const todayISO = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 12, 0, 0).toISOString()
const yesterdayISO = new Date(
  now.getFullYear(),
  now.getMonth(),
  now.getDate() - 1,
  12,
  0,
  0,
).toISOString()
const earlierISO = new Date(
  now.getFullYear(),
  now.getMonth(),
  now.getDate() - 4,
  12,
  0,
  0,
).toISOString()

const sampleNotifications: NotificationDTO[] = [
  {
    id: 'notif_invite_today',
    type: 'workspace_invite',
    title: 'Workspace Invitation Today',
    body: 'Join Team Design today',
    action_payload: { invite_id: 'inv_1', workspace_id: 'ws_1' },
    status: 'unread',
    read_at: null,
    created_at: todayISO,
    updated_at: todayISO,
  },
  {
    id: 'notif_billing_today',
    type: 'billing_credit_low',
    title: 'Low Balance Warning',
    body: 'Your credits are almost depleted',
    action_payload: {},
    status: 'unread',
    read_at: null,
    created_at: todayISO,
    updated_at: todayISO,
  },
  {
    id: 'notif_handover_yesterday',
    type: 'studio_handover',
    title: 'Studio Handover Yesterday',
    body: 'Ownership transfer pending',
    action_payload: {},
    status: 'read',
    read_at: yesterdayISO,
    created_at: yesterdayISO,
    updated_at: yesterdayISO,
  },
  {
    id: 'notif_recharge_earlier',
    type: 'billing_recharge_success',
    title: 'Recharge Success Earlier',
    body: '500 credits added to account',
    action_payload: {},
    status: 'read',
    read_at: earlierISO,
    created_at: earlierISO,
    updated_at: earlierISO,
  },
  {
    id: 'notif_archived',
    type: 'workspace_invite',
    title: 'Archived Invite',
    body: 'Past invitation',
    action_payload: {},
    status: 'archived',
    read_at: earlierISO,
    created_at: earlierISO,
    updated_at: earlierISO,
  },
]

describe('NotificationCenterDialog', () => {
  it('renders header with title and unread badge when unreadCount > 0', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Notifications')).toBeInTheDocument()

    const unreadBadge = await screen.findByTestId('dialog-unread-badge')
    expect(unreadBadge).toBeInTheDocument()
    expect(unreadBadge).toHaveTextContent('2')
  })

  it('renders Mark all read and Clear read buttons in header with correct enabled/disabled state', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    const markAllBtn = screen.getByTestId('mark-all-read-btn')
    const clearReadBtn = screen.getByTestId('clear-read-btn')

    expect(markAllBtn).toBeInTheDocument()
    expect(markAllBtn).toHaveTextContent('Mark all as read')
    await waitFor(() => expect(markAllBtn).not.toBeDisabled())

    expect(clearReadBtn).toBeInTheDocument()
    expect(clearReadBtn).toHaveTextContent('Clear read')
    await waitFor(() => expect(clearReadBtn).not.toBeDisabled())
  })

  it('disables Mark all read when unreadCount === 0 and disables Clear read when no read items exist', async () => {
    seedNotificationFacts([])
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByTestId('mark-all-read-btn')).toBeDisabled()
    expect(screen.getByTestId('clear-read-btn')).toBeDisabled()
    expect(screen.queryByTestId('dialog-unread-badge')).toBeNull()
  })

  it('marks all notifications as read when Mark all read is clicked', async () => {
    let isMarkAllCalled = false
    server.use(
      http.post('*/api/v1/user/notifications/read-all', () => {
        isMarkAllCalled = true
        return HttpResponse.json({
          success: true,
          data: { updated_count: 2 },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    const markAllBtn = screen.getByTestId('mark-all-read-btn')
    await waitFor(() => expect(markAllBtn).not.toBeDisabled())
    await fireEvent.click(markAllBtn)

    await waitFor(() => {
      expect(isMarkAllCalled).toBe(true)
    })
    expect(screen.queryByTestId('dialog-unread-badge')).toBeNull()
    expect(markAllBtn).toBeDisabled()
  })

  it('asks for confirmation before archiving read notifications', async () => {
    const archivedIds: string[] = []
    server.use(
      http.post('*/api/v1/user/notifications/:id/archive', ({ params }) => {
        archivedIds.push(String(params.id))
        return HttpResponse.json({
          success: true,
          data: {
            id: String(params.id),
            status: 'archived',
            type: 'studio_handover',
            title: 'Archived',
            body: '',
            action_payload: {},
            read_at: new Date().toISOString(),
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    const clearReadBtn = screen.getByTestId('clear-read-btn')
    await waitFor(() => expect(clearReadBtn).not.toBeDisabled())
    await fireEvent.click(clearReadBtn)

    // The confirmation names the archived count and points at the Archived tab.
    // (The open confirm hides the center dialog from the accessibility tree,
    // so the dialog is located through the hidden query.)
    const confirmTitles = await screen.findAllByText('Archive read notifications?')
    expect(confirmTitles.length).toBeGreaterThanOrEqual(1)
    const stackedDialogs = screen.getAllByRole('dialog', { hidden: true })
    expect(stackedDialogs.length).toBe(2)
    const confirmDialog = stackedDialogs[stackedDialogs.length - 1]!
    expect(confirmDialog.textContent).toContain('Archive read notifications?')
    expect(confirmDialog.textContent).toContain(
      'Archive 2 read notifications? You can find them in Archived.',
    )

    // Nothing archives before the explicit confirmation.
    expect(archivedIds).toHaveLength(0)

    await fireEvent.click(within(confirmDialog).getByRole('button', { name: 'Archive' }))

    await waitFor(() => {
      expect(archivedIds).toContain('notif_handover_yesterday')
      expect(archivedIds).toContain('notif_recharge_earlier')
    })
    expect(clearReadBtn).toBeDisabled()
  })

  it('does not archive when the clear-read confirmation is cancelled', async () => {
    const archivedIds: string[] = []
    server.use(
      http.post('*/api/v1/user/notifications/:id/archive', ({ params }) => {
        archivedIds.push(String(params.id))
        return HttpResponse.json({
          success: true,
          data: {
            id: String(params.id),
            status: 'archived',
            type: 'studio_handover',
            title: 'Archived',
            body: '',
            action_payload: {},
            read_at: new Date().toISOString(),
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('clear-read-btn')).not.toBeDisabled())
    await fireEvent.click(screen.getByTestId('clear-read-btn'))

    await screen.findAllByText('Archive read notifications?')
    const stackedDialogs = screen.getAllByRole('dialog', { hidden: true })
    const confirmDialog = stackedDialogs[stackedDialogs.length - 1]!
    await fireEvent.click(within(confirmDialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryAllByText('Archive read notifications?')).toHaveLength(0)
    })
    expect(archivedIds).toHaveLength(0)
    expect(screen.getByTestId('clear-read-btn')).not.toBeDisabled()
  })

  it('marks unread notification as read when item is clicked', async () => {
    let markedId = ''
    server.use(
      http.post('*/api/v1/user/notifications/:id/read', ({ params }) => {
        markedId = String(params.id)
        return HttpResponse.json({
          success: true,
          data: {
            id: markedId,
            type: 'workspace_invite',
            title: 'Workspace Invitation Today',
            body: 'Join Team Design today',
            action_payload: {},
            status: 'read',
            read_at: new Date().toISOString(),
            created_at: todayISO,
            updated_at: todayISO,
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const items = await screen.findAllByTestId('notification-item')
    // Click unread notification item
    await fireEvent.click(items[0]!)
    await waitFor(() => {
      expect(markedId).toBe('notif_invite_today')
    })
  })

  it('renders all 5 tab triggers: all, unread, invites, billing, archived', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByTestId('tab-all')).toBeInTheDocument()
    expect(screen.getByTestId('tab-unread')).toBeInTheDocument()
    expect(screen.getByTestId('tab-invites')).toBeInTheDocument()
    expect(screen.getByTestId('tab-billing')).toBeInTheDocument()
    expect(screen.getByTestId('tab-archived')).toBeInTheDocument()
  })

  it('renders time-chunked grouping (Today, Yesterday, Earlier) in list', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByTestId('timeline-group-today')).toHaveTextContent('Today')
    expect(screen.getByTestId('timeline-group-yesterday')).toHaveTextContent('Yesterday')
    expect(screen.getByTestId('timeline-group-earlier')).toHaveTextContent('Earlier')

    // In All tab, 4 non-archived items are rendered
    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(4)
  })

  it('filters by unread notifications when unread tab is clicked', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const unreadTab = await screen.findByTestId('tab-unread')
    await fireEvent.click(unreadTab)

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(2)
    expect(screen.getByText('Workspace Invitation Today')).toBeInTheDocument()
    expect(screen.getByText('Low Balance Warning')).toBeInTheDocument()
    expect(screen.queryByText('Studio Handover Yesterday')).toBeNull()
  })

  it('filters by workspace invites & handovers when invites tab is clicked', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const invitesTab = await screen.findByTestId('tab-invites')
    await fireEvent.click(invitesTab)

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(2)
    expect(screen.getByText('Workspace Invitation Today')).toBeInTheDocument()
    expect(screen.getByText('Studio Handover Yesterday')).toBeInTheDocument()
    expect(screen.queryByText('Low Balance Warning')).toBeNull()
  })

  it('filters by billing notifications when billing tab is clicked', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const billingTab = await screen.findByTestId('tab-billing')
    await fireEvent.click(billingTab)

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(2)
    expect(screen.getByText('Low Balance Warning')).toBeInTheDocument()
    expect(screen.getByText('Recharge Success Earlier')).toBeInTheDocument()
    expect(screen.queryByText('Workspace Invitation Today')).toBeNull()
  })

  it('filters by archived notifications when archived tab is clicked', async () => {
    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    const archivedTab = await screen.findByTestId('tab-archived')
    await fireEvent.click(archivedTab)

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(1)
    expect(screen.getByText('Archived Invite')).toBeInTheDocument()
  })

  it('renders distinct empty states per tab when there are no matching items', async () => {
    seedNotificationFacts([])
    renderView({
      props: {
        isOpen: true,
      },
    })

    // Default 'all' tab empty state
    expect(await screen.findByTestId('empty-all')).toBeInTheDocument()
    expect(screen.getByText('No notifications')).toBeInTheDocument()

    // Switch to 'unread' tab
    await fireEvent.click(screen.getByTestId('tab-unread'))
    expect(await screen.findByTestId('empty-unread')).toBeInTheDocument()
    expect(screen.getByText('All caught up')).toBeInTheDocument()

    // Switch to 'invites' tab
    await fireEvent.click(screen.getByTestId('tab-invites'))
    expect(await screen.findByTestId('empty-invites')).toBeInTheDocument()
    expect(screen.getByText('No invitations')).toBeInTheDocument()

    // Switch to 'billing' tab
    await fireEvent.click(screen.getByTestId('tab-billing'))
    expect(await screen.findByTestId('empty-billing')).toBeInTheDocument()
    expect(screen.getByText('No billing notifications')).toBeInTheDocument()

    // Switch to 'archived' tab
    await fireEvent.click(screen.getByTestId('tab-archived'))
    expect(await screen.findByTestId('empty-archived')).toBeInTheDocument()
    expect(screen.getByText('No archived notifications')).toBeInTheDocument()
  })

  it('removes item from unread tab immediately when marked as read with next item sliding up', async () => {
    server.use(
      http.post('*/api/v1/user/notifications/:id/read', () => {
        return HttpResponse.json({
          success: true,
          data: {
            id: 'notif_invite_today',
            type: 'workspace_invite',
            title: 'Workspace Invitation Today',
            body: 'Join Team Design today',
            action_payload: {},
            status: 'read',
            read_at: new Date().toISOString(),
            created_at: todayISO,
            updated_at: todayISO,
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await fireEvent.click(screen.getByTestId('tab-unread'))

    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(2)
    expect(screen.getByText('Workspace Invitation Today')).toBeInTheDocument()
    expect(screen.getByText('Low Balance Warning')).toBeInTheDocument()

    // Click item 1 to mark as read
    await fireEvent.click(items[0]!)

    // Immediately removed from unread tab
    await waitFor(() => {
      expect(screen.queryByText('Workspace Invitation Today')).toBeNull()
    })
    const remainingItems = screen.getAllByTestId('notification-item')
    expect(remainingItems).toHaveLength(1)
    expect(remainingItems[0]).toHaveTextContent('Low Balance Warning')
  })

  it('keeps timeline grouping stable in all tab when item is marked as read', async () => {
    server.use(
      http.post('*/api/v1/user/notifications/:id/read', () => {
        return HttpResponse.json({
          success: true,
          data: {
            id: 'notif_invite_today',
            type: 'workspace_invite',
            title: 'Workspace Invitation Today',
            body: 'Join Team Design today',
            action_payload: {},
            status: 'read',
            read_at: new Date().toISOString(),
            created_at: todayISO,
            updated_at: todayISO,
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    const items = await screen.findAllByTestId('notification-item')
    expect(items).toHaveLength(4)
    expect(items[0]).toHaveTextContent('Workspace Invitation Today')
    expect(items[0]!.querySelector('[data-testid="notification-unread-dot"]')).toBeInTheDocument()

    // Click to mark as read
    await fireEvent.click(items[0]!)

    // Item remains in Today timeline group, but unread dot is gone
    await waitFor(() => {
      expect(items[0]!.querySelector('[data-testid="notification-unread-dot"]')).toBeNull()
    })
    expect(screen.getByText('Workspace Invitation Today')).toBeInTheDocument()
    expect(screen.getAllByTestId('notification-item')).toHaveLength(4)
    expect(screen.getByTestId('timeline-group-today')).toBeInTheDocument()
  })

  it('renders the joined pill from the session-owned decision after accepting an invite', async () => {
    const toast = mockAppToast()
    let acceptedInviteId = ''
    server.use(
      http.post('*/api/v1/user/invites/:inviteId/accept', ({ params }) => {
        acceptedInviteId = String(params.inviteId)
        return HttpResponse.json({
          success: true,
          data: { workspace_id: 'ws_1', user_id: 'usr_me', role: 'member', status: 'accepted' },
        })
      }),
      http.post('*/api/v1/user/notifications/:id/read', () => {
        return HttpResponse.json({
          success: true,
          data: {
            id: 'notif_invite_today',
            type: 'workspace_invite',
            title: 'Workspace Invitation Today',
            body: 'Join Team Design today',
            action_payload: { invite_id: 'inv_1', workspace_id: 'ws_1' },
            status: 'read',
            read_at: new Date().toISOString(),
            created_at: todayISO,
            updated_at: todayISO,
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await fireEvent.click(await screen.findByTestId('accept-invite-btn'))

    await waitFor(() => {
      expect(acceptedInviteId).toBe('inv_1')
    })
    // Pill comes from the session-owned invite decision, not a local copy
    expect(await screen.findByTestId('invite-status-joined')).toHaveTextContent('Joined')
    expect(screen.queryByTestId('accept-invite-btn')).toBeNull()
    expect(screen.queryByTestId('decline-invite-btn')).toBeNull()
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Joined workspace'))
  })

  it('renders the declined pill from the session-owned decision after declining an invite', async () => {
    const toast = mockAppToast()
    let declinedInviteId = ''
    server.use(
      http.post('*/api/v1/user/invites/:inviteId/decline', ({ params }) => {
        declinedInviteId = String(params.inviteId)
        return HttpResponse.json({
          success: true,
          data: { workspace_id: 'ws_1', user_id: 'usr_me', role: 'member', status: 'declined' },
        })
      }),
      http.post('*/api/v1/user/notifications/:id/read', () => {
        return HttpResponse.json({
          success: true,
          data: {
            id: 'notif_invite_today',
            type: 'workspace_invite',
            title: 'Workspace Invitation Today',
            body: 'Join Team Design today',
            action_payload: { invite_id: 'inv_1', workspace_id: 'ws_1' },
            status: 'read',
            read_at: new Date().toISOString(),
            created_at: todayISO,
            updated_at: todayISO,
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await fireEvent.click(await screen.findByTestId('decline-invite-btn'))

    await waitFor(() => {
      expect(declinedInviteId).toBe('inv_1')
    })
    expect(await screen.findByTestId('invite-status-declined')).toHaveTextContent('Declined')
    expect(screen.queryByTestId('accept-invite-btn')).toBeNull()
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Declined invitation'))
  })

  it('surfaces an explicit failure toast when one archive in clear-read fails', async () => {
    const toast = mockAppToast()
    const archivedIds: string[] = []
    server.use(
      http.post('*/api/v1/user/notifications/:id/archive', ({ params }) => {
        const id = String(params.id)
        if (id === 'notif_handover_yesterday') {
          return HttpResponse.json({ success: false, message: 'archive failed' }, { status: 500 })
        }
        archivedIds.push(id)
        return HttpResponse.json({
          success: true,
          data: {
            id,
            type: 'studio_handover',
            title: 'Archived',
            body: '',
            action_payload: {},
            status: 'archived',
            read_at: new Date().toISOString(),
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
        })
      }),
    )

    seedNotificationFacts(sampleNotifications)
    renderView({
      props: {
        isOpen: true,
      },
    })

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('clear-read-btn')).not.toBeDisabled())
    await fireEvent.click(screen.getByTestId('clear-read-btn'))

    await screen.findAllByText('Archive read notifications?')
    const stackedDialogs = screen.getAllByRole('dialog', { hidden: true })
    const confirmDialog = stackedDialogs[stackedDialogs.length - 1]!
    await fireEvent.click(within(confirmDialog).getByRole('button', { name: 'Archive' }))

    // The succeeding item still archives per-item; the failure is explicit
    // via toast instead of a bare Promise.all rejection.
    await waitFor(() => {
      expect(archivedIds).toContain('notif_recharge_earlier')
    })
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith('Could not archive notifications. Please try again')
    })
  })

  describe('loading, error, and tab chrome', () => {
    it('renders skeleton rows that preserve row height while loading', async () => {
      seedNotificationFacts([])
      const response = createDeferred<Response>()
      server.use(http.get('*/api/v1/user/notifications', () => response.promise))
      renderView({ props: { isOpen: true } })

      const skeleton = await screen.findByTestId('notification-center-skeleton')
      expect(skeleton).toHaveAttribute('role', 'status')
      expect(screen.queryByTestId('empty-all')).toBeNull()
      expect(screen.queryByTestId('notification-item')).toBeNull()
      response.resolve(HttpResponse.json({ data: { items: [], total: 0, limit: 20, offset: 0 } }))
      expect(await screen.findByTestId('empty-all')).toBeInTheDocument()
    })

    it('toasts the failed request and retries the notification facts', async () => {
      seedNotificationFacts([])
      const toast = mockAppToast()
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

      // 初载失败只走 toast；中性空态 + 重试按钮即恢复入口。
      await expectToastFeedback(toast, 'Could not load notifications. Please try again.')
      expect(screen.getByTestId('notification-center-error')).toBeInTheDocument()
      expect(screen.queryByTestId('empty-all')).toBeNull()
      await fireEvent.click(screen.getByTestId('notification-center-retry-btn'))
      expect(await screen.findByTestId('empty-all')).toBeInTheDocument()
      expect(requests).toBe(2)
    })

    it('shows the unread tab as a numeric badge instead of a color-only dot', async () => {
      seedNotificationFacts(sampleNotifications)
      renderView({
        props: {
          isOpen: true,
        },
      })

      expect(await screen.findByRole('dialog')).toBeInTheDocument()
      expect(screen.queryByTestId('tab-unread-dot')).toBeNull()
      expect(await screen.findByTestId('tab-unread-count')).toHaveTextContent('2')
      const unreadTab = screen.getByTestId('tab-unread')
      expect(unreadTab.textContent).toContain('Unread')
      expect(unreadTab.textContent).toContain('2')
    })
  })
})
