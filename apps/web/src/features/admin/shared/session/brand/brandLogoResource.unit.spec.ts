import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getAdminBrandLogoOverride, replaceAdminBrandLogoOverrides } from './brandLogos'
import { createAdminBrandLogoResource, hydrateAdminBrandLogoOverrides } from './brandLogoResource'
import { ADMIN_CONTROL_UNAVAILABLE_CODE } from '../../core/controlGuidance'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../../api/client', () => ({
  api: { request: transport.request },
}))

function createQueryCache() {
  return {
    cancelQueries: vi.fn().mockResolvedValue(undefined),
  }
}

function override(
  scope: 'provider-endpoint' | 'storage-profile',
  targetKey: string,
  logoKey: 'flux' | 'minio',
) {
  return {
    scope,
    target_key: targetKey,
    logo_key: logoKey,
    created_at: '2026-08-23T00:00:00Z',
    updated_at: '2026-08-23T00:00:00Z',
  }
}

// Brand logo overrides have no Control Plane equivalent: save/remove fail
// closed with guidance (no remote call). Local hydration stays.
describe('adminBrandLogoResource', () => {
  beforeEach(() => {
    transport.request.mockReset()
    replaceAdminBrandLogoOverrides([])
  })

  it('hydrates the shared cache from the list query payload', () => {
    hydrateAdminBrandLogoOverrides([override('storage-profile', 'r2-main', 'minio')])

    expect(getAdminBrandLogoOverride('storage-profile', 'r2-main')).toBe('minio')
  })

  it('fails saves closed with guidance and no transport call', async () => {
    const cache = createQueryCache()

    await expect(
      createAdminBrandLogoResource(cache as never).save('provider-endpoint', '2', 'minio'),
    ).rejects.toMatchObject({ code: ADMIN_CONTROL_UNAVAILABLE_CODE })

    expect(cache.cancelQueries).toHaveBeenCalledWith({
      queryKey: ['admin', 'brand-logo-overrides'],
    })
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('fails removes closed with guidance and no transport call', async () => {
    const cache = createQueryCache()

    await expect(
      createAdminBrandLogoResource(cache as never).remove('storage-profile', 'r2-main'),
    ).rejects.toMatchObject({ code: ADMIN_CONTROL_UNAVAILABLE_CODE })

    expect(transport.request).not.toHaveBeenCalled()
  })
})
