import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'
import {
  getGetAdminTaskMonitorSettingQueryKey,
  updateAdminTaskMonitorSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { TaskMonitorSetting } from '../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../shared/api/apiResponse'
import type { AdminTaskMonitorSettingsFeedback } from './useAdminSystemSettingsFeedback'
import {
  DEFAULT_ADMIN_TASK_MONITOR_SETTING,
  TASK_MONITOR_MAX_PAGE_SIZE,
  TASK_MONITOR_MAX_REFRESH_INTERVAL_SECONDS,
  TASK_MONITOR_MIN_PAGE_SIZE,
  TASK_MONITOR_MIN_REFRESH_INTERVAL_SECONDS,
  normalizeTaskMonitorSetting,
  useAdminTaskMonitorSettingQuery,
} from './taskMonitorSettingsResource'
export function useAdminTaskMonitorSettings(options: {
  feedback: AdminTaskMonitorSettingsFeedback
}) {
  const queryClient = useQueryClient()
  const { query, settings } = useAdminTaskMonitorSettingQuery()
  const form = ref<TaskMonitorSetting>(
    cloneSetting(DEFAULT_ADMIN_TASK_MONITOR_SETTING),
  ) as Ref<TaskMonitorSetting>
  const lastSyncedForm = ref<TaskMonitorSetting>(
    cloneSetting(DEFAULT_ADMIN_TASK_MONITOR_SETTING),
  ) as Ref<TaskMonitorSetting>
  const validationError = ref('')
  const loading = computed(() => query.isLoading.value || query.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: TaskMonitorSetting) =>
      resolveApiData(updateAdminTaskMonitorSetting(draft), '保存任务监控设置失败'),
    onSuccess: async (savedSetting) => {
      hydrate(savedSetting)
      writeTaskMonitorQueryCache(queryClient, savedSetting)
      await queryClient.invalidateQueries({ queryKey: getGetAdminTaskMonitorSettingQueryKey() })
      if (settings.value) hydrate(settings.value)
    },
  })
  const saving = computed(() => saveMutation.isPending.value)
  const isDirty = computed(() => !isSameTaskMonitorSetting(form.value, lastSyncedForm.value))

  function hydrate(savedSetting: Partial<TaskMonitorSetting> | null | undefined) {
    const nextForm = normalizeTaskMonitorSetting(savedSetting)
    lastSyncedForm.value = cloneSetting(nextForm)
    form.value = cloneSetting(nextForm)
    validationError.value = ''
  }

  async function save(): Promise<boolean> {
    const validationMessage = validateTaskMonitorSetting(form.value)
    if (validationMessage) {
      validationError.value = validationMessage
      options.feedback.validationFailed(validationMessage)
      return false
    }
    try {
      await saveMutation.mutateAsync(normalizeTaskMonitorSetting(form.value))
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
    settings,
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

function writeTaskMonitorQueryCache(queryClient: QueryClient, setting: TaskMonitorSetting) {
  queryClient.setQueryData(getGetAdminTaskMonitorSettingQueryKey(), {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}

function cloneSetting(setting: TaskMonitorSetting): TaskMonitorSetting {
  return structuredClone(toRaw(setting))
}
