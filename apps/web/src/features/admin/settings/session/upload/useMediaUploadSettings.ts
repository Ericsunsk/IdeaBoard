import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  getControlMediaUploadSetting,
  updateControlMediaUploadSetting,
} from '../../../../../api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../../../api/contractKernel'
import { buildRoutineControlMutationHeaders } from '../../../shared/index'
import type { AdminMediaUploadSettingsFeedback } from '../useSettingsFeedback'
import { useSettingsLifecycle } from '../useSettingsLifecycle'
import { withControlCopy } from '../../../shared/index'

// Admin media upload boundary (W2 transports). Reads and writes go through
// the Control Plane with the contract envelope (`{data}`); every mutation
// carries a header-only Idempotency-Key and emits a control audit record.
// Media upload is a routine config write (backend ReasonOptional), so saves
// skip the confirm dialog and audit an empty reason. The
// deleted `/api/admin/settings/media-upload` route is gone; this module is
// the single transport.
export type MediaUploadSetting = {
  upload_max_concurrent: number
}

const ADMIN_MEDIA_UPLOAD_QUERY_KEY = ['admin', 'settings', 'media-upload'] as const

export const DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING: MediaUploadSetting = {
  upload_max_concurrent: 8,
}

export const MEDIA_UPLOAD_MIN_CONCURRENT = 1
export const MEDIA_UPLOAD_MAX_CONCURRENT = 64

export function useMediaUploadSettings(options: { feedback: AdminMediaUploadSettingsFeedback }) {
  const queryClient = useQueryClient()
  const query = useQuery<MediaUploadSetting, Error>({
    queryKey: ADMIN_MEDIA_UPLOAD_QUERY_KEY,
    queryFn: ({ signal }) => fetchMediaUploadSetting(signal),
  })
  const setting = computed(() => query.data.value)
  const loading = computed(() => query.isLoading.value || query.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: MediaUploadSetting) => saveMediaUploadSetting(draft),
  })
  const saving = computed(() => saveMutation.isPending.value)
  const lifecycle = useSettingsLifecycle({
    defaultValue: DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
    source: setting,
    loadError: query.error,
    loading,
    saving,
    normalize: normalizeMediaUploadSetting,
    isSame: isSameMediaUploadSetting,
    validate: validateMediaUploadSetting,
    save: (draft) => saveMutation.mutateAsync(draft),
    synchronizeCache: async (savedSetting) => {
      writeMediaUploadQueryCache(queryClient, savedSetting)
      await queryClient.invalidateQueries({ queryKey: ADMIN_MEDIA_UPLOAD_QUERY_KEY })
    },
    feedback: options.feedback,
  })

  async function saveSetting(): Promise<boolean> {
    // Routine config write (backend ReasonOptional): save directly without
    // the confirm dialog; the audit records an empty reason.
    return lifecycle.save()
  }

  return {
    form: lifecycle.form,
    isDirty: lifecycle.isDirty,
    loading: lifecycle.loading,
    reset: lifecycle.reset,
    save: saveSetting,
    saving: lifecycle.saving,
    validationError: lifecycle.validationError,
  }
}

async function fetchMediaUploadSetting(signal?: AbortSignal): Promise<MediaUploadSetting> {
  try {
    const body = await getControlMediaUploadSetting({ signal })
    return requireContractData<MediaUploadSetting>(body, '加载上传设置失败')
  } catch (error) {
    throw withControlCopy(error, '加载上传设置失败')
  }
}

async function saveMediaUploadSetting(draft: MediaUploadSetting): Promise<MediaUploadSetting> {
  try {
    const body = await updateControlMediaUploadSetting(
      { upload_max_concurrent: draft.upload_max_concurrent },
      {
        // Routine config write (backend ReasonOptional): no operator-reason
        // header, the audit records an empty reason. Idempotency-Key stays
        // required on both tiers.
        headers: buildRoutineControlMutationHeaders(),
      },
    )
    return requireContractData<MediaUploadSetting>(body, '保存上传设置失败')
  } catch (error) {
    throw withControlCopy(error, '保存上传设置失败')
  }
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.

export function normalizeMediaUploadSetting(
  setting: Partial<MediaUploadSetting> | null | undefined,
): MediaUploadSetting {
  return {
    upload_max_concurrent: normalizeConcurrentValue(
      setting?.upload_max_concurrent,
      DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING.upload_max_concurrent,
    ),
  }
}

export function validateMediaUploadSetting(setting: MediaUploadSetting): string {
  const value = Number(setting.upload_max_concurrent)
  if (
    !Number.isInteger(value) ||
    value < MEDIA_UPLOAD_MIN_CONCURRENT ||
    value > MEDIA_UPLOAD_MAX_CONCURRENT
  ) {
    return `上传并发上限必须在 ${MEDIA_UPLOAD_MIN_CONCURRENT} 到 ${MEDIA_UPLOAD_MAX_CONCURRENT} 之间`
  }
  return ''
}

function isSameMediaUploadSetting(
  current: MediaUploadSetting,
  synced: MediaUploadSetting,
): boolean {
  return current.upload_max_concurrent === synced.upload_max_concurrent
}

function normalizeConcurrentValue(value: unknown, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.trunc(value)
}

function writeMediaUploadQueryCache(queryClient: QueryClient, setting: MediaUploadSetting) {
  queryClient.setQueryData(ADMIN_MEDIA_UPLOAD_QUERY_KEY, setting)
}
