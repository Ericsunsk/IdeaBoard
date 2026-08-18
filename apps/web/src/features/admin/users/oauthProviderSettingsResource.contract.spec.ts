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
  it('normalizes provider settings and capabilities through the generated API boundary', async () => {
    const api = {
      getSettings: vi.fn().mockResolvedValue(summaryEnvelope),
      updateSettings: vi.fn().mockResolvedValue(summaryEnvelope),
      getCapabilities: vi.fn().mockResolvedValue({
        code: 0,
        msg: 'success',
        data: { social: { google: true, discord: false } },
      }),
    }
    const resource = createOAuthProviderSettingsResource(api)

    await expect(resource.get('google')).resolves.toMatchObject({
      provider: 'google',
      clientId: 'google-client.apps.googleusercontent.com',
      clientSecretConfigured: true,
      callbackUrl: 'https://example.com/api/auth/callback/google',
    })
    await expect(resource.getCapabilities()).resolves.toEqual({
      social: { google: true, discord: false },
    })
    expect(api.getSettings).toHaveBeenCalledWith('google')
  })

  it('omits a blank Secret and forwards a nonblank Secret', async () => {
    const api = {
      getSettings: vi.fn().mockResolvedValue(summaryEnvelope),
      updateSettings: vi.fn().mockResolvedValue(summaryEnvelope),
      getCapabilities: vi.fn(),
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
      getCapabilities: vi.fn(),
    })

    await expect(resource.get('google')).rejects.toThrow('auth unavailable')
  })
})
