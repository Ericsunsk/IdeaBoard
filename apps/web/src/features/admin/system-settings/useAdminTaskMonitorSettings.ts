import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getGetAdminTaskMonitorSettingQueryKey,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseTaskMonitorSetting,
  TaskMonitorSetting,
} from '../../../shared/api/generated-query/model'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'
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

type AdminTaskMonitorSettingsMessage = {
  error: (message: string) => void
  success: (message: string) => void
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
