import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import { uploadAdminAsset } from '../../../shared/api/generated-query/admin-assets/admin-assets'
import {
  getGetAdminBrandingSettingQueryKey,
  updateAdminBrandingSetting,
  useGetAdminBrandingSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { BrandingSetting } from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'
import { prepareImageAssetForUpload } from '../../../shared/media/imageAssetPipeline'
import type { AdminBrandingSettingsFeedback } from './useAdminSystemSettingsFeedback'
import {
  DEFAULT_SITE_BRANDING_SETTING,
  SITE_BRANDING_QUERY_KEY,
  getSiteBrandFallbackText,
  normalizeSiteBrandingSetting,
  resolveSiteBrandingLogoURL,
  validateSiteBrandingSetting,
  writeSiteBrandingQueryCache,
} from '../../../shared/composables/useSiteBranding'
import { useAdminSystemSettingsLifecycle } from './useAdminSystemSettingsLifecycle'
const selectAdminBrandingSetting = createApiDataSelector<BrandingSetting>('加载项目品牌失败')

export function useAdminBrandingSettings(options: { feedback: AdminBrandingSettingsFeedback }) {
  const queryClient = useQueryClient()
  const brandingQuery = useGetAdminBrandingSetting<BrandingSetting>({
    query: { select: selectAdminBrandingSetting },
  })
  const setting = computed(() => brandingQuery.data.value)
  const loading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: BrandingSetting) =>
      resolveApiData(updateAdminBrandingSetting(draft), '保存项目品牌失败'),
  })
  const saving = computed(() => saveMutation.isPending.value)
  const lifecycle = useAdminSystemSettingsLifecycle({
    defaultValue: DEFAULT_SITE_BRANDING_SETTING,
    source: setting,
    loadError: brandingQuery.error,
    loading,
    saving,
    normalize: normalizeSiteBrandingSetting,
    isSame: isSameBrandingSetting,
    validate: validateSiteBrandingSetting,
    save: (draft) => saveMutation.mutateAsync(draft),
    synchronizeCache: async (savedSetting) => {
      writeBrandingQueryCache(queryClient, savedSetting)
      writeSiteBrandingQueryCache(queryClient, savedSetting)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: getGetAdminBrandingSettingQueryKey() }),
        queryClient.invalidateQueries({ queryKey: SITE_BRANDING_QUERY_KEY }),
      ])
    },
    feedback: options.feedback,
  })

  const uploadLogoMutation = useMutation({
    mutationFn: uploadBrandingLogoFile,
    onSuccess: (logoURL) => {
      lifecycle.form.value.logo_url = normalizeSiteBrandingSetting({
        ...lifecycle.form.value,
        logo_url: logoURL,
      }).logo_url
    },
  })

  const uploading = computed(() => uploadLogoMutation.isPending.value)
  const fallbackText = computed(() => getSiteBrandFallbackText(lifecycle.form.value.project_name))
  const logoPreviewURL = computed(() => resolveSiteBrandingLogoURL(lifecycle.form.value.logo_url))

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
    lifecycle.form.value.logo_url = ''
  }

  return {
    clearLogo,
    fallbackText,
    form: lifecycle.form,
    handleUploadLogo,
    isDirty: lifecycle.isDirty,
    loading: lifecycle.loading,
    logoPreviewURL,
    reset: lifecycle.reset,
    save: lifecycle.save,
    saving: lifecycle.saving,
    uploading,
    validationError: lifecycle.validationError,
  }
}

function isSameBrandingSetting(current: BrandingSetting, synced: BrandingSetting): boolean {
  return current.project_name === synced.project_name && current.logo_url === synced.logo_url
}

async function uploadBrandingLogoFile(file: File): Promise<string> {
  const prepared = await prepareImageAssetForUpload(file, { compressionPolicy: 'none' })
  const data = await resolveApiData(
    uploadAdminAsset({ file: prepared.uploadFile }),
    '上传 Logo 失败',
  )
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
