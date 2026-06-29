import { computed } from 'vue'
import { useGetAdminTaskMonitorSetting } from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { TaskMonitorSetting } from '../../../shared/api/generated-query/model'

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

function clampInteger(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isInteger(value) || value < min || value > max) return fallback
  return value
}
