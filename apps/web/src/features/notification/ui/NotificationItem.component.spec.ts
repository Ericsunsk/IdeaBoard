import { describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@/test/render'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import NotificationItem from './NotificationItem.vue'

function buildNotification(overrides: Partial<NotificationDTO> = {}): NotificationDTO {
  return {
    id: 'notif_1',
    type: 'studio_handover',
    title: 'Studio handover',
    body: 'Ownership transfer pending',
    action_payload: {},
    status: 'unread',
    read_at: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

describe('NotificationItem', () => {
  it('marks an unread notification as read when the row is clicked', async () => {
    const { emitted } = render(NotificationItem, {
      props: { notification: buildNotification() },
    })

    await fireEvent.click(screen.getByTestId('notification-item'))

    expect(emitted().read).toEqual([['notif_1']])
  })

  it('exposes the unread row as a keyboard-operable button', async () => {
    const { emitted } = render(NotificationItem, {
      props: { notification: buildNotification() },
    })

    const row = screen.getByTestId('notification-item')
    expect(row).toHaveAttribute('tabindex', '0')
    expect(row).toHaveAttribute('role', 'button')

    await fireEvent.keyDown(row, { key: 'Enter' })
    expect(emitted().read).toEqual([['notif_1']])

    await fireEvent.keyDown(row, { key: ' ' })
    expect(emitted().read).toEqual([['notif_1'], ['notif_1']])
  })

  it('ignores row keydown events that come from inner action buttons', async () => {
    const { emitted } = render(NotificationItem, {
      props: { notification: buildNotification() },
    })

    const archiveBtn = screen.getByTestId('archive-notification-btn')
    await fireEvent.keyDown(archiveBtn, { key: 'Enter' })

    expect(emitted().read).toBeUndefined()
  })

  it('keeps read rows out of the tab order', async () => {
    const { emitted } = render(NotificationItem, {
      props: {
        notification: buildNotification({ status: 'read', read_at: new Date().toISOString() }),
      },
    })

    const row = screen.getByTestId('notification-item')
    expect(row).not.toHaveAttribute('tabindex')
    expect(row).not.toHaveAttribute('role')

    await fireEvent.click(row)
    expect(emitted().read).toBeUndefined()
  })

  it('emits archive from the hover action button without marking read', async () => {
    const { emitted } = render(NotificationItem, {
      props: { notification: buildNotification() },
    })

    await fireEvent.click(screen.getByTestId('archive-notification-btn'))

    expect(emitted().archive).toEqual([['notif_1']])
    expect(emitted().read).toBeUndefined()
  })

  it('hides the archive button for archived notifications', () => {
    render(NotificationItem, {
      props: { notification: buildNotification({ status: 'archived' }) },
    })

    expect(screen.queryByTestId('archive-notification-btn')).toBeNull()
  })
})
