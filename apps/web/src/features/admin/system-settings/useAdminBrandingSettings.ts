import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'
import { uploadAdminAsset } from '../../../shared/api/generated-query/admin-assets/admin-assets'
import {
  getGetAdminBrandingSettingQueryKey,
  updateAdminBrandingSetting,
  useGetAdminBrandingSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { BrandingSetting } from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'
import type { AdminBrandingSettingsFeedback } from './useAdminSystemSettingsFeedback'
import {
  isSupportedUploadImageFile,
  unsupportedUploadImageMessage,
} from '../../../shared/utils/imageUploadSupport'
import {
  DEFAULT_SITE_BRANDING_SETTING,
  SITE_BRANDING_QUERY_KEY,
  getSiteBrandFallbackText,
  normalizeSiteBrandingSetting,
  resolveSiteBrandingLogoURL,
  validateSiteBrandingSetting,
  writeSiteBrandingQueryCache,
} from '../../../shared/composables/useSiteBranding'
const selectAdminBrandingSetting = createApiDataSelector<BrandingSetting>('加载项目品牌失败')

export function useAdminBrandingSettings(options: { feedback: AdminBrandingSettingsFeedback }) {
  const queryClient = useQueryClient()
  const brandingQuery = useGetAdminBrandingSetting<BrandingSetting>({
    query: { select: selectAdminBrandingSetting },
  })
  const setting = computed(() => brandingQuery.data.value)
  const form = ref<BrandingSetting>(
    cloneSetting(DEFAULT_SITE_BRANDING_SETTING),
  ) as Ref<BrandingSetting>
  const lastSyncedForm = ref<BrandingSetting>(
    cloneSetting(DEFAULT_SITE_BRANDING_SETTING),
  ) as Ref<BrandingSetting>
  const validationError = ref('')
  const loading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: BrandingSetting) =>
      resolveApiData(updateAdminBrandingSetting(draft), '保存项目品牌失败'),
    onSuccess: async (savedSetting) => {
      hydrate(savedSetting)
      writeBrandingQueryCache(queryClient, savedSetting)
      writeSiteBrandingQueryCache(queryClient, savedSetting)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: getGetAdminBrandingSettingQueryKey() }),
        queryClient.invalidateQueries({ queryKey: SITE_BRANDING_QUERY_KEY }),
      ])
      if (brandingQuery.data.value) hydrate(brandingQuery.data.value)
    },
  })
  const saving = computed(() => saveMutation.isPending.value)
  const isDirty = computed(() => !isSameBrandingSetting(form.value, lastSyncedForm.value))

  const uploadLogoMutation = useMutation({
    mutationFn: uploadBrandingLogoFile,
    onSuccess: (logoURL) => {
      form.value.logo_url = normalizeSiteBrandingSetting({
        ...form.value,
        logo_url: logoURL,
      }).logo_url
    },
  })

  const uploading = computed(() => uploadLogoMutation.isPending.value)
  const fallbackText = computed(() => getSiteBrandFallbackText(form.value.project_name))
  const logoPreviewURL = computed(() => resolveSiteBrandingLogoURL(form.value.logo_url))

  function hydrate(savedSetting: Partial<BrandingSetting> | null | undefined) {
    const nextForm = normalizeSiteBrandingSetting(savedSetting)
    lastSyncedForm.value = cloneSetting(nextForm)
    form.value = cloneSetting(nextForm)
    validationError.value = ''
  }

  async function save(): Promise<boolean> {
    const normalizedDraft = normalizeSiteBrandingSetting(form.value)
    const validationMessage = validateSiteBrandingSetting(normalizedDraft)
    if (validationMessage) {
      validationError.value = validationMessage
      options.feedback.validationFailed(validationMessage)
      return false
    }
    try {
      await saveMutation.mutateAsync(normalizedDraft)
      options.feedback.saved()
      return true
    } catch (error) {
      options.feedback.saveFailed(error)
      return false
    }
  }

  async function handleUploadLogo(file: File) {
    try {
      await uploadLogoMutation.mutateAsync(file)
      options.feedback.logoUploaded()
      return true
    } catch (error) {
      options.feedback.logoUploadFailed(error)
      return false
    }
  }

  function clearLogo() {
    form.value.logo_url = ''
  }

  function reset() {
    form.value = cloneSetting(lastSyncedForm.value)
    validationError.value = ''
  }

  watch(form, () => (validationError.value = ''), { deep: true, flush: 'sync' })
  watch(
    () => brandingQuery.error.value,
    (error) => {
      if (error) options.feedback.loadFailed(error)
    },
  )
  watch(
    setting,
    (savedSetting) => {
      if (!savedSetting || saving.value || isDirty.value) return
      hydrate(savedSetting)
    },
    { immediate: true },
  )

  return {
    clearLogo,
    fallbackText,
    form,
    handleUploadLogo,
    isDirty,
    loading,
    logoPreviewURL,
    reset,
    save,
    saving,
    uploading,
    validationError,
  }
}

function isSameBrandingSetting(current: BrandingSetting, synced: BrandingSetting): boolean {
  return current.project_name === synced.project_name && current.logo_url === synced.logo_url
}

async function uploadBrandingLogoFile(file: File): Promise<string> {
  if (!isSupportedUploadImageFile(file)) {
    throw new Error(unsupportedUploadImageMessage())
  }

  const data = await resolveApiData(uploadAdminAsset({ file }), '上传 Logo 失败')
  if (!data.url) throw new Error('上传 Logo 失败')
  return data.url
}

function writeBrandingQueryCache(queryClient: QueryClient, setting: BrandingSetting) {
  queryClient.setQueryData(getGetAdminBrandingSettingQueryKey(), {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}

function cloneSetting(setting: BrandingSetting): BrandingSetting {
  return structuredClone(toRaw(setting))
}
