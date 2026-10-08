export const EXTERNAL_IMAGE_PROVIDERS = ['unsplash', 'pexels'] as const

export type ExternalImageProvider = (typeof EXTERNAL_IMAGE_PROVIDERS)[number]

export const EXTERNAL_IMAGE_PROVIDER_LABELS: Readonly<Record<ExternalImageProvider, string>> = {
  unsplash: 'Unsplash',
  pexels: 'Pexels',
}

export function isExternalImageProvider(value: unknown): value is ExternalImageProvider {
  return EXTERNAL_IMAGE_PROVIDERS.some((provider) => provider === value)
}
