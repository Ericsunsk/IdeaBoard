import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@/test/render'
import NotificationBell from './NotificationBell.vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { seedNotificationFacts } from '@/test/notificationFacts'

vi.mock('@/features/auth', () => ({
  useUserStore: () => ({ user: { public_id: 'usr_notifications' } }),
}))

let client: QueryClient
function renderBell(count: number) {
  seedNotificationFacts([], count)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(NotificationBell, {
    global: { plugins: [[VueQueryPlugin, { queryClient: client }]] },
  })
}
afterEach(() => client?.clear())

describe('NotificationBell', () => {
  it('renders bell button with generic page chrome control styles and hit-target-32', () => {
    renderBell(0)

    const bellBtn = screen.getByTestId('notification-bell-btn')
    expect(bellBtn).toBeInTheDocument()
    const bellClass = bellBtn.getAttribute('class') ?? ''
    expect(bellClass).toContain('page-chrome-icon-button')
    expect(bellClass).not.toContain('canvas-chrome-icon-button')
    expect(bellClass).toContain('hit-target-32')
    expect(bellClass).toContain('page-chrome-control')

    const icon = bellBtn.querySelector('svg')
    expect(icon).toBeTruthy()
    expect(icon?.getAttribute('class')).toContain('page-chrome-icon')
  })

  it('provides accessible aria-label without count when unreadCount is 0', () => {
    renderBell(0)

    const bellBtn = screen.getByTestId('notification-bell-btn')
    expect(bellBtn).toHaveAttribute('aria-label', 'Notifications')
    expect(screen.queryByTestId('notification-unread-badge')).toBeNull()
  })

  it('renders unread red dot indicator without number text and announces unread count in aria-label', async () => {
    renderBell(7)

    const bellBtn = screen.getByTestId('notification-bell-btn')
    await waitFor(() => expect(bellBtn).toHaveAttribute('aria-label', 'Notifications, 7 unread'))

    const badge = screen.getByTestId('notification-unread-badge')
    expect(badge).toBeInTheDocument()
    expect(badge).toHaveTextContent('')
    const badgeClass = badge.getAttribute('class') ?? ''
    expect(badgeClass).toContain('bg-destructive')
    expect(badgeClass).toContain('rounded-pill')
  })

  it('keeps badge as clean red dot without text even when unreadCount exceeds 99', async () => {
    renderBell(105)

    const bellBtn = screen.getByTestId('notification-bell-btn')
    await waitFor(() => expect(bellBtn).toHaveAttribute('aria-label', 'Notifications, 105 unread'))

    const badge = screen.getByTestId('notification-unread-badge')
    expect(badge).toHaveTextContent('')
  })

  it('opens popover when bell button is clicked', async () => {
    renderBell(2)

    const bellBtn = screen.getByTestId('notification-bell-btn')
    await fireEvent.click(bellBtn)

    expect(await screen.findByTestId('notification-popover-content')).toBeInTheDocument()
  })
})
