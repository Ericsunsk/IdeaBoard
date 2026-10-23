import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import {
  banAdminUser,
  deleteAdminUser,
  revokeAdminUserSessions,
  unbanAdminUser,
  updateAdminUserRole,
} from './api'

// User mutations use Control routes, a header-only Idempotency-Key and
// contract envelopes. Generated clients own transport; adapters own request
// headers, bodies and error-code mapping.
describe('admin users control api boundary', () => {
  const userId = 'usr_abc123'

  it('sends ban duration with idempotency headers', async () => {
    let seen: { headers: Record<string, string>; body: unknown } | undefined
    useMswHandlers(
      http.post('/api/control/v1/users/:userId/ban', async ({ request }) => {
        seen = {
          headers: {
            idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
          },
          body: await request.json(),
        }
        return HttpResponse.json({ data: { banned: true } })
      }),
    )

    await banAdminUser({
      publicId: userId,

      expires_in_days: 7,

      idempotencyKey: 'control-ui:test-ban',
    })

    expect(seen?.body).toEqual({ expires_in_days: 7 })
    expect(seen?.headers.idempotencyKey).toBe('control-ui:test-ban')
  })

  it('mints idempotency keys per attempt for user lifecycle mutations', async () => {
    const keys: Array<string | null> = []
    useMswHandlers(
      http.post('/api/control/v1/users/:userId/unban', ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        return HttpResponse.json({ data: { unbanned: true } })
      }),
    )

    await unbanAdminUser(userId)
    await unbanAdminUser(userId)

    expect(keys).toHaveLength(2)
    expect(keys[0]).toMatch(/^control:users\.unban:/)
    expect(keys[0]).not.toBe(keys[1])
  })

  it('maps control error codes to operator copy', async () => {
    useMswHandlers(
      http.post('/api/control/v1/users/:userId/unban', () =>
        HttpResponse.json(
          { error: { code: 'control_not_found', message: 'missing', request_id: 'r' } },
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

    await expect(unbanAdminUser(userId)).rejects.toThrow('用户不存在')
    await expect(deleteAdminUser(userId)).rejects.toThrow('操作已提交，请勿重复操作')
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

    await updateAdminUserRole({ publicId: userId, role: 'admin' })
    await revokeAdminUserSessions(userId)

    expect(calls).toHaveLength(2)
    expect(calls[0]).toMatchObject({ method: 'PUT', body: { role: 'admin' } })
    expect(calls[0]?.url).toContain(`/api/control/v1/users/${userId}/role`)
    expect(calls[1]?.url).toContain(`/api/control/v1/users/${userId}/sessions`)
  })
})
