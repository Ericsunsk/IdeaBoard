import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '@/test/msw'
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

// Search providers use Control routes and contract envelopes. Mutation
// headers carry Idempotency-Key; adapters own bodies and error-code mapping.
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

  it('updates a provider through control with idempotency headers', async () => {
    let seen: { method: string; body: unknown; headers: Record<string, string> } | undefined
    useMswHandlers(
      http.put('/api/control/v1/platform/search/providers/unsplash', async ({ request }) => {
        seen = {
          method: request.method,
          body: await request.json(),
          headers: {
            idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
          },
        }
        return HttpResponse.json({ data: {} })
      }),
    )

    await createAdminSearchProviderResource().update('unsplash', {
      api_key: 'secret',
      enabled: false,
    })

    expect(seen?.method).toBe('PUT')
    expect(seen?.body).toEqual({ api_key: 'secret', enabled: false })
    expect(seen?.headers.idempotencyKey).toMatch(/.+/)
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
    await resource.update('unsplash', { enabled: true })
    await resource.update('unsplash', { enabled: true })

    expect(keys).toHaveLength(2)
    expect(keys[0]).toMatch(/.+/)
    expect(keys[0]).not.toBe(keys[1])
  })

  it('tests a provider through control', async () => {
    let seen: { method: string } | undefined
    useMswHandlers(
      http.post('/api/control/v1/platform/search/providers/pexels/test', ({ request }) => {
        seen = {
          method: request.method,
        }
        return HttpResponse.json({
          data: { provider: 'pexels', test_state: 'passed', message: 'ok' },
        })
      }),
    )

    const result = await createAdminSearchProviderResource().test('pexels')

    expect(seen?.method).toBe('POST')
    expect(result).toMatchObject({ provider: 'pexels', test_state: 'passed' })
  })

  it('sends update/test mutations with an idempotency key', async () => {
    const seen: Array<{ route: string; key: string | null }> = []
    useMswHandlers(
      http.put('/api/control/v1/platform/search/providers/unsplash', ({ request }) => {
        seen.push({
          route: 'update',
          key: request.headers.get('Idempotency-Key'),
        })
        return HttpResponse.json({ data: {} })
      }),
      http.post('/api/control/v1/platform/search/providers/pexels/test', ({ request }) => {
        seen.push({
          route: 'test',
          key: request.headers.get('Idempotency-Key'),
        })
        return HttpResponse.json({
          data: { provider: 'pexels', test_state: 'passed', message: 'ok' },
        })
      }),
    )
    const resource = createAdminSearchProviderResource()

    await resource.update('unsplash', { enabled: true })
    await resource.test('pexels')

    expect(seen).toHaveLength(2)
    expect(seen[0]?.key).toMatch(/.+/)
    expect(seen[1]?.key).toMatch(/.+/)
  })

  it('maps control error codes to operator copy', async () => {
    useMswHandlers(
      http.put('/api/control/v1/platform/search/providers/unsplash', () =>
        HttpResponse.json(
          { error: { code: 'control_not_found', message: 'missing', request_id: 'r' } },
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
    await expect(resource.update('unsplash', { enabled: true })).rejects.toThrow('missing')
    await expect(resource.test('pexels')).rejects.toThrow('操作已提交，请勿重复操作')
  })
})
