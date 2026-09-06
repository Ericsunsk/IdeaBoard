import { adminBrandLogoMap, getAdminBrandLogo } from '../shared/logos/adminBrandLogos'

const searchProviderLogoMap: Record<string, string> = {
  unsplash: adminBrandLogoMap['unsplash'],
  pexels: adminBrandLogoMap['pexels'],
}

export function getSearchProviderLogo(input: {
  provider: string
  name?: string
  baseUrl?: string
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
