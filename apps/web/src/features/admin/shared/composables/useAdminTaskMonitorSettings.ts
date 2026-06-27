import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getGetAdminTaskMonitorSettingQueryKey,
  useGetAdminTaskMonitorSetting,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseTaskMonitorSetting,
  TaskMonitorSetting,
} from '../../../../shared/api/generated-query/model'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import { updateTaskMonitorSetting } from '../queries/adminSettingsActions'

type AdminTaskMonitorSettingsMessage = {
  error: (message: string) => void
  success: (message: string) => void
}

export const DEFAULT_ADMIN_TASK_MONITOR_SETTING: TaskMonitorSetting = {
  list_page_size: 12,
  auto_refresh_enabled: true,
  refresh_interval_seconds: 4,
  result_preview_enabled: true,
}

export const TASK_MONITOR_MIN_PAGE_SIZE = 5
export const TASK_MONITOR_MAX_PAGE_SIZE = 50
export const TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS = 2
export const TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS = 60

export function useAdminTaskMonitorSettingQuery() {
  const query = useGetAdminTaskMonitorSetting()
  const settings = computed(() => normalizeTaskMonitorSetting(query.data.value?.data))
  const settingsError = computed(() => (query.isError.value ? (query.error.value ?? null) : null))

  return {
    query,
    settings,
    settingsError,
  }
}

export function useAdminTaskMonitorSettings(options: {
  message: AdminTaskMonitorSettingsMessage
}) {
  const queryClient = useQueryClient()
  const { query, settings } = useAdminTaskMonitorSettingQuery()
  const form = ref<TaskMonitorSetting>({ ...DEFAULT_ADMIN_TASK_MONITOR_SETTING })
  const lastSyncedForm = ref<TaskMonitorSetting>({ ...DEFAULT_ADMIN_TASK_MONITOR_SETTING })

  function invalidateTaskMonitorSettingQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminTaskMonitorSettingQueryKey() })
  }

  function writeTaskMonitorSettingQueryCache(setting: TaskMonitorSetting) {
    queryClient.setQueryData<ApiResponseTaskMonitorSetting>(
      getGetAdminTaskMonitorSettingQueryKey(),
      (previous) => ({
        code: previous?.code ?? 0,
        msg: previous?.msg ?? 'ok',
        data: setting,
      }),
    )
  }

  const saveTaskMonitorMutation = useMutation({
    mutationFn: updateTaskMonitorSetting,
    onSuccess: async (setting) => {
      const normalizedSetting = hydrateTaskMonitorForm(setting)
      writeTaskMonitorSettingQueryCache(normalizedSetting)
      await invalidateTaskMonitorSettingQuery()
    },
  })

  const loading = computed(() => query.isLoading.value || query.isFetching.value)
  const saving = computed(() => saveTaskMonitorMutation.isPending.value)
  const formDirty = computed(() => !isSameTaskMonitorSetting(form.value, lastSyncedForm.value))

  watch(
    () => query.error.value,
    (error) => {
      if (error) {
        options.message.error(formatDisplayErrorMessage(error, '加载任务监控设置失败'))
      }
    },
  )

  watch(
    settings,
    (nextSettings) => {
      if (saveTaskMonitorMutation.isPending.value || formDirty.value) return
      hydrateTaskMonitorForm(nextSettings)
    },
    { immediate: true },
  )

  async function handleSaveTaskMonitorSettings() {
    const validationError = validateTaskMonitorSetting(form.value)
    if (validationError) {
      options.message.error(validationError)
      return
    }

    try {
      await saveTaskMonitorMutation.mutateAsync(normalizeTaskMonitorSetting(form.value))
      options.message.success('任务监控设置已保存')
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '保存任务监控设置失败'))
    }
  }

  function hydrateTaskMonitorForm(setting: Partial<TaskMonitorSetting> | null | undefined) {
    const normalizedSetting = normalizeTaskMonitorSetting(setting)
    lastSyncedForm.value = { ...normalizedSetting }
    form.value = { ...normalizedSetting }
    return normalizedSetting
  }

  function resetTaskMonitorForm() {
    form.value = { ...lastSyncedForm.value }
  }

  return {
    form,
    handleSaveTaskMonitorSettings,
    loading,
    resetTaskMonitorForm,
    saving,
  }
}

function isSameTaskMonitorSetting(current: TaskMonitorSetting, synced: TaskMonitorSetting): boolean {
  return (
    current.list_page_size === synced.list_page_size &&
    current.auto_refresh_enabled === synced.auto_refresh_enabled &&
    current.refresh_interval_seconds === synced.refresh_interval_seconds &&
    current.result_preview_enabled === synced.result_preview_enabled
  )
}

export function normalizeTaskMonitorSetting(
  setting: Partial<TaskMonitorSetting> | null | undefined,
): TaskMonitorSetting {
  const fallback = DEFAULT_ADMIN_TASK_MONITOR_SETTING
  const listPageSize =
    typeof setting?.list_page_size === 'number' ? setting.list_page_size : fallback.list_page_size
  const refreshIntervalSeconds =
    typeof setting?.refresh_interval_seconds === 'number'
      ? setting.refresh_interval_seconds
      : fallback.refresh_interval_seconds

  return {
    list_page_size: clampInteger(
      listPageSize,
      TASK_MONITOR_MIN_PAGE_SIZE,
      TASK_MONITOR_MAX_PAGE_SIZE,
      fallback.list_page_size,
    ),
    auto_refresh_enabled:
      typeof setting?.auto_refresh_enabled === 'boolean'
        ? setting.auto_refresh_enabled
        : fallback.auto_refresh_enabled,
    refresh_interval_seconds: clampInteger(
      refreshIntervalSeconds,
      TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS,
      TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS,
      fallback.refresh_interval_seconds,
    ),
    result_preview_enabled:
      typeof setting?.result_preview_enabled === 'boolean'
        ? setting.result_preview_enabled
        : fallback.result_preview_enabled,
  }
}

export function validateTaskMonitorSetting(setting: TaskMonitorSetting): string {
  if (
    !Number.isInteger(setting.list_page_size) ||
    setting.list_page_size < TASK_MONITOR_MIN_PAGE_SIZE ||
    setting.list_page_size > TASK_MONITOR_MAX_PAGE_SIZE
  ) {
    return `任务列表每页数量必须在 ${TASK_MONITOR_MIN_PAGE_SIZE} 到 ${TASK_MONITOR_MAX_PAGE_SIZE} 之间`
  }
  if (
    !Number.isInteger(setting.refresh_interval_seconds) ||
    setting.refresh_interval_seconds < TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS ||
    setting.refresh_interval_seconds > TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS
  ) {
    return `刷新间隔必须在 ${TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS} 到 ${TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS} 秒之间`
  }
  return ''
}

function clampInteger(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isInteger(value) || value < min || value > max) return fallback
  return value
}
