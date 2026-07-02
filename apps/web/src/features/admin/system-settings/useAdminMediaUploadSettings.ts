import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import {
  getGetAdminMediaUploadSettingQueryKey,
  useGetAdminMediaUploadSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseMediaUploadSetting,
  MediaUploadSetting,
} from '../../../shared/api/generated-query/model'
import type { AdminMediaUploadSettingsFeedback } from '../feedback/useAdminFeedback'
import { updateMediaUploadSetting } from './systemSettingsCommands'
import { useSettingFormLifecycle } from './useSettingFormLifecycle'

export const DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING: MediaUploadSetting = {
  upload_max_concurrent: 8,
}

export const MEDIA_UPLOAD_MIN_CONCURRENT = 1
export const MEDIA_UPLOAD_MAX_CONCURRENT = 64

export function useAdminMediaUploadSettings(options: { feedback: AdminMediaUploadSettingsFeedback }) {
  const queryClient = useQueryClient()
  const query = useGetAdminMediaUploadSetting()
  const form = ref<MediaUploadSetting>({ ...DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING })
  const lastSyncedForm = ref<MediaUploadSetting>({ ...DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING })

  function invalidateMediaUploadQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminMediaUploadSettingQueryKey() })
  }

  function writeMediaUploadQueryCache(setting: MediaUploadSetting) {
    queryClient.setQueryData<ApiResponseMediaUploadSetting>(
      getGetAdminMediaUploadSettingQueryKey(),
      (previous) => ({
        code: previous?.code ?? 0,
        msg: previous?.msg ?? 'ok',
        data: setting,
      }),
    )
  }

  const saveMutation = useMutation({
    mutationFn: updateMediaUploadSetting,
    onSuccess: async (setting) => {
      const normalizedSetting = hydrateMediaUploadForm(setting)
      writeMediaUploadQueryCache(normalizedSetting)
      await invalidateMediaUploadQuery()
    },
  })

  const loading = computed(() => query.isLoading.value || query.isFetching.value)
  const saving = computed(() => saveMutation.isPending.value)
  const sourceSetting = computed(() => query.data.value?.data)

  const formLifecycle = useSettingFormLifecycle<Partial<MediaUploadSetting>, MediaUploadSetting>({
    error: query.error,
    source: sourceSetting,
    current: form,
    synced: lastSyncedForm,
    isSaving: saveMutation.isPending,
    feedback: options.feedback,
    hydrate: hydrateMediaUploadForm,
    isSame: isSameMediaUploadSetting,
    reset: (synced) => {
      form.value = { ...synced }
    },
  })

  async function handleSaveMediaUploadSettings() {
    await formLifecycle.save({
      validate: () => validateMediaUploadSetting(form.value),
      run: () => saveMutation.mutateAsync(normalizeMediaUploadSetting(form.value)),
    })
  }

  function hydrateMediaUploadForm(setting: Partial<MediaUploadSetting> | null | undefined) {
    const normalizedSetting = normalizeMediaUploadSetting(setting)
    lastSyncedForm.value = { ...normalizedSetting }
    form.value = { ...normalizedSetting }
    return normalizedSetting
  }

  function resetMediaUploadForm() {
    formLifecycle.reset()
  }

  return {
    form,
    handleSaveMediaUploadSettings,
    loading,
    resetMediaUploadForm,
    saving,
  }
}

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

function isSameMediaUploadSetting(current: MediaUploadSetting, synced: MediaUploadSetting): boolean {
  return current.upload_max_concurrent === synced.upload_max_concurrent
}

function normalizeConcurrentValue(value: unknown, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.trunc(value)
}
