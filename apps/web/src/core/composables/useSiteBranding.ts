import type { QueryClient } from '@tanstack/vue-query'
import { useQuery } from '@tanstack/vue-query'
import { computed, unref } from 'vue'
import type { MaybeRef } from 'vue'
import {
  createSiteBrandingSelector,
  fetchSiteBrandingEnvelope,
  fetchSiteBrandingSetting,
  SITE_BRANDING_QUERY_KEY,
  type ApiResponseBrandingSetting,
  type BrandingSetting,
} from '@/api/siteBranding'
import { canonicalizeRuntimeUploadRef, resolveAssetUrl } from '../config/runtimeUrls'

export type { ApiResponseBrandingSetting, BrandingSetting }
export { SITE_BRANDING_QUERY_KEY }

export const DEFAULT_SITE_BRANDING_SETTING: BrandingSetting = {
  project_name: 'IdeaBoard',
  logo_url: '',
}

export const SITE_BRANDING_PROJECT_NAME_MAX_CHARS = 40
export const SITE_BRANDING_LOGO_URL_MAX_LENGTH = 2048
export const SITE_BRANDING_LOGO_ROUTE_PATH = '/api/v1/public/branding/logo'
export const SITE_BRANDING_LOGO_STORAGE_KEY_PREFIX = 'uploads/site/assets/'
export type SiteBrandingOptions = {
  enabled?: MaybeRef<boolean>
  /** Caller-supplied load-failure copy; shared owns no feature locale keys. */
  fallback: string
}

export async function fetchSiteBranding(fallback: string): Promise<BrandingSetting> {
  return normalizeSiteBrandingSetting(await fetchSiteBrandingSetting(fallback))
}

export function useSiteBranding(options: SiteBrandingOptions) {
  const isEnabled = computed(() =>
    options?.enabled === undefined ? true : Boolean(unref(options.enabled)),
  )
  const brandingQuery = useQuery({
    queryKey: SITE_BRANDING_QUERY_KEY,
    queryFn: ({ signal }) => fetchSiteBrandingEnvelope(signal),
    enabled: isEnabled,
    select: createSiteBrandingSelector(options.fallback),
  })
  const branding = computed(() => normalizeSiteBrandingSetting(brandingQuery.data.value))
  const projectName = computed(() => branding.value.project_name)
  const logoURL = computed(() => resolveSiteBrandingLogoURL(branding.value.logo_url))
  const fallbackText = computed(() => getSiteBrandFallbackText(projectName.value))

  return {
    branding,
    brandingQuery,
    fallbackText,
    logoURL,
    projectName,
  }
}

export function writeSiteBrandingQueryCache(
  queryClient: QueryClient,
  setting: Partial<BrandingSetting> | null | undefined,
) {
  const normalizedSetting = normalizeSiteBrandingSetting(setting)
  queryClient.setQueryData<ApiResponseBrandingSetting>(SITE_BRANDING_QUERY_KEY, {
    data: normalizedSetting,
  })
}

export function normalizeSiteBrandingSetting(
  setting: Partial<BrandingSetting> | null | undefined,
): BrandingSetting {
  const projectName = typeof setting?.project_name === 'string' ? setting.project_name.trim() : ''
  const rawLogoURL = typeof setting?.logo_url === 'string' ? setting.logo_url.trim() : ''
  const logoURL = rawLogoURL ? canonicalizeRuntimeUploadRef(rawLogoURL) : ''
  return {
    project_name: projectName || DEFAULT_SITE_BRANDING_SETTING.project_name,
    logo_url: logoURL,
  }
}

export function resolveSiteBrandingLogoURL(logoURL: string): string {
  const trimmed = logoURL.trim()
  if (isSiteBrandingLogoStorageKey(trimmed)) {
    return `${SITE_BRANDING_LOGO_ROUTE_PATH}?key=${encodeURIComponent(trimmed)}`
  }
  return resolveAssetUrl(trimmed)
}

export type SiteBrandingValidationError =
  'nameRequired' | 'nameTooLong' | 'logoTooLong' | 'logoInvalid'

export function validateSiteBrandingSetting(
  setting: BrandingSetting,
): SiteBrandingValidationError | '' {
  const projectName = setting.project_name.trim()
  const logoURL = setting.logo_url.trim()
  if (!projectName) {
    return 'nameRequired'
  }
  if (Array.from(projectName).length > SITE_BRANDING_PROJECT_NAME_MAX_CHARS) {
    return 'nameTooLong'
  }
  if (logoURL.length > SITE_BRANDING_LOGO_URL_MAX_LENGTH) {
    return 'logoTooLong'
  }
  if (!isValidSiteBrandingLogoURL(logoURL)) {
    return 'logoInvalid'
  }
  return ''
}

export function isValidSiteBrandingLogoURL(logoURL: string): boolean {
  const value = logoURL.trim()
  if (!value) return true
  if (isSiteBrandingLogoStorageKey(value)) return true
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function isSiteBrandingLogoStorageKey(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed.startsWith(SITE_BRANDING_LOGO_STORAGE_KEY_PREFIX)) return false
  if (trimmed.includes('..') || trimmed.includes('\\')) return false
  return trimmed.split('/').every((segment) => segment !== '' && segment !== '.')
}

export function getSiteBrandFallbackText(projectName: string): string {
  const trimmed = projectName.trim()
  if (!trimmed) return 'IB'

  const upperChars = Array.from(trimmed).filter((char) => /[A-Z0-9]/.test(char))
  if (upperChars.length >= 2) return upperChars.slice(0, 2).join('')

  const words = trimmed.split(/[\s._-]+/).filter(Boolean)
  if (words.length >= 2) {
    return words
      .slice(0, 2)
      .map((word) => Array.from(word)[0] || '')
      .join('')
      .toUpperCase()
  }

  return Array.from(trimmed).slice(0, 2).join('').toUpperCase()
}
