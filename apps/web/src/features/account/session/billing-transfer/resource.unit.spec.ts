import { describe, expect, test } from 'vitest'
import { acceptTransferInvite, billingTransferQueryKeys, initiateTransferInvite } from './resource'

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
})
