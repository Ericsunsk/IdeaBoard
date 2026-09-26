import { describe, expect, it } from 'vitest'
import { createOAuthProviderSettingsResource } from './oauthProviderSettingsResource'
import { ADMIN_CONTROL_UNAVAILABLE_CODE } from '../shared/lib/controlGuidance'

// OAuth providers have no Control Plane equivalent: get/save fail closed
// with guidance instead of calling the deleted
// `/api/admin/users/oauth-providers/*` routes.
describe('OAuth Provider settings resource', () => {
  it('fails the settings read closed with guidance', async () => {
    const resource = createOAuthProviderSettingsResource()

    await expect(resource.get('google')).rejects.toMatchObject({
      code: ADMIN_CONTROL_UNAVAILABLE_CODE,
    })
  })

  it('fails the settings save closed with guidance', async () => {
    const resource = createOAuthProviderSettingsResource()

    await expect(
      resource.save('google', {
        enabled: true,
        clientId: 'google-client.apps.googleusercontent.com',
        clientSecret: 'google-secret',
      }),
    ).rejects.toMatchObject({ code: ADMIN_CONTROL_UNAVAILABLE_CODE })
  })
})
