import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import { api } from '../../../shared/api/client'
import {
  createApiDataSelector,
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import { prepareImageAssetForUpload } from '../../../shared/media/imageAssetPipeline'
import type { AdminBrandingSettingsFeedback } from './useSettingsFeedback'
import {
  DEFAULT_SITE_BRANDING_SETTING,
  SITE_BRANDING_QUERY_KEY,
  getSiteBrandFallbackText,
  normalizeSiteBrandingSetting,
  resolveSiteBrandingLogoURL,
  validateSiteBrandingSetting,
  writeSiteBrandingQueryCache,
  type BrandingSetting,
} from '../../../shared/composables/useSiteBranding'
import { useSettingsLifecycle } from './useSettingsLifecycle'

// Admin branding boundary (issue #809). The generated `admin-settings` and
// `admin-assets` clients are gone but the backend admin plane still serves
// `/api/admin/settings/branding` and `/api/admin/upload` with the shared
// envelope, so this composable transports through the central `api.request`.
const ADMIN_BRANDING_QUERY_KEY = ['admin', 'settings', 'branding'] as const
const selectAdminBrandingSetting = createApiDataSelector<BrandingSetting>('加载项目品牌失败')

export function useBrandingSettings(options: { feedback: AdminBrandingSettingsFeedback }) {
  const queryClient = useQueryClient()
  const brandingQuery = useQuery<
    ApiResponseWithData<BrandingSetting>,
    Error,
    BrandingSetting
  >({
    queryKey: ADMIN_BRANDING_QUERY_KEY,
    queryFn: ({ signal }) =>
      api.request<ApiResponseWithData<BrandingSetting>>('/admin/settings/branding', {
        method: 'GET',
        ...(signal == null ? {} : { signal }),
      }),
    select: selectAdminBrandingSetting,
  })
  const setting = computed(() => brandingQuery.data.value)
  const loading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: BrandingSetting) =>
      resolveApiData(
        api.request<ApiResponseWithData<BrandingSetting>>('/admin/settings/branding', {
          method: 'PUT',
          body: draft as unknown as Record<string, unknown>,
        }),
        '保存项目品牌失败',
      ),
  })
  const saving = computed(() => saveMutation.isPending.value)
  const lifecycle = useSettingsLifecycle({
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
        queryClient.invalidateQueries({ queryKey: ADMIN_BRANDING_QUERY_KEY }),
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
  const formData = new FormData()
  formData.append('file', prepared.uploadFile)
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ url: string }>>('/admin/upload', {
      method: 'POST',
      body: formData,
    }),
    '上传 Logo 失败',
  )
  if (!data.url) throw new Error('上传 Logo 失败')
  return data.url
}

function writeBrandingQueryCache(queryClient: QueryClient, setting: BrandingSetting) {
  queryClient.setQueryData(ADMIN_BRANDING_QUERY_KEY, {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}
