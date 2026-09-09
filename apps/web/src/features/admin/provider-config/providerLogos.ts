import { adminBrandLogoMap, getAdminBrandLogo } from '../shared/logos/brandLogos'

const searchProviderLogoMap: Record<string, string | undefined> = {
  unsplash: adminBrandLogoMap['unsplash'],
  pexels: adminBrandLogoMap['pexels'],
}

export function getSearchProviderLogo(input: {
  provider: string
  name?: string | undefined
  baseUrl?: string | undefined
}): string | undefined {
  return (
    getAdminBrandLogo(
      { name: input.name, baseUrl: input.baseUrl, provider: input.provider },
      'search-provider',
      input.provider,
    ) ?? searchProviderLogoMap[input.provider.toLowerCase()]
  )
}

export function getProviderEndpointLogo(
  endpoint: {
    name?: string
    baseUrl?: string
    apiType?: string
  },
  endpointId?: number | string,
): string | undefined {
  return getAdminBrandLogo(endpoint, 'provider-endpoint', endpointId)
}
