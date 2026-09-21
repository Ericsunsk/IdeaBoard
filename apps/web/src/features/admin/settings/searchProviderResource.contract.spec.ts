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

describe('searchProviderResource', () => {
  it('lists providers through the generated contract', async () => {
    useMswHandlers(
      http.get('/api/admin/search/providers', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: providersPayload }),
      ),
    )

    const list = await createAdminSearchProviderResource().list()

    expect(list.providers).toHaveLength(2)
    expect(list.providers[0]).toMatchObject({ provider: 'unsplash', enabled: false })
  })

  it('updates a provider with method and body from the generated contract', async () => {
    let method = ''
    let body: unknown = null
    useMswHandlers(
      http.put('/api/admin/search/providers/unsplash', async ({ request }) => {
        method = request.method
        body = await request.json()
        return HttpResponse.json({ code: 0, msg: 'ok' })
      }),
    )

    await createAdminSearchProviderResource().update('unsplash', {
      api_key: 'secret',
      enabled: false,
    })

    expect(method).toBe('PUT')
    expect(body).toEqual({ api_key: 'secret', enabled: false })
  })

  it('tests a provider through the generated contract', async () => {
    let method = ''
    useMswHandlers(
      http.post('/api/admin/search/providers/pexels/test', ({ request }) => {
        method = request.method
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { provider: 'pexels', test_state: 'passed', message: 'ok' },
        })
      }),
    )

    const result = await createAdminSearchProviderResource().test('pexels')

    expect(method).toBe('POST')
    expect(result).toMatchObject({ provider: 'pexels', test_state: 'passed' })
  })
})
