import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  getControlBrandingSetting,
  updateControlBrandingSetting,
} from '../../../../../api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../../../api/contractKernel'
import { buildRoutineControlMutationHeaders } from '../../../shared/index'
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
} from '../../../../../core/composables/useSiteBranding'
import { useSettingsLifecycle } from '../useSettingsLifecycle'
import { adminControlUnavailable } from '../../../shared/index'
import { withControlCopy } from '../../../shared/index'

// Admin branding boundary (W2 transports). Reads and writes go through the
// Control Plane with the contract envelope (`{data}`); every mutation
// carries a header-only Idempotency-Key and emits a control audit record.
// Branding is a routine config write (backend ReasonOptional), so saves skip
// the confirm dialog and audit an empty reason. The deleted
// `/api/admin/settings/branding` route is gone; this module is the
// single transport. Direct logo upload has no control equivalent
// (`POST /api/admin/upload` is deleted), so it fails closed with guidance:
// operators paste a Logo URL instead.
const ADMIN_BRANDING_QUERY_KEY = ['admin', 'settings', 'branding'] as const

export function useBrandingSettings(options: { feedback: AdminBrandingSettingsFeedback }) {
  const queryClient = useQueryClient()
  const brandingQuery = useQuery<BrandingSetting, Error>({
    queryKey: ADMIN_BRANDING_QUERY_KEY,
    queryFn: ({ signal }) => fetchBrandingSetting(signal),
  })
  const setting = computed(() => brandingQuery.data.value)
  const loading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: BrandingSetting) => saveBrandingSetting(draft),
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

  async function saveSetting(): Promise<boolean> {
    // Routine config write (backend ReasonOptional): save directly without
    // the confirm dialog; the audit records an empty reason.
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
    loading: lifecycle.loading,
    logoPreviewURL,
    reset: lifecycle.reset,
    save: saveSetting,
    saving: lifecycle.saving,
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
        // Routine config write (backend ReasonOptional): no operator-reason
        // header, the audit records an empty reason. Idempotency-Key stays
        // required on both tiers.
        headers: buildRoutineControlMutationHeaders(),
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

async function uploadBrandingLogoFile(_file: File): Promise<string> {
  // No control equivalent for direct logo upload: fail closed with
  // guidance instead of calling the deleted `POST /api/admin/upload`.
  // Backend gap: control branding logo upload (see the stream report).
  throw adminControlUnavailable('branding-upload')
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.

function writeBrandingQueryCache(queryClient: QueryClient, setting: BrandingSetting) {
  queryClient.setQueryData(ADMIN_BRANDING_QUERY_KEY, setting)
}
