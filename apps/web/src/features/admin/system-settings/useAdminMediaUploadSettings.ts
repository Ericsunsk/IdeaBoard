import { computed } from 'vue'
import {
  getGetAdminMediaUploadSettingQueryKey,
  useGetAdminMediaUploadSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { MediaUploadSetting } from '../../../shared/api/generated-query/model'
import type { AdminMediaUploadSettingsFeedback } from '../feedback/useAdminFeedback'
import { updateMediaUploadSetting } from './systemSettingsCommands'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

export const DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING: MediaUploadSetting = {
  upload_max_concurrent: 8,
}

export const MEDIA_UPLOAD_MIN_CONCURRENT = 1
export const MEDIA_UPLOAD_MAX_CONCURRENT = 64

export function useAdminMediaUploadSettings(options: { feedback: AdminMediaUploadSettingsFeedback }) {
  const query = useGetAdminMediaUploadSetting()
  const sourceSetting = computed(() => query.data.value?.data)
  const resource = useSettingResourceLifecycle<MediaUploadSetting>({
    defaultSetting: DEFAULT_ADMIN_MEDIA_UPLOAD_SETTING,
    feedback: options.feedback,
    isSame: isSameMediaUploadSetting,
    normalize: normalizeMediaUploadSetting,
    query,
    queryKey: getGetAdminMediaUploadSettingQueryKey(),
    save: updateMediaUploadSetting,
    source: sourceSetting,
    validate: validateMediaUploadSetting,
  })

  async function handleSaveMediaUploadSettings() {
    await resource.save()
  }

  function resetMediaUploadForm() {
    resource.reset()
  }

  return {
    form: resource.form,
    handleSaveMediaUploadSettings,
    loading: resource.loading,
    resetMediaUploadForm,
    saving: resource.saving,
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
