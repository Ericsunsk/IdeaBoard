import { describe, expect, it } from 'vitest'
import type { NotificationDTO } from '@/api/generated-query/public/model'
import {
  applyNotificationViewOverrides,
  readPayloadInviteDecision,
  resolveNotificationDerivedState,
} from './notificationDisplay'

function makeNotification(
  overrides: Partial<NotificationDTO> & { id: string },
): NotificationDTO {
  return {
    type: 'workspace_invite',
    title: 'Invite',
    body: 'Join workspace',
    action_payload: {},
    status: 'unread',
    read_at: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

describe('notificationDisplay', () => {
  it('reads the invite decision from the payload invite_status', () => {
    expect(
      readPayloadInviteDecision(makeNotification({ id: 'a' })),
    ).toBeNull()
    expect(
      readPayloadInviteDecision(
        makeNotification({ id: 'b', action_payload: { invite_id: 'inv_1', invite_status: 'accepted' } }),
      ),
    ).toBe('joined')
    for (const terminal of ['declined', 'cancelled', 'expired']) {
      expect(
        readPayloadInviteDecision(
          makeNotification({ id: `d-${terminal}`, action_payload: { invite_id: 'inv_1', invite_status: terminal } }),
        ),
      ).toBe('declined')
    }
  })

  it('resolves a pending invite to actions with no pill', () => {
    const state = resolveNotificationDerivedState(
      makeNotification({ id: 'n', action_payload: { invite_id: 'inv_1' } }),
    )
    expect(state.inviteDecision).toBeNull()
    expect(state.pill).toBeUndefined()
  })

  it('resolves the joined/declined pill from the payload when already decided', () => {
    const joined = resolveNotificationDerivedState(
      makeNotification({
        id: 'n',
        status: 'read',
        action_payload: { invite_id: 'inv_1', invite_status: 'accepted' },
      }),
    )
    expect(joined.inviteDecision).toBe('joined')
    expect(joined.pill?.testId).toBe('invite-status-joined')

    const declined = resolveNotificationDerivedState(
      makeNotification({
        id: 'n',
        status: 'read',
        action_payload: { invite_id: 'inv_1', invite_status: 'declined' },
      }),
    )
    expect(declined.inviteDecision).toBe('declined')
    expect(declined.pill?.testId).toBe('invite-status-declined')
  })

  it('lets the local decision win over the payload with identical pill copy', () => {
    const pending = makeNotification({ id: 'n', action_payload: { invite_id: 'inv_1' } })
    const viaLocal = resolveNotificationDerivedState(pending, 'joined')
    const viaPayload = resolveNotificationDerivedState(
      makeNotification({ id: 'n', action_payload: { invite_id: 'inv_1', invite_status: 'accepted' } }),
    )
    expect(viaLocal.inviteDecision).toBe('joined')
    expect(viaLocal.pill).toEqual(viaPayload.pill)

    const declinedLocal = resolveNotificationDerivedState(pending, 'declined')
    expect(declinedLocal.pill?.testId).toBe('invite-status-declined')
  })

  it('merges status and decision overrides onto a base list without touching others', () => {
    const items = [
      makeNotification({ id: 'a', action_payload: { invite_id: 'inv_a' } }),
      makeNotification({ id: 'b', status: 'read' }),
    ]
    const merged = applyNotificationViewOverrides(items, {
      statuses: new Map([['a', { status: 'read', readAt: '2026-01-01T00:00:00.000Z' }]]),
      decisions: new Map([['a', 'joined']]),
    })
    expect(merged).toHaveLength(2)
    expect(merged[0]?.status).toBe('read')
    expect(merged[0]?.read_at).toBe('2026-01-01T00:00:00.000Z')
    expect(
      (merged[0]?.action_payload as Record<string, unknown> | undefined)?.invite_status,
    ).toBe('accepted')
    // Untouched items keep identity
    expect(merged[1]).toBe(items[1])
    // Base list is never mutated
    expect(items[0]?.status).toBe('unread')
  })

  it('returns a copy of the base list when no overrides are pending', () => {
    const items = [makeNotification({ id: 'a' })]
    const merged = applyNotificationViewOverrides(items, {
      statuses: new Map(),
      decisions: new Map(),
    })
    expect(merged).toEqual(items)
    expect(merged).not.toBe(items)
  })
})
