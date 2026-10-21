// Billing transfer resource contract (#948).
//
// MSW-level contract: exercises the real Public generated transport against
// the AGREED envelopes, so backend drift surfaces here instead of in UI.
// No generated-query mocks by design (see checkTestGuardrails).
import { describe, expect, test } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import {
  acceptTransferInvite,
  cancelTransferInvite,
  detachBillingMember,
  fetchBillingManagers,
  fetchTransferInvites,
  initiateTransferInvite,
  rejectTransferInvite,
} from './resource'

const WORKSPACE_ID = 'ws_handover1'
const INVITE_ID = 'bti_agreed01'
const FROM_USER = 'usr_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
const TO_USER = 'usr_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'

const AGREED_INVITE = {
  public_id: INVITE_ID,
  billing_account_id: 'ba_shared01',
  from_user_id: FROM_USER,
  to_user_id: TO_USER,
  status: 'pending',
  scope_workspace_ids: [],
  created_at: '2026-10-21T00:00:00Z',
}

/**
 * Detach wire shape.
 * @externalContract Public API RemoveWorkspaceBillingAccountMember200Data:
 * consumed verbatim from the generated contract (`DELETE
 * .../billing-account/members/{id}` renders `{ data: { removed: true } }`).
 */
type DetachConfirmation = { data: { removed: boolean } }

describe('billing transfer invite envelope', () => {
  test('lists the agreed pending invite', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/billing-account/transfer-invites', () =>
        HttpResponse.json({ data: [AGREED_INVITE] }),
      ),
    )
    const invites = await fetchTransferInvites(WORKSPACE_ID)
    expect(invites).toEqual([
      {
        publicId: INVITE_ID,
        billingAccountId: 'ba_shared01',
        fromUserId: FROM_USER,
        toUserId: TO_USER,
        status: 'pending',
        scopeWorkspaceIds: [],
        createdAt: '2026-10-21T00:00:00Z',
      },
    ])
  })

  test('rejects hidden invites with the contract not-found code', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/billing-account/transfer-invites', () =>
        HttpResponse.json(
          { error: { code: 'billing_account_not_found', message: '资金账户不存在' } },
          { status: 404 },
        ),
      ),
    )
    await expect(fetchTransferInvites(WORKSPACE_ID)).rejects.toThrow()
  })

  test.each([
    { ...AGREED_INVITE, status: 'expired' },
    { ...AGREED_INVITE, public_id: 'ws_not_an_invite' },
    { ...AGREED_INVITE, scope_workspace_ids: ['usr_not_a_workspace'] },
  ])('rejects malformed invitation facts from the wire ($status)', async (malformed) => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/billing-account/transfer-invites', () =>
        HttpResponse.json({ data: [malformed] }),
      ),
    )
    await expect(fetchTransferInvites(WORKSPACE_ID)).rejects.toThrow()
  })

  test('initiates with the agreed new-owner body', async () => {
    useMswHandlers(
      http.post(
        '/api/v1/workspaces/:workspaceId/billing-account/transfer-invites',
        async ({ request }) => {
          const body = (await request.json()) as { data: { new_owner_user_id: string } }
          expect(body.data.new_owner_user_id).toBe(TO_USER)
          return HttpResponse.json(
            { data: { ...AGREED_INVITE, status: 'pending' } },
            { status: 201 },
          )
        },
      ),
    )
    const invited = await initiateTransferInvite(WORKSPACE_ID, TO_USER)
    expect(invited.status).toBe('pending')
  })

  test('accept converges to the accepted state', async () => {
    useMswHandlers(
      http.post(
        '/api/v1/workspaces/:workspaceId/billing-account/transfer-invites/:inviteId/accept',
        () => HttpResponse.json({ data: { ...AGREED_INVITE, status: 'accepted' } }),
      ),
    )
    const decided = await acceptTransferInvite(WORKSPACE_ID, INVITE_ID)
    expect(decided.status).toBe('accepted')
  })

  test('reject and cancel converge to their terminal states', async () => {
    useMswHandlers(
      http.post(
        '/api/v1/workspaces/:workspaceId/billing-account/transfer-invites/:inviteId/reject',
        () => HttpResponse.json({ data: { ...AGREED_INVITE, status: 'rejected' } }),
      ),
    )
    await expect(rejectTransferInvite(WORKSPACE_ID, INVITE_ID)).resolves.toMatchObject({
      status: 'rejected',
    })

    useMswHandlers(
      http.post(
        '/api/v1/workspaces/:workspaceId/billing-account/transfer-invites/:inviteId/cancel',
        () => HttpResponse.json({ data: { ...AGREED_INVITE, status: 'cancelled' } }),
      ),
    )
    await expect(cancelTransferInvite(WORKSPACE_ID, INVITE_ID)).resolves.toMatchObject({
      status: 'cancelled',
    })
  })

  test('detach removes the management row without a response body contract', async () => {
    useMswHandlers(
      http.delete('/api/v1/workspaces/:workspaceId/billing-account/members/:memberId', () =>
        HttpResponse.json({ data: { removed: true } } satisfies DetachConfirmation),
      ),
    )
    await expect(detachBillingMember(WORKSPACE_ID, TO_USER)).resolves.toBeUndefined()
  })
})

describe('financial manager roster', () => {
  test('parses the finance-only role contract', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/billing-account/members', () =>
        HttpResponse.json({
          data: [
            { user_id: FROM_USER, role: 'owner' },
            { user_id: TO_USER, role: 'admin' },
          ],
        }),
      ),
    )
    expect(await fetchBillingManagers(WORKSPACE_ID)).toEqual([
      { userId: FROM_USER, role: 'owner' },
      { userId: TO_USER, role: 'admin' },
    ])
  })
  test('rejects content-only roles in the financial manager roster', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/billing-account/members', () =>
        HttpResponse.json({ data: [{ user_id: TO_USER, role: 'member' }] }),
      ),
    )
    await expect(fetchBillingManagers(WORKSPACE_ID)).rejects.toThrow()
  })
})
