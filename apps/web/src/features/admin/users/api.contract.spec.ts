import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import {
  banAdminUser,
  deleteAdminUser,
  impersonateAdminUser,
  revokeAdminUserSessions,
  unbanAdminUser,
  updateAdminUserRole,
} from './api'

// Control client boundary contract (issue 808): every users mutation
// hits /api/control/v1 with a header-only Idempotency-Key and an operator
// reason, and speaks the contract envelope. Transport itself is owned by
// the generated control client (controlMutator prefix); this spec pins
// the adapter half: headers, bodies, and control error-code branches.
describe('admin users control api boundary', () => {
  const userId = 'usr_abc123'

  it('sends ban with reason body plus idempotency and operator headers', async () => {
    let seen: { headers: Record<string, string>; body: unknown } | undefined
    useMswHandlers(
      http.post('/api/control/v1/users/:userId/ban', async ({ request }) => {
        seen = {
          headers: {
            idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
            operatorReason: request.headers.get('X-Operator-Reason') ?? '',
          },
          body: await request.json(),
        }
        return HttpResponse.json({ data: { banned: true } })
      }),
    )

    await banAdminUser({
      publicId: userId,
      reason: 'spam wave',
      expires_in_days: 7,
      operatorReason: 'spam wave',
      idempotencyKey: 'control-ui:test-ban',
    })

    expect(seen?.body).toEqual({ reason: 'spam wave', expires_in_days: 7 })
    expect(seen?.headers.idempotencyKey).toBe('control-ui:test-ban')
    expect(seen?.headers.operatorReason).toBe('spam wave')
  })

  it('mints idempotency keys per attempt for reason-only mutations', async () => {
    const keys: Array<string | null> = []
    useMswHandlers(
      http.post('/api/control/v1/users/:userId/unban', ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        return HttpResponse.json({ data: { unbanned: true } })
      }),
    )

    await unbanAdminUser(userId, { operatorReason: 'appeal accepted' })
    await unbanAdminUser(userId, { operatorReason: 'appeal accepted' })

    expect(keys).toHaveLength(2)
    expect(keys[0]).toMatch(/^control:users\.unban:/)
    expect(keys[0]).not.toBe(keys[1])
  })

  it('maps control error codes to operator copy', async () => {
    useMswHandlers(
      http.post('/api/control/v1/users/:userId/unban', () =>
        HttpResponse.json(
          { error: { code: 'control_reason_required', message: 'no reason', request_id: 'r' } },
          { status: 400 },
        ),
      ),
      http.delete('/api/control/v1/users/:userId', () =>
        HttpResponse.json(
          { error: { code: 'idempotency_conflict', message: 'reused', request_id: 'r' } },
          { status: 409 },
        ),
      ),
    )

    await expect(unbanAdminUser(userId, { operatorReason: 'x' })).rejects.toThrow(
      '需要填写操作原因',
    )
    await expect(deleteAdminUser(userId, { operatorReason: 'x' })).rejects.toThrow(
      '操作已提交，请勿重复操作',
    )
  })

  it('refuses to send mutations without an operator reason', async () => {
    await expect(
      updateAdminUserRole({ publicId: userId, role: 'admin', operatorReason: '   ' }),
    ).rejects.toThrow('需要填写操作原因')
    await expect(revokeAdminUserSessions(userId, { operatorReason: '' })).rejects.toThrow(
      '需要填写操作原因',
    )
    await expect(impersonateAdminUser(userId, { operatorReason: '' })).rejects.toThrow(
      '需要填写操作原因',
    )
  })

  it('sends role updates and session revokes through control', async () => {
    const calls: Array<{ method: string; url: string; body: unknown }> = []
    useMswHandlers(
      http.put('/api/control/v1/users/:userId/role', async ({ request }) => {
        calls.push({ method: 'PUT', url: request.url, body: await request.json() })
        return HttpResponse.json({ data: { updated: true } })
      }),
      http.delete('/api/control/v1/users/:userId/sessions', async ({ request }) => {
        calls.push({ method: 'DELETE', url: request.url, body: null })
        return HttpResponse.json({ data: { revoked: true } })
      }),
    )

    await updateAdminUserRole({ publicId: userId, role: 'admin', operatorReason: 'promotion' })
    await revokeAdminUserSessions(userId, { operatorReason: 'compromise' })

    expect(calls).toHaveLength(2)
    expect(calls[0]).toMatchObject({ method: 'PUT', body: { role: 'admin' } })
    expect(calls[0]?.url).toContain(`/api/control/v1/users/${userId}/role`)
    expect(calls[1]?.url).toContain(`/api/control/v1/users/${userId}/sessions`)
  })
})
