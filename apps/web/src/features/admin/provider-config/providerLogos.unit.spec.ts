import { afterEach, describe, expect, it } from 'vitest'
import { adminBrandLogoMap, replaceAdminBrandLogoOverrides } from '../shared/logos/adminBrandLogos'
import { getProviderEndpointLogo, getSearchProviderLogo } from './providerLogos'

describe('providerLogos', () => {
  afterEach(() => replaceAdminBrandLogoOverrides([]))

  it('resolves official search provider logos', () => {
    expect(getSearchProviderLogo({ provider: 'unsplash' })).toBeDefined()
    expect(getSearchProviderLogo({ provider: 'pexels' })).toBeDefined()
    expect(getSearchProviderLogo({ provider: 'unknown' })).toBeUndefined()
  })

  it('reads Search Provider overrides from the search-provider scope', () => {
    replaceAdminBrandLogoOverrides([
      { scope: 'search-provider', targetKey: 'unsplash', logoKey: 'minio' },
    ])

    expect(getSearchProviderLogo({ provider: 'unsplash' })).toBe(adminBrandLogoMap.minio)
  })

  it('applies endpoint-scoped overrides before automatic detection', () => {
    replaceAdminBrandLogoOverrides([
      { scope: 'provider-endpoint', targetKey: '42', logoKey: 'minio' },
    ])

    expect(getProviderEndpointLogo({ name: 'Flux' }, 42)).toBe(adminBrandLogoMap.minio)
    expect(getProviderEndpointLogo({ name: 'Flux' })).toBeDefined()
    expect(getProviderEndpointLogo({ name: 'Custom endpoint' })).toBeUndefined()
  })
})
