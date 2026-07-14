import { computed } from 'vue'
import {
  getGetAdminGoogleNativeGenerationSettingQueryKey,
  useGetAdminGoogleNativeGenerationSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import {
  GoogleNativeGenerationSettingMode,
  type GoogleNativeGenerationSetting,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector } from '../../../shared/api/apiResponse'
import type { AdminGoogleNativeGenerationSettingsFeedback } from './useAdminSystemSettingsFeedback'
import { updateGoogleNativeGenerationSetting } from './systemSettingsCommands'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

export const GOOGLE_NATIVE_GENERATION_CANARY_MIN_PERCENT = 0
export const GOOGLE_NATIVE_GENERATION_CANARY_MAX_PERCENT = 100

export const DEFAULT_GOOGLE_NATIVE_GENERATION_SETTING: GoogleNativeGenerationSetting = {
  mode: GoogleNativeGenerationSettingMode.enabled,
  canary_percent: 0,
}

export const googleNativeGenerationModeOptions = [
  { label: '关闭', value: GoogleNativeGenerationSettingMode.disabled },
  { label: '启用', value: GoogleNativeGenerationSettingMode.enabled },
  { label: '灰度', value: GoogleNativeGenerationSettingMode.canary },
]

const selectGoogleNativeGenerationSetting =
  createApiDataSelector<GoogleNativeGenerationSetting>('加载生成策略灰度失败')

export function useAdminGoogleNativeGenerationSettings(options: {
  feedback: AdminGoogleNativeGenerationSettingsFeedback
}) {
  const query = useGetAdminGoogleNativeGenerationSetting<GoogleNativeGenerationSetting>({
    query: { select: selectGoogleNativeGenerationSetting },
  })
  const sourceSetting = computed(() => query.data.value)
  const resource = useSettingResourceLifecycle<GoogleNativeGenerationSetting>({
    defaultSetting: DEFAULT_GOOGLE_NATIVE_GENERATION_SETTING,
    feedback: options.feedback,
    isSame: isSameGoogleNativeGenerationSetting,
    normalize: normalizeGoogleNativeGenerationSetting,
    query,
    queryKey: getGetAdminGoogleNativeGenerationSettingQueryKey(),
    save: updateGoogleNativeGenerationSetting,
    source: sourceSetting,
    validate: validateGoogleNativeGenerationSetting,
  })

  const statusLabel = computed(() => getGoogleNativeGenerationStatusLabel(resource.form.value))
  const statusSeverity = computed(() =>
    getGoogleNativeGenerationStatusSeverity(resource.form.value),
  )

  async function handleSaveGoogleNativeGenerationSettings() {
    await resource.save()
  }

  function resetGoogleNativeGenerationForm() {
    resource.reset()
  }

  return {
    form: resource.form,
    handleSaveGoogleNativeGenerationSettings,
    loading: resource.loading,
    resetGoogleNativeGenerationForm,
    saving: resource.saving,
    statusLabel,
    statusSeverity,
  }
}

export function normalizeGoogleNativeGenerationSetting(
  setting: Partial<GoogleNativeGenerationSetting> | null | undefined,
): GoogleNativeGenerationSetting {
  return {
    mode: normalizeGoogleNativeGenerationMode(setting?.mode),
    canary_percent: normalizeCanaryPercent(
      setting?.canary_percent,
      DEFAULT_GOOGLE_NATIVE_GENERATION_SETTING.canary_percent,
    ),
  }
}

export function validateGoogleNativeGenerationSetting(
  setting: GoogleNativeGenerationSetting,
): string {
  if (!isGoogleNativeGenerationMode(setting.mode)) {
    return '模式仅支持关闭、启用或灰度'
  }
  const percent = Number(setting.canary_percent)
  if (
    !Number.isInteger(percent) ||
    percent < GOOGLE_NATIVE_GENERATION_CANARY_MIN_PERCENT ||
    percent > GOOGLE_NATIVE_GENERATION_CANARY_MAX_PERCENT
  ) {
    return `灰度比例必须在 ${GOOGLE_NATIVE_GENERATION_CANARY_MIN_PERCENT} 到 ${GOOGLE_NATIVE_GENERATION_CANARY_MAX_PERCENT} 之间`
  }
  return ''
}

export function getGoogleNativeGenerationStatusLabel(
  setting: GoogleNativeGenerationSetting,
): string {
  switch (setting.mode) {
    case GoogleNativeGenerationSettingMode.disabled:
      return '已关闭'
    case GoogleNativeGenerationSettingMode.canary:
      return `灰度 ${setting.canary_percent}%`
    default:
      return '全部启用'
  }
}

export function getGoogleNativeGenerationStatusSeverity(
  setting: GoogleNativeGenerationSetting,
): 'danger' | 'secondary' | 'success' | 'warn' {
  switch (setting.mode) {
    case GoogleNativeGenerationSettingMode.disabled:
      return 'danger'
    case GoogleNativeGenerationSettingMode.canary:
      return 'warn'
    case GoogleNativeGenerationSettingMode.enabled:
      return 'success'
    default:
      return 'secondary'
  }
}

function isSameGoogleNativeGenerationSetting(
  current: GoogleNativeGenerationSetting,
  synced: GoogleNativeGenerationSetting,
): boolean {
  return current.mode === synced.mode && current.canary_percent === synced.canary_percent
}

function normalizeGoogleNativeGenerationMode(
  value: unknown,
): GoogleNativeGenerationSetting['mode'] {
  return isGoogleNativeGenerationMode(value) ? value : DEFAULT_GOOGLE_NATIVE_GENERATION_SETTING.mode
}

function isGoogleNativeGenerationMode(
  value: unknown,
): value is GoogleNativeGenerationSetting['mode'] {
  return (
    value === GoogleNativeGenerationSettingMode.disabled ||
    value === GoogleNativeGenerationSettingMode.enabled ||
    value === GoogleNativeGenerationSettingMode.canary
  )
}

function normalizeCanaryPercent(value: unknown, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.trunc(value)
}
