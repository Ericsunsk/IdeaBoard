import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'
import {
  getGetAdminMediaUploadSettingQueryKey,
  updateAdminMediaUploadSetting,
  useGetAdminMediaUploadSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { MediaUploadSetting } from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'
import type { AdminMediaUploadSettingsFeedback } from './useAdminSystemSettingsFeedback'

export const DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING: MediaUploadSetting = {
  upload_max_concurrent: 8,
}

export const MEDIA_UPLOAD_MIN_CONCURRENT = 1
export const MEDIA_UPLOAD_MAX_CONCURRENT = 64

const selectMediaUploadSetting = createApiDataSelector<MediaUploadSetting>('加载上传设置失败')

export function useAdminMediaUploadSettings(options: {
  feedback: AdminMediaUploadSettingsFeedback
}) {
  const queryClient = useQueryClient()
  const query = useGetAdminMediaUploadSetting<MediaUploadSetting>({
    query: { select: selectMediaUploadSetting },
  })
  const setting = computed(() => query.data.value)
  const form = ref<MediaUploadSetting>(
    cloneSetting(DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING),
  ) as Ref<MediaUploadSetting>
  const lastSyncedForm = ref<MediaUploadSetting>(
    cloneSetting(DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING),
  ) as Ref<MediaUploadSetting>
  const validationError = ref('')
  const loading = computed(() => query.isLoading.value || query.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: MediaUploadSetting) =>
      resolveApiData(updateAdminMediaUploadSetting(draft), '保存上传设置失败'),
    onSuccess: async (savedSetting) => {
      hydrate(savedSetting)
      writeMediaUploadQueryCache(queryClient, savedSetting)
      await queryClient.invalidateQueries({ queryKey: getGetAdminMediaUploadSettingQueryKey() })
      if (query.data.value) hydrate(query.data.value)
    },
  })
  const saving = computed(() => saveMutation.isPending.value)
  const isDirty = computed(() => !isSameMediaUploadSetting(form.value, lastSyncedForm.value))

  function hydrate(savedSetting: Partial<MediaUploadSetting> | null | undefined) {
    const nextForm = normalizeMediaUploadSetting(savedSetting)
    lastSyncedForm.value = cloneSetting(nextForm)
    form.value = cloneSetting(nextForm)
    validationError.value = ''
  }

  async function save(): Promise<boolean> {
    const validationMessage = validateMediaUploadSetting(form.value)
    if (validationMessage) {
      validationError.value = validationMessage
      options.feedback.validationFailed(validationMessage)
      return false
    }
    try {
      await saveMutation.mutateAsync(normalizeMediaUploadSetting(form.value))
      options.feedback.saved()
      return true
    } catch (error) {
      options.feedback.saveFailed(error)
      return false
    }
  }

  function reset() {
    form.value = cloneSetting(lastSyncedForm.value)
    validationError.value = ''
  }

  watch(form, () => (validationError.value = ''), { deep: true, flush: 'sync' })
  watch(
    () => query.error.value,
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
    form,
    isDirty,
    loading,
    reset,
    save,
    saving,
    validationError,
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
  queryClient.setQueryData(getGetAdminMediaUploadSettingQueryKey(), {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}

function cloneSetting(setting: MediaUploadSetting): MediaUploadSetting {
  return structuredClone(toRaw(setting))
}
