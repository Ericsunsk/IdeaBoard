import { describe, expect, it, vi } from 'vitest'
import { createOAuthProviderSettingsResource } from './oauthProviderSettingsResource'

const summaryEnvelope = {
  code: 0,
  msg: 'success',
  data: {
    provider: 'google' as const,
    enabled: true,
    configured: true,
    available: true,
    client_id: 'google-client.apps.googleusercontent.com',
    client_secret_configured: true,
    scopes: ['openid', 'email', 'profile'],
    config_version: 2,
    callback_url: 'https://example.com/api/auth/callback/google',
    updated_at: '2026-08-06T00:00:00Z',
  },
}

describe('OAuth Provider settings resource', () => {
  it('normalizes provider settings through the generated API boundary', async () => {
    const api = {
      getSettings: vi.fn().mockResolvedValue(summaryEnvelope),
      updateSettings: vi.fn().mockResolvedValue(summaryEnvelope),
    }
    const resource = createOAuthProviderSettingsResource(api)

    await expect(resource.get('google')).resolves.toMatchObject({
      clientId: 'google-client.apps.googleusercontent.com',
      clientSecretConfigured: true,
      callbackUrl: 'https://example.com/api/auth/callback/google',
    })
    expect(api.getSettings).toHaveBeenCalledWith('google')
  })

  it('omits a blank Secret and forwards a nonblank Secret', async () => {
    const api = {
      getSettings: vi.fn().mockResolvedValue(summaryEnvelope),
      updateSettings: vi.fn().mockResolvedValue(summaryEnvelope),
    }
    const resource = createOAuthProviderSettingsResource(api)

    await resource.save('google', {
      enabled: true,
      clientId: 'google-client.apps.googleusercontent.com',
      clientSecret: '   ',
    })
    expect(api.updateSettings).toHaveBeenLastCalledWith('google', {
      enabled: true,
      client_id: 'google-client.apps.googleusercontent.com',
    })

    await resource.save('google', {
      enabled: true,
      clientId: 'google-client.apps.googleusercontent.com',
      clientSecret: 'google-secret',
    })
    expect(api.updateSettings).toHaveBeenLastCalledWith('google', {
      enabled: true,
      client_id: 'google-client.apps.googleusercontent.com',
      client_secret: 'google-secret',
    })
  })

  it('propagates API envelope errors with operation context', async () => {
    const resource = createOAuthProviderSettingsResource({
      getSettings: vi
        .fn()
        .mockResolvedValue({ code: 502, msg: 'auth unavailable', message_safe: true }),
      updateSettings: vi.fn(),
    })

    await expect(resource.get('google')).rejects.toThrow('auth unavailable')
  })
})
