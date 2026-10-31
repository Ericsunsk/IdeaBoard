import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  getControlBrandingSetting,
  updateControlBrandingSetting,
  uploadControlBrandingLogo,
} from '@/api/generated-query/control/control-platform/control-platform'
import type { ControlBrandingLogoUpload } from '@/api/generated-query/control/model/controlBrandingLogoUpload'
import { requireContractData } from '@/api/http/contractKernel'
import { buildControlMutationHeaders } from '../../../shared/index'
import type { AdminBrandingSettingsFeedback } from '../useSettingsFeedback'
import {
  DEFAULT_SITE_BRANDING_SETTING,
  SITE_BRANDING_QUERY_KEY,
  getSiteBrandFallbackText,
  normalizeSiteBrandingSetting,
  resolveSiteBrandingLogoURL,
  validateSiteBrandingSetting,
  writeSiteBrandingQueryCache,
  type BrandingSetting,
} from '@/core/composables/useSiteBranding'
import { useSettingsLifecycle } from '../useSettingsLifecycle'
import { withControlCopy } from '../../../shared/index'

const ADMIN_BRANDING_QUERY_KEY = ['admin', 'settings', 'branding'] as const

export function useBrandingSettings(options: { feedback: AdminBrandingSettingsFeedback }) {
  const queryClient = useQueryClient()
  const brandingQuery = useQuery<BrandingSetting, Error>({
    queryKey: ADMIN_BRANDING_QUERY_KEY,
    queryFn: ({ signal }) => fetchBrandingSetting(signal),
  })
  const setting = computed(() => brandingQuery.data.value)
  const isLoading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: BrandingSetting) => saveBrandingSetting(draft),
  })
  const isSaving = computed(() => saveMutation.isPending.value)
  const lifecycle = useSettingsLifecycle({
    defaultValue: DEFAULT_SITE_BRANDING_SETTING,
    source: setting,
    loadError: brandingQuery.error,
    isLoading,
    isSaving,
    normalize: normalizeSiteBrandingSetting,
    isSame: isSameBrandingSetting,
    validate: validateSiteBrandingSetting,
    save: async (draft) => {
      const saved = await saveMutation.mutateAsync(draft)
      writeBrandingQueryCache(queryClient, saved)
      writeSiteBrandingQueryCache(queryClient, saved)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ADMIN_BRANDING_QUERY_KEY }),
        queryClient.invalidateQueries({ queryKey: SITE_BRANDING_QUERY_KEY }),
      ])
      return saved
    },
    feedback: options.feedback,
  })

  async function saveSetting(): Promise<boolean> {
    return lifecycle.save()
  }

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
    isLoading: lifecycle.isLoading,
    logoPreviewURL,
    reset: lifecycle.reset,
    save: saveSetting,
    isSaving: lifecycle.isSaving,
    uploading,
    validationError: lifecycle.validationError,
  }
}

async function fetchBrandingSetting(signal?: AbortSignal): Promise<BrandingSetting> {
  try {
    const body = await getControlBrandingSetting({ signal })
    return requireContractData<BrandingSetting>(body, '加载项目品牌失败')
  } catch (error) {
    throw withControlCopy(error, '加载项目品牌失败')
  }
}

async function saveBrandingSetting(draft: BrandingSetting): Promise<BrandingSetting> {
  try {
    const body = await updateControlBrandingSetting(
      { project_name: draft.project_name, logo_url: draft.logo_url },
      {
        headers: buildControlMutationHeaders(),
      },
    )
    return requireContractData<BrandingSetting>(body, '保存项目品牌失败')
  } catch (error) {
    throw withControlCopy(error, '保存项目品牌失败')
  }
}

function isSameBrandingSetting(current: BrandingSetting, synced: BrandingSetting): boolean {
  return current.project_name === synced.project_name && current.logo_url === synced.logo_url
}

async function uploadBrandingLogoFile(file: File): Promise<string> {
  try {
    const body = await uploadControlBrandingLogo(
      { file },
      { headers: buildControlMutationHeaders() },
    )
    return requireContractData<ControlBrandingLogoUpload>(body, '上传 Logo 失败').logo_url
  } catch (error) {
    throw withControlCopy(error, '上传 Logo 失败')
  }
}

function writeBrandingQueryCache(queryClient: QueryClient, setting: BrandingSetting) {
  queryClient.setQueryData(ADMIN_BRANDING_QUERY_KEY, setting)
}
