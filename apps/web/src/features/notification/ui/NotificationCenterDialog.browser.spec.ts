import { beforeEach, describe, expect, test, vi } from 'vitest'
import { page } from 'vitest/browser'
import { computed, ref } from 'vue'
import { render, screen, waitFor } from '@/test/render'
import { i18n } from '@/core/i18n'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import type { useNotifications } from '../session/useNotifications'
import NotificationCenterDialog from './NotificationCenterDialog.vue'
import NotificationPopover from './NotificationPopover.vue'
import '@/app/style.css'

const mocks = vi.hoisted(() => ({ useNotifications: vi.fn<typeof useNotifications>() }))
vi.mock('../session/useNotifications', () => ({ useNotifications: mocks.useNotifications }))

type NotificationSession = ReturnType<typeof useNotifications>

beforeEach(() => {
  i18n.global.locale.value = 'en'
  mocks.useNotifications.mockReturnValue({
    notifications: computed<NotificationDTO[]>(() => [
      {
        id: 'notif_read',
        type: 'studio_handover',
        title: 'Studio handover',
        body: 'Ownership transferred',
        action_payload: {},
        status: 'read',
        read_at: '2026-10-28T00:00:00Z',
        created_at: '2026-10-28T00:00:00Z',
        updated_at: '2026-10-28T00:00:00Z',
      },
    ]),
    hasNextPage: ref(false),
    isFetchingNextPage: ref(false),
    isFetchNextPageError: ref(false),
    fetchNextPage: vi.fn<NotificationSession['fetchNextPage']>(),
    unreadCount: computed(() => 2),
    hasUnread: computed(() => true),
    isLoading: computed(() => false),
    isError: computed(() => false),
    isFetching: computed(() => false),
    isMarkingRead: computed(() => false),
    isMarkingAllRead: computed(() => false),
    isArchiving: computed(() => false),
    markAsRead: vi.fn<NotificationSession['markAsRead']>(),
    markAllAsRead: vi.fn<NotificationSession['markAllAsRead']>(),
    archive: vi.fn<NotificationSession['archive']>(),
    archiveRead: vi.fn<NotificationSession['archiveRead']>(),
    acceptInvite: vi.fn<NotificationSession['acceptInvite']>(),
    declineInvite: vi.fn<NotificationSession['declineInvite']>(),
    refetch: vi.fn<NotificationSession['refetch']>(),
  })
})

function expectTouchTarget(element: Element) {
  const bounds = element.getBoundingClientRect()
  const overlay = getComputedStyle(element, '::after')
  expect(Math.max(bounds.width, Number.parseFloat(overlay.width))).toBeGreaterThanOrEqual(32)
  expect(Math.max(bounds.height, Number.parseFloat(overlay.height))).toBeGreaterThanOrEqual(32)
}

describe('notification chrome geometry', () => {
  test.each([360, 1280])('preserves tab and header hit bounds at %ipx', async (width) => {
    await page.viewport(width, 800)
    render(NotificationCenterDialog, { props: { isOpen: true } })
    const dialog = await screen.findByRole('dialog')
    const tabs = screen.getAllByRole('tab')
    const rail = screen.getByRole('tablist').parentElement!

    await waitFor(() => expect(dialog.getBoundingClientRect().width).toBeGreaterThan(0))
    expect(tabs).toHaveLength(5)
    expect(getComputedStyle(rail).overflowX).toBe('auto')
    for (const tab of tabs) {
      expect(getComputedStyle(tab).flexShrink).toBe('0')
      expectTouchTarget(tab)
    }
    if (width === 360) {
      expect(rail.scrollWidth).toBeGreaterThan(rail.clientWidth)
      rail.scrollLeft = rail.scrollWidth
      expect(rail.scrollLeft).toBeGreaterThan(0)
    }
    for (const name of ['Mark all as read', 'Clear read']) {
      expectTouchTarget(screen.getByRole('button', { name }))
    }
  })

  test('expands the popover action hit bounds in both axes', async () => {
    await page.viewport(1280, 800)
    render(NotificationPopover, {
      props: { isOpen: true },
      slots: { trigger: '<button>Notifications</button>' },
    })
    const action = await screen.findByRole('button', { name: 'View all notifications' })
    await waitFor(() => expect(action.getBoundingClientRect().width).toBeGreaterThan(0))
    expectTouchTarget(action)
    const overlay = getComputedStyle(action, '::after')
    expect(Number.parseFloat(overlay.width)).toBeGreaterThanOrEqual(32)
  })
})
