import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
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
import {
  DEFAULT_SITE_BRANDING_SETTING,
  SITE_BRANDING_QUERY_KEY,
  getSiteBrandFallbackText,
  normalizeSiteBrandingSetting,
  resolveSiteBrandingLogoURL,
  validateSiteBrandingSetting,
  writeSiteBrandingQueryCache,
} from '../../../../shared/composables/useSiteBranding'
import { updateBrandingSetting } from '../queries/adminSettingsActions'

type AdminBrandingSettingsMessage = {
  error: (message: string) => void
  success: (message: string) => void
}

export function useAdminBrandingSettings(options: { message: AdminBrandingSettingsMessage }) {
  const queryClient = useQueryClient()
  const brandingQuery = useGetAdminBrandingSetting()
  const form = ref<BrandingSetting>({ ...DEFAULT_SITE_BRANDING_SETTING })
  const lastSyncedForm = ref<BrandingSetting>({ ...DEFAULT_SITE_BRANDING_SETTING })

  function invalidateBrandingQueries() {
    return Promise.all([
      queryClient.invalidateQueries({ queryKey: getGetAdminBrandingSettingQueryKey() }),
      queryClient.invalidateQueries({ queryKey: SITE_BRANDING_QUERY_KEY }),
    ])
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
    writeSiteBrandingQueryCache(queryClient, setting)
  }

  const saveBrandingMutation = useMutation({
    mutationFn: updateBrandingSetting,
    onSuccess: async (setting) => {
      const normalizedSetting = hydrateBrandingForm(setting)
      writeBrandingQueryCache(normalizedSetting)
      await invalidateBrandingQueries()
    },
  })

  const uploadLogoMutation = useMutation({
    mutationFn: uploadBrandingLogoFile,
    onSuccess: (logoURL) => {
      form.value.logo_url = normalizeSiteBrandingSetting({
        ...form.value,
        logo_url: logoURL,
      }).logo_url
    },
  })

  const loading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  const saving = computed(() => saveBrandingMutation.isPending.value)
  const uploading = computed(() => uploadLogoMutation.isPending.value)
  const fallbackText = computed(() => getSiteBrandFallbackText(form.value.project_name))
  const logoPreviewURL = computed(() => resolveSiteBrandingLogoURL(form.value.logo_url))
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
    const normalizedSetting = normalizeSiteBrandingSetting(form.value)
    const validationError = validateSiteBrandingSetting(normalizedSetting)
    if (validationError) {
      options.message.error(validationError)
      return
    }

    try {
      await saveBrandingMutation.mutateAsync(normalizedSetting)
      options.message.success('项目品牌已保存')
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '保存项目品牌失败'))
    }
  }

  async function handleUploadLogo(file: File) {
    try {
      await uploadLogoMutation.mutateAsync(file)
      options.message.success('Logo 已上传，保存后生效')
      return true
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '上传 Logo 失败'))
      return false
    }
  }

  function clearLogo() {
    form.value.logo_url = ''
  }

  function resetBrandingForm() {
    form.value = { ...lastSyncedForm.value }
  }

  function hydrateBrandingForm(setting: Partial<BrandingSetting> | null | undefined) {
    const normalizedSetting = normalizeSiteBrandingSetting(setting)
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
    logoPreviewURL,
    resetBrandingForm,
    saving,
    uploading,
  }
}

function isSameBrandingSetting(current: BrandingSetting, synced: BrandingSetting): boolean {
  return current.project_name === synced.project_name && current.logo_url === synced.logo_url
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
