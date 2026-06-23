import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, unref, watch } from 'vue'
import type { MaybeRef } from 'vue'
import { uploadAdminAsset } from '../../../../shared/api/generated-query/admin-assets/admin-assets'
import {
  getGetAdminBrandingSettingQueryKey,
  useGetAdminBrandingSetting,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseBrandingSetting,
  BrandingSetting,
} from '../../../../shared/api/generated-query/model'
import {
  sanitizeApiErrorMessage,
  formatDisplayErrorMessage,
} from '../../../../shared/utils/errorMessages'
import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../../shared/utils/imageUploadSupport'
import { updateBrandingSetting } from '../queries/adminActions'

type AdminBrandingSettingsMessage = {
  error: (message: string) => void
  success: (message: string) => void
}

export const DEFAULT_ADMIN_BRANDING_SETTING: BrandingSetting = {
  project_name: 'IdeaBoard',
  logo_url: '',
}

export const ADMIN_BRANDING_PROJECT_NAME_MAX_CHARS = 40
export const ADMIN_BRANDING_LOGO_URL_MAX_LENGTH = 2048

export function useAdminBranding(options?: { enabled?: MaybeRef<boolean> }) {
  const isEnabled = computed(() =>
    options?.enabled === undefined ? true : Boolean(unref(options.enabled)),
  )
  const brandingQuery = useGetAdminBrandingSetting({
    query: {
      enabled: isEnabled,
    },
  })
  const branding = computed(() => normalizeAdminBrandingSetting(brandingQuery.data.value?.data))
  const projectName = computed(() => branding.value.project_name)
  const logoURL = computed(() => branding.value.logo_url)
  const fallbackText = computed(() => getAdminBrandFallbackText(projectName.value))

  return {
    branding,
    brandingQuery,
    fallbackText,
    logoURL,
    projectName,
  }
}

export function useAdminBrandingSettings(options: { message: AdminBrandingSettingsMessage }) {
  const queryClient = useQueryClient()
  const brandingQuery = useGetAdminBrandingSetting()
  const form = ref<BrandingSetting>({ ...DEFAULT_ADMIN_BRANDING_SETTING })
  const lastSyncedForm = ref<BrandingSetting>({ ...DEFAULT_ADMIN_BRANDING_SETTING })

  function invalidateBrandingQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminBrandingSettingQueryKey() })
  }

  function writeBrandingQueryCache(setting: BrandingSetting) {
    queryClient.setQueryData<ApiResponseBrandingSetting>(
      getGetAdminBrandingSettingQueryKey(),
      (previous) => ({
        code: previous?.code ?? 0,
        msg: previous?.msg ?? 'ok',
        data: setting,
      }),
    )
  }

  const saveBrandingMutation = useMutation({
    mutationFn: updateBrandingSetting,
    onSuccess: async (setting) => {
      const normalizedSetting = hydrateBrandingForm(setting)
      writeBrandingQueryCache(normalizedSetting)
      await invalidateBrandingQuery()
    },
  })

  const uploadLogoMutation = useMutation({
    mutationFn: uploadBrandingLogoFile,
    onSuccess: (logoURL) => {
      form.value.logo_url = logoURL
    },
  })

  const loading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  const saving = computed(() => saveBrandingMutation.isPending.value)
  const uploading = computed(() => uploadLogoMutation.isPending.value)
  const fallbackText = computed(() => getAdminBrandFallbackText(form.value.project_name))
  const formDirty = computed(() => !isSameBrandingSetting(form.value, lastSyncedForm.value))

  watch(
    () => brandingQuery.error.value,
    (error) => {
      if (error) {
        options.message.error(formatDisplayErrorMessage(error, '加载系统设置失败'))
      }
    },
  )

  watch(
    () => brandingQuery.data.value,
    (response) => {
      if (!response?.data || saveBrandingMutation.isPending.value || formDirty.value) return
      hydrateBrandingForm(response.data)
    },
    { immediate: true },
  )

  async function handleSaveBranding() {
    const validationError = validateAdminBrandingSetting(form.value)
    if (validationError) {
      options.message.error(validationError)
      return
    }

    try {
      await saveBrandingMutation.mutateAsync(normalizeAdminBrandingSetting(form.value))
      options.message.success('项目品牌已保存')
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '保存项目品牌失败'))
    }
  }

  async function handleUploadLogo(file: File) {
    try {
      await uploadLogoMutation.mutateAsync(file)
      options.message.success('Logo 已上传，保存后生效')
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '上传 Logo 失败'))
    }
  }

  function clearLogo() {
    form.value.logo_url = ''
  }

  function hydrateBrandingForm(setting: Partial<BrandingSetting> | null | undefined) {
    const normalizedSetting = normalizeAdminBrandingSetting(setting)
    lastSyncedForm.value = { ...normalizedSetting }
    form.value = { ...normalizedSetting }
    return normalizedSetting
  }

  return {
    clearLogo,
    fallbackText,
    form,
    handleSaveBranding,
    handleUploadLogo,
    loading,
    saving,
    uploading,
  }
}

function isSameBrandingSetting(current: BrandingSetting, synced: BrandingSetting): boolean {
  return current.project_name === synced.project_name && current.logo_url === synced.logo_url
}

export function normalizeAdminBrandingSetting(
  setting: Partial<BrandingSetting> | null | undefined,
): BrandingSetting {
  const projectName =
    typeof setting?.project_name === 'string' ? setting.project_name.trim() : ''
  const logoURL = typeof setting?.logo_url === 'string' ? setting.logo_url.trim() : ''
  return {
    project_name: projectName || DEFAULT_ADMIN_BRANDING_SETTING.project_name,
    logo_url: logoURL,
  }
}

export function validateAdminBrandingSetting(setting: BrandingSetting): string {
  const projectName = setting.project_name.trim()
  const logoURL = setting.logo_url.trim()
  if (!projectName) {
    return '项目名称不能为空'
  }
  if (Array.from(projectName).length > ADMIN_BRANDING_PROJECT_NAME_MAX_CHARS) {
    return `项目名称不能超过 ${ADMIN_BRANDING_PROJECT_NAME_MAX_CHARS} 个字符`
  }
  if (logoURL.length > ADMIN_BRANDING_LOGO_URL_MAX_LENGTH) {
    return `Logo 地址不能超过 ${ADMIN_BRANDING_LOGO_URL_MAX_LENGTH} 个字符`
  }
  if (!isValidAdminBrandingLogoURL(logoURL)) {
    return 'Logo 地址必须为空、/uploads/ 路径或 http(s) URL'
  }
  return ''
}

export function isValidAdminBrandingLogoURL(logoURL: string): boolean {
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

export function getAdminBrandFallbackText(projectName: string): string {
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

async function uploadBrandingLogoFile(file: File): Promise<string> {
  if (!isSupportedUploadImageFile(file)) {
    throw new Error(unsupportedUploadImageMessage())
  }

  const res = await uploadAdminAsset({ file })
  if (res.code !== 0 || !res.data?.url) {
    throw new Error(sanitizeApiErrorMessage(res.msg, '上传 Logo 失败'))
  }
  return res.data.url
}
