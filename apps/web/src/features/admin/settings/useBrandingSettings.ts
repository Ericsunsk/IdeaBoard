import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import {
  getControlBrandingSetting,
  updateControlBrandingSetting,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'
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
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { confirmSettingsSave } from '../shared/lib/confirmSettingsSave'
import { adminControlUnavailable } from '../shared/lib/controlGuidance'
import { withSimpleControlCopy as withControlCopy } from '../shared/lib/controlCopy'

// Admin branding boundary (W2 transports). Reads and writes go through the
// Control Plane with the contract envelope (`{data}`); every mutation
// carries a header-only Idempotency-Key plus an operator reason collected
// through the shared confirm dialog, and emits a control audit record. The
// deleted `/api/admin/settings/branding` route is gone; this module is the
// single transport. Direct logo upload has no control equivalent
// (`POST /api/admin/upload` is deleted), so it fails closed with guidance:
// operators paste a Logo URL instead.
const ADMIN_BRANDING_QUERY_KEY = ['admin', 'settings', 'branding'] as const

export function useBrandingSettings(options: {
  feedback: AdminBrandingSettingsFeedback
  confirm: AdminConfirmPort
}) {
  const queryClient = useQueryClient()
  const brandingQuery = useQuery<BrandingSetting, Error>({
    queryKey: ADMIN_BRANDING_QUERY_KEY,
    queryFn: ({ signal }) => fetchBrandingSetting(signal),
  })
  const setting = computed(() => brandingQuery.data.value)
  const loading = computed(() => brandingQuery.isLoading.value || brandingQuery.isFetching.value)
  // The lifecycle save runs inside saveSetting below with the reason
  // collected first; the ref bridges the two without changing the
  // lifecycle contract.
  const pendingOperatorReason = ref<string | null>(null)
  const saveMutation = useMutation({
    mutationFn: (draft: BrandingSetting) =>
      saveBrandingSetting(draft, pendingOperatorReason.value ?? ''),
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
    const operatorReason = await confirmSettingsSave(options.confirm, {
      title: '保存站点设置',
      message: '确定保存站点名称与 Logo 配置吗？该操作将记录操作人与原因。',
      confirmLabel: '保存',
    })
    if (!operatorReason) return false
    pendingOperatorReason.value = operatorReason
    try {
      return await lifecycle.save()
    } finally {
      pendingOperatorReason.value = null
    }
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

async function saveBrandingSetting(
  draft: BrandingSetting,
  operatorReason: string,
): Promise<BrandingSetting> {
  const reason = requireOperatorReason(operatorReason)
  try {
    const body = await updateControlBrandingSetting(
      { project_name: draft.project_name, logo_url: draft.logo_url },
      {
        headers: buildContractRequestHeaders({
          requestId: newRequestId(),
          idempotencyKey: newRequestId(),
          operatorReason: reason,
        }),
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

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.

function writeBrandingQueryCache(queryClient: QueryClient, setting: BrandingSetting) {
  queryClient.setQueryData(ADMIN_BRANDING_QUERY_KEY, setting)
}
