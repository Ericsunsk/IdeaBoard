import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '../../../test/msw'
import { createAdminSearchProviderResource } from './searchProviderResource'

const providersPayload = {
  providers: [
    {
      provider: 'unsplash',
      api_key_configured: true,
      enabled: false,
      test_state: 'passed',
    },
    {
      provider: 'pexels',
      api_key_configured: false,
      enabled: false,
      test_state: 'never',
    },
  ],
}

// Search provider boundary contract (issue #809 follow-up): reads stay on
// the legacy admin route, while writes go through the Control Plane with a
// header-only Idempotency-Key and an operator reason. Control marks these
// routes `x-control-envelope: legacy`, so success bodies keep the legacy
// `{ code, msg, data? }` shape. This spec pins the adapter half: control
// URLs, headers, bodies, and control error-code branches.
describe('searchProviderResource', () => {
  it('lists providers through the legacy admin route', async () => {
    useMswHandlers(
      http.get('/api/admin/search/providers', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: providersPayload }),
      ),
    )

    const list = await createAdminSearchProviderResource().list()

    expect(list.providers).toHaveLength(2)
    expect(list.providers[0]).toMatchObject({ provider: 'unsplash', enabled: false })
  })

  it('updates a provider through control with reason and idempotency headers', async () => {
    let seen: { method: string; body: unknown; headers: Record<string, string> } | undefined
    useMswHandlers(
      http.put('/api/control/v1/platform/search/providers/unsplash', async ({ request }) => {
        seen = {
          method: request.method,
          body: await request.json(),
          headers: {
            idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
            operatorReason: request.headers.get('X-Operator-Reason') ?? '',
          },
        }
        return HttpResponse.json({ code: 0, msg: 'ok' })
      }),
    )

    await createAdminSearchProviderResource().update(
      'unsplash',
      { api_key: 'secret', enabled: false },
      { operatorReason: 'rotate leaked key' },
    )

    expect(seen?.method).toBe('PUT')
    expect(seen?.body).toEqual({ api_key: 'secret', enabled: false })
    expect(seen?.headers.idempotencyKey).toMatch(/.+/)
    expect(seen?.headers.operatorReason).toBe('rotate leaked key')
  })

  it('mints a fresh idempotency key per attempt', async () => {
    const keys: Array<string | null> = []
    useMswHandlers(
      http.put('/api/control/v1/platform/search/providers/unsplash', ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        return HttpResponse.json({ code: 0, msg: 'ok' })
      }),
    )

    const resource = createAdminSearchProviderResource()
    await resource.update('unsplash', { enabled: true }, { operatorReason: 'enable' })
    await resource.update('unsplash', { enabled: true }, { operatorReason: 'enable' })

    expect(keys).toHaveLength(2)
    expect(keys[0]).toMatch(/.+/)
    expect(keys[0]).not.toBe(keys[1])
  })

  it('tests a provider through control with an operator reason', async () => {
    let seen: { method: string; operatorReason: string } | undefined
    useMswHandlers(
      http.post('/api/control/v1/platform/search/providers/pexels/test', ({ request }) => {
        seen = { method: request.method, operatorReason: request.headers.get('X-Operator-Reason') ?? '' }
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { provider: 'pexels', test_state: 'passed', message: 'ok' },
        })
      }),
    )

    const result = await createAdminSearchProviderResource().test('pexels', {
      operatorReason: 'verify after rotation',
    })

    expect(seen?.method).toBe('POST')
    expect(seen?.operatorReason).toBe('verify after rotation')
    expect(result).toMatchObject({ provider: 'pexels', test_state: 'passed' })
  })

  it('refuses to send mutations without an operator reason', async () => {
    const resource = createAdminSearchProviderResource()

    await expect(
      resource.update('unsplash', { enabled: true }, { operatorReason: '   ' }),
    ).rejects.toThrow('需要填写操作原因')
    await expect(resource.test('pexels', { operatorReason: '' })).rejects.toThrow(
      '需要填写操作原因',
    )
  })

  it('maps control error codes to operator copy', async () => {
    useMswHandlers(
      http.put('/api/control/v1/platform/search/providers/unsplash', () =>
        HttpResponse.json(
          { error: { code: 'control_reason_required', message: 'no reason', request_id: 'r' } },
          { status: 400 },
        ),
      ),
      http.post('/api/control/v1/platform/search/providers/pexels/test', () =>
        HttpResponse.json(
          { error: { code: 'idempotency_conflict', message: 'reused', request_id: 'r' } },
          { status: 409 },
        ),
      ),
    )

    const resource = createAdminSearchProviderResource()
    await expect(
      resource.update('unsplash', { enabled: true }, { operatorReason: 'x' }),
    ).rejects.toThrow('需要填写操作原因')
    await expect(resource.test('pexels', { operatorReason: 'x' })).rejects.toThrow(
      '操作已提交，请勿重复操作',
    )
  })
})
