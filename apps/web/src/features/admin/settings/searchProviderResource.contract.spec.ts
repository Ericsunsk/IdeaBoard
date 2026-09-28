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

// Search provider boundary contract (final cut): reads moved to the Control
// Plane alongside the writes (header-only Idempotency-Key and an operator
// reason) with the unified contract envelope (`{ data }` success /
// `{ error: { code, message, request_id } }` errors). This spec pins the
// adapter half: control URLs, headers, bodies, and control error-code
// branches.
describe('searchProviderResource', () => {
  it('lists providers through the control plane route', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/search/providers', () =>
        HttpResponse.json({ data: providersPayload }),
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
        return HttpResponse.json({ data: {} })
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
        return HttpResponse.json({ data: {} })
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
        seen = {
          method: request.method,
          operatorReason: request.headers.get('X-Operator-Reason') ?? '',
        }
        return HttpResponse.json({
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
