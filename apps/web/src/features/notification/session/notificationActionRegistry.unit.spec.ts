import { describe, expect, it, vi } from 'vitest'
import type { NotificationDTO, NotificationDTOType } from '@/api/generated-query/public/model'
import {
  registerNotificationActionResolver,
  resolveNotificationActions,
} from './notificationActionRegistry'

describe('notificationActionRegistry', () => {
  it('resolves built-in workspace_invite actions when pending', async () => {
    const notification: NotificationDTO = {
      id: 'n_1',
      type: 'workspace_invite',
      title: 'Invite',
      body: 'Join workspace',
      action_payload: { invite_id: 'inv_123' },
      status: 'unread',
      read_at: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }

    const onAccept = vi.fn().mockResolvedValue(true)
    const onDecline = vi.fn().mockResolvedValue(true)

    const resolution = resolveNotificationActions(notification, {
      onAcceptInvite: onAccept,
      onDeclineInvite: onDecline,
    })

    expect(resolution).not.toBeNull()
    expect(resolution?.status).toBeUndefined()
    expect(resolution?.actions).toHaveLength(2)
    expect(resolution?.actions[0]?.id).toBe('accept')
    expect(resolution?.actions[1]?.id).toBe('decline')

    // Execute accept
    const isAcceptSuccess = await resolution?.actions[0]?.execute({
      notification,
      payload: { invite_id: 'inv_123' },
    })
    expect(isAcceptSuccess).toBe(true)
    expect(onAccept).toHaveBeenCalledWith('inv_123', 'n_1')

    // Execute decline
    const isDeclineSuccess = await resolution?.actions[1]?.execute({
      notification,
      payload: { invite_id: 'inv_123' },
    })
    expect(isDeclineSuccess).toBe(true)
    expect(onDecline).toHaveBeenCalledWith('inv_123', 'n_1')
  })

  it('resolves built-in organization_invite actions when pending', async () => {
    const notification: NotificationDTO = {
      id: 'n_org_1',
      type: 'organization_invite',
      title: 'Org Invite',
      body: 'Join organization',
      action_payload: { invite_id: 'osi_123' },
      status: 'unread',
      read_at: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }

    const onAccept = vi.fn().mockResolvedValue(true)
    const onDecline = vi.fn().mockResolvedValue(true)

    const resolution = resolveNotificationActions(notification, {
      onAcceptInvite: onAccept,
      onDeclineInvite: onDecline,
    })

    expect(resolution).not.toBeNull()
    expect(resolution?.status).toBeUndefined()
    expect(resolution?.actions).toHaveLength(2)
    expect(resolution?.actions[0]?.id).toBe('accept')
    expect(resolution?.actions[1]?.id).toBe('decline')

    const isAcceptSuccess = await resolution?.actions[0]?.execute({
      notification,
      payload: { invite_id: 'osi_123' },
    })
    expect(isAcceptSuccess).toBe(true)
    expect(onAccept).toHaveBeenCalledWith('osi_123', 'n_org_1')
  })

  it('resolves built-in workspace_invite status pill when already accepted or declined', () => {
    const acceptedNotif: NotificationDTO = {
      id: 'n_2',
      type: 'workspace_invite',
      title: 'Invite',
      body: 'Join workspace',
      action_payload: { invite_id: 'inv_123', invite_status: 'accepted' },
      status: 'read',
      read_at: new Date().toISOString(),
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }

    const resAccepted = resolveNotificationActions(acceptedNotif)
    expect(resAccepted?.status?.statusKey).toBe('joined')
    expect(resAccepted?.status?.testId).toBe('invite-status-joined')
    expect(resAccepted?.actions).toHaveLength(0)

    const declinedNotif: NotificationDTO = {
      id: 'n_3',
      type: 'workspace_invite',
      title: 'Invite',
      body: 'Join workspace',
      action_payload: { invite_id: 'inv_123', invite_status: 'declined' },
      status: 'read',
      read_at: new Date().toISOString(),
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }

    const resDeclined = resolveNotificationActions(declinedNotif)
    expect(resDeclined?.status?.statusKey).toBe('declined')
    expect(resDeclined?.status?.testId).toBe('invite-status-declined')
    expect(resDeclined?.actions).toHaveLength(0)

    for (const terminal of ['cancelled', 'expired']) {
      const notif: NotificationDTO = {
        id: `n_${terminal}`,
        type: 'workspace_invite',
        title: 'Invite',
        body: 'Join workspace',
        action_payload: { invite_id: 'inv_123', invite_status: terminal },
        status: 'read',
        read_at: new Date().toISOString(),
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      }
      const resolved = resolveNotificationActions(notif)
      expect(resolved?.actions).toHaveLength(0)
      expect(resolved?.status?.testId).toBe('invite-status-declined')
    }
  })

  it('resolves studio_handover and billing notices to explicit no-action (typed keys)', () => {
    const handoverNotif: NotificationDTO = {
      id: 'n_handover_builtin',
      type: 'studio_handover',
      title: 'Handover',
      body: 'Studio transfer requested',
      action_payload: {
        transfer_invite_id: 'bti_1',
        workspace_id: 'ws_1',
        workspace_name: 'Studio',
        from_user_id: 'usr_1',
      },
      status: 'unread',
      read_at: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }
    expect(resolveNotificationActions(handoverNotif)).toBeNull()

    for (const type of ['billing_credit_low', 'billing_recharge_success'] as const) {
      const notif: NotificationDTO = {
        id: `n_${type}`,
        type,
        title: 'Billing',
        body: 'Billing notice',
        action_payload: { billing_account_id: 'bac_1' },
        status: 'unread',
        read_at: null,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      }
      expect(resolveNotificationActions(notif)).toBeNull()
    }

    for (const type of [
      'workspace_member_joined',
      'workspace_invite_declined',
      'organization_member_joined',
      'organization_invite_declined',
    ] as const) {
      const notif: NotificationDTO = {
        id: `n_${type}`,
        type,
        title: 'Notice',
        body: 'Inviter-side notice',
        action_payload: { invite_id: 'wsi_1' },
        status: 'unread',
        read_at: null,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      }
      expect(resolveNotificationActions(notif)).toBeNull()
    }
  })

  it('allows registering a custom action resolver for other notification types', async () => {
    const customExecute = vi.fn().mockResolvedValue(true)
    registerNotificationActionResolver('custom_demo', (_notif) => ({
      actions: [
        {
          id: 'confirm_transfer',
          labelKey: 'studio.handover.confirm',
          variant: 'primary',
          execute: customExecute,
        },
      ],
    }))

    const handoverNotif: NotificationDTO = {
      id: 'n_handover',
      // Registry is string-keyed (runtime-extensible); generated NotificationDTOType is closed.
      type: 'custom_demo' as unknown as NotificationDTOType,
      title: 'Handover',
      body: 'Studio transfer requested',
      action_payload: { studio_id: 'st_1' },
      status: 'unread',
      read_at: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }

    const resolution = resolveNotificationActions(handoverNotif)
    expect(resolution?.actions).toHaveLength(1)
    expect(resolution?.actions[0]?.id).toBe('confirm_transfer')

    const isSuccess = await resolution?.actions[0]?.execute({
      notification: handoverNotif,
      payload: { studio_id: 'st_1' },
    })
    expect(isSuccess).toBe(true)
    expect(customExecute).toHaveBeenCalled()
  })

  it('returns null for unregistered notification types without actions', () => {
    const notif: NotificationDTO = {
      id: 'n_info',
      // Unregistered runtime type: keeps string-keyed miss path covered without widening generated DTO union.
      type: 'generic_broadcast' as unknown as NotificationDTOType,
      title: 'Broadcast',
      body: 'Hello',
      action_payload: {},
      status: 'unread',
      read_at: null,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }

    expect(resolveNotificationActions(notif)).toBeNull()
  })
})
