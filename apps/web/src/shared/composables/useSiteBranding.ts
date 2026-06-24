import type { QueryClient } from '@tanstack/vue-query'
import { computed, unref } from 'vue'
import type { MaybeRef } from 'vue'
import {
  getGetSiteBrandingSettingQueryKey,
  getSiteBrandingSetting,
  useGetSiteBrandingSetting,
} from '../api/generated-query/settings/settings'
import type { ApiResponseBrandingSetting, BrandingSetting } from '../api/generated-query/model'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'

export const SITE_BRANDING_QUERY_KEY = getGetSiteBrandingSettingQueryKey()

export const DEFAULT_SITE_BRANDING_SETTING: BrandingSetting = {
  project_name: 'IdeaBoard',
  logo_url: '',
}

export const SITE_BRANDING_PROJECT_NAME_MAX_CHARS = 40
export const SITE_BRANDING_LOGO_URL_MAX_LENGTH = 2048

export async function fetchSiteBranding(): Promise<BrandingSetting> {
  const res = await getSiteBrandingSetting()
  if (res.code !== 0 || !res.data) {
    throw new Error(sanitizeApiErrorMessage(res.msg, '读取站点设置失败'))
  }
  return normalizeSiteBrandingSetting(res.data)
}

export function useSiteBranding(options?: { enabled?: MaybeRef<boolean> }) {
  const isEnabled = computed(() =>
    options?.enabled === undefined ? true : Boolean(unref(options.enabled)),
  )
  const brandingQuery = useGetSiteBrandingSetting({
    query: {
      enabled: isEnabled,
    },
  })
  const branding = computed(() => normalizeSiteBrandingSetting(brandingQuery.data.value?.data))
  const projectName = computed(() => branding.value.project_name)
  const logoURL = computed(() => branding.value.logo_url)
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
  queryClient.setQueryData<ApiResponseBrandingSetting>(SITE_BRANDING_QUERY_KEY, (previous) => ({
    code: previous?.code ?? 0,
    msg: previous?.msg ?? 'ok',
    data: normalizedSetting,
  }))
}

export function normalizeSiteBrandingSetting(
  setting: Partial<BrandingSetting> | null | undefined,
): BrandingSetting {
  const projectName = typeof setting?.project_name === 'string' ? setting.project_name.trim() : ''
  const logoURL = typeof setting?.logo_url === 'string' ? setting.logo_url.trim() : ''
  return {
    project_name: projectName || DEFAULT_SITE_BRANDING_SETTING.project_name,
    logo_url: logoURL,
  }
}

export function validateSiteBrandingSetting(setting: BrandingSetting): string {
  const projectName = setting.project_name.trim()
  const logoURL = setting.logo_url.trim()
  if (!projectName) {
    return '项目名称不能为空'
  }
  if (Array.from(projectName).length > SITE_BRANDING_PROJECT_NAME_MAX_CHARS) {
    return `项目名称不能超过 ${SITE_BRANDING_PROJECT_NAME_MAX_CHARS} 个字符`
  }
  if (logoURL.length > SITE_BRANDING_LOGO_URL_MAX_LENGTH) {
    return `Logo 地址不能超过 ${SITE_BRANDING_LOGO_URL_MAX_LENGTH} 个字符`
  }
  if (!isValidSiteBrandingLogoURL(logoURL)) {
    return 'Logo 地址必须为空、/uploads/ 路径或 http(s) URL'
  }
  return ''
}

export function isValidSiteBrandingLogoURL(logoURL: string): boolean {
  const value = logoURL.trim()
  if (!value) return true
  if (value.startsWith('/uploads/')) return !value.includes('..') && !value.includes('\\')
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
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
