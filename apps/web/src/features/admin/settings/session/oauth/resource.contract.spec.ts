import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { createOAuthProviderSettingsResource } from './resource'

const CONTROL_SUMMARY = {
  provider: 'google',
  enabled: true,
  configured: true,
  available: true,
  client_id: 'google-client.apps.googleusercontent.com',
  client_secret_configured: true,
  scopes: ['openid', 'email'],
  config_version: 3,
  callback_url: 'https://example.com/api/auth/callback/google',
  updated_at: null,
}

const VIEW_SUMMARY = {
  enabled: true,
  configured: true,
  clientId: 'google-client.apps.googleusercontent.com',
  clientSecretConfigured: true,
  callbackUrl: 'https://example.com/api/auth/callback/google',
}

// OAuth providers are wired to the Control OAuth-provider family
// (GET/PUT /users/oauth/:provider behind control:users).
describe('OAuth Provider settings resource', () => {
  it('reads a provider summary from Control Plane', async () => {
    useMswHandlers(
      http.get('/api/control/v1/users/oauth/google', () =>
        HttpResponse.json({ data: CONTROL_SUMMARY }),
      ),
    )

    expect(await createOAuthProviderSettingsResource().get('google')).toEqual(VIEW_SUMMARY)
  })

  it('saves with audit headers and maps the returned summary', async () => {
    let body: unknown
    let key: string | null = null
    useMswHandlers(
      http.put('/api/control/v1/users/oauth/google', async ({ request }) => {
        body = await request.json()
        key = request.headers.get('Idempotency-Key')
        return HttpResponse.json({ data: { ...CONTROL_SUMMARY, enabled: false } })
      }),
    )

    const saved = await createOAuthProviderSettingsResource().save('google', {
      enabled: false,
      clientId: 'google-client.apps.googleusercontent.com',
      clientSecret: '',
    })
    expect(saved).toEqual({ ...VIEW_SUMMARY, enabled: false })
    expect(body).toMatchObject({
      enabled: false,
      client_id: 'google-client.apps.googleusercontent.com',
      client_secret: '',
    })
    expect(key).toBeTruthy()
  })

  it('surfaces control errors with copy', async () => {
    useMswHandlers(
      http.get('/api/control/v1/users/oauth/google', () => HttpResponse.json({ data: null })),
    )
    await expect(createOAuthProviderSettingsResource().get('google')).rejects.toThrow(
      '加载 OAuth Provider 设置失败，请重试',
    )
  })
})
