import { describe, expect, test } from 'vitest'
import {
  acceptTransferInvite,
  billingTransferQueryKeys,
  initiateTransferInvite,
  isTransferInviteLapsed,
  isTransferInviteLive,
  type BillingTransferInviteSummary,
} from './resource'

function buildInvite(
  overrides: Partial<BillingTransferInviteSummary> = {},
): BillingTransferInviteSummary {
  return {
    publicId: 'bti_expiry01',
    billingAccountId: 'ba_shared01',
    fromUserId: 'usr_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    toUserId: 'usr_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
    status: 'pending',
    scopeWorkspaceIds: [],
    expiresAt: '2026-10-28T00:00:00Z',
    ...overrides,
  }
}

describe('billingTransferResource', () => {
  test('rejects malformed operation targets before a request can be sent', async () => {
    await expect(initiateTransferInvite('ws_valid', 'ba_not_a_user')).rejects.toThrow(
      'Invalid user opaque ID',
    )
    await expect(acceptTransferInvite('ws_valid', 'ws_not_an_invite')).rejects.toThrow(
      'Invalid transfer invite opaque ID',
    )
    await expect(acceptTransferInvite('', 'bti_valid')).rejects.toThrow()
  })
  test('isolates manager cache keys by viewer identity', () => {
    expect(billingTransferQueryKeys.members('ws_123', 'usr_owner')).not.toEqual(
      billingTransferQueryKeys.members('ws_123', 'usr_member'),
    )
  })
  test('treats only pending invites at or past expires_at as lapsed', () => {
    const deadline = Date.parse('2026-10-28T00:00:00Z')
    expect(isTransferInviteLapsed(buildInvite(), deadline - 1)).toBe(false)
    expect(isTransferInviteLapsed(buildInvite(), deadline)).toBe(true)
    expect(isTransferInviteLapsed(buildInvite({ status: 'accepted' }), deadline + 1)).toBe(false)
    expect(isTransferInviteLapsed(buildInvite({ expiresAt: undefined }), deadline + 1)).toBe(false)
    expect(isTransferInviteLive(buildInvite(), deadline - 1)).toBe(true)
    expect(isTransferInviteLive(buildInvite(), deadline)).toBe(false)
    expect(isTransferInviteLive(buildInvite({ status: 'accepted' }), deadline - 1)).toBe(false)
  })
})
