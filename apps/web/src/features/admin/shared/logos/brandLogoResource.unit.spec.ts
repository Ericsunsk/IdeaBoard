import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiResponseAdminBrandLogoOverrideList } from './brandLogoResource'
import { getAdminBrandLogoOverride, replaceAdminBrandLogoOverrides } from './brandLogos'
import { createAdminBrandLogoResource, hydrateAdminBrandLogoOverrides } from './brandLogoResource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

type QueryCache = {
  cancelQueries: ReturnType<typeof vi.fn>
  getQueryData: ReturnType<typeof vi.fn>
  setQueryData: ReturnType<typeof vi.fn>
  value?: ApiResponseAdminBrandLogoOverrideList | undefined
}

function createQueryCache(initial?: ApiResponseAdminBrandLogoOverrideList): QueryCache {
  const cache: QueryCache = {
    value: initial,
    cancelQueries: vi.fn().mockResolvedValue(undefined),
    getQueryData: vi.fn(() => cache.value),
    setQueryData: vi.fn((_key, value) => {
      cache.value = value
    }),
  }
  return cache
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

describe('adminBrandLogoResource', () => {
  beforeEach(() => {
    transport.request.mockReset()
    replaceAdminBrandLogoOverrides([])
  })

  it('hydrates the shared cache from the list query payload', () => {
    hydrateAdminBrandLogoOverrides([override('storage-profile', 'r2-main', 'minio')])

    expect(getAdminBrandLogoOverride('storage-profile', 'r2-main')).toBe('minio')
  })

  it('updates the Vue Query list and shared cache after a save', async () => {
    const initial = override('provider-endpoint', '1', 'flux')
    const saved = override('provider-endpoint', '2', 'minio')
    const cache = createQueryCache({ code: 0, msg: 'success', data: { overrides: [initial] } })
    transport.request.mockResolvedValue({ code: 0, msg: 'success', data: saved })

    await createAdminBrandLogoResource(cache as never).save('provider-endpoint', '2', 'minio')

    expect(cache.cancelQueries).toHaveBeenCalledWith({
      queryKey: ['admin', 'brand-logo-overrides'],
    })
    expect(cache.value?.data?.overrides).toEqual([initial, saved])
    expect(getAdminBrandLogoOverride('provider-endpoint', '1')).toBe('flux')
    expect(getAdminBrandLogoOverride('provider-endpoint', '2')).toBe('minio')
  })

  it('removes the override from the Vue Query list and shared cache on reset', async () => {
    const existing = override('storage-profile', 'r2-main', 'minio')
    const cache = createQueryCache({ code: 0, msg: 'success', data: { overrides: [existing] } })
    hydrateAdminBrandLogoOverrides([existing])
    transport.request.mockResolvedValue({ code: 0, msg: 'success' })

    await createAdminBrandLogoResource(cache as never).remove('storage-profile', 'r2-main')

    expect(cache.value?.data?.overrides).toEqual([])
    expect(getAdminBrandLogoOverride('storage-profile', 'r2-main')).toBeUndefined()
  })

  it('does not mutate either cache when the server mutation fails', async () => {
    const existing = override('provider-endpoint', '1', 'flux')
    const cache = createQueryCache({ code: 0, msg: 'success', data: { overrides: [existing] } })
    hydrateAdminBrandLogoOverrides([existing])
    transport.request.mockRejectedValue(new Error('offline'))

    await expect(
      createAdminBrandLogoResource(cache as never).save('provider-endpoint', '2', 'minio'),
    ).rejects.toThrow('offline')

    expect(cache.value?.data?.overrides).toEqual([existing])
    expect(getAdminBrandLogoOverride('provider-endpoint', '1')).toBe('flux')
    expect(getAdminBrandLogoOverride('provider-endpoint', '2')).toBeUndefined()
  })
})
