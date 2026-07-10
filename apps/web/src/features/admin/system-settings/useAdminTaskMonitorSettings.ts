import { getGetAdminTaskMonitorSettingQueryKey } from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { TaskMonitorSetting } from '../../../shared/api/generated-query/model'
import type { AdminTaskMonitorSettingsFeedback } from '../feedback/useAdminFeedback'
import {
  DEFAULT_ADMIN_TASK_MONITOR_SETTING,
  TASK_MONITOR_MAX_PAGE_SIZE,
  TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS,
  TASK_MONITOR_MIN_PAGE_SIZE,
  TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS,
  normalizeTaskMonitorSetting,
  useAdminTaskMonitorSettingQuery,
} from './taskMonitorSettingsResource'
import { updateTaskMonitorSetting } from './systemSettingsCommands'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

export function useAdminTaskMonitorSettings(options: {
  feedback: AdminTaskMonitorSettingsFeedback
}) {
  const { query, settings } = useAdminTaskMonitorSettingQuery()
  const resource = useSettingResourceLifecycle<TaskMonitorSetting>({
    defaultSetting: DEFAULT_ADMIN_TASK_MONITOR_SETTING,
    feedback: options.feedback,
    isSame: isSameTaskMonitorSetting,
    normalize: normalizeTaskMonitorSetting,
    query,
    queryKey: getGetAdminTaskMonitorSettingQueryKey(),
    save: updateTaskMonitorSetting,
    source: settings,
    validate: validateTaskMonitorSetting,
  })

  async function handleSaveTaskMonitorSettings() {
    await resource.save()
  }

  function resetTaskMonitorForm() {
    resource.reset()
  }

  return {
    form: resource.form,
    handleSaveTaskMonitorSettings,
    loading: resource.loading,
    resetTaskMonitorForm,
    saving: resource.saving,
  }
}

function isSameTaskMonitorSetting(
  current: TaskMonitorSetting,
  synced: TaskMonitorSetting,
): boolean {
  return (
    current.list_page_size === synced.list_page_size &&
    current.auto_refresh_enabled === synced.auto_refresh_enabled &&
    current.refresh_interval_seconds === synced.refresh_interval_seconds &&
    current.result_preview_enabled === synced.result_preview_enabled
  )
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
