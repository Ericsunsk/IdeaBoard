import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getGetAdminRateLimitSettingsQueryKey,
  getGetAdminRegistrationSettingQueryKey,
  useGetAdminRateLimitSettings,
  useGetAdminRegistrationSetting,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import {
  updateRateLimitSettings,
  updateRegistrationSetting,
} from '../../shared/queries/adminActions'

type AdminUserSettingsMessage = {
  error: (message: string) => void
  success: (message: string) => void
}

export const RATE_LIMIT_MAX_REQUESTS = 10_000
export const RATE_LIMIT_MAX_WINDOW_SECONDS = 3_600

export type AdminUserRateLimitForm = {
  window_seconds: number | null
  max_requests: number | null
}

const DEFAULT_RATE_LIMIT_SETTINGS: AdminUserRateLimitForm = {
  window_seconds: 60,
  max_requests: 30,
}
const DEFAULT_REGISTRATION_ENABLED = false

export function useAdminUserSettings(options: { message: AdminUserSettingsMessage }) {
  const queryClient = useQueryClient()
  const registrationSettingQuery = useGetAdminRegistrationSetting()
  const rateLimitSettingsQuery = useGetAdminRateLimitSettings()
  const rateLimitForm = ref<AdminUserRateLimitForm>({ ...DEFAULT_RATE_LIMIT_SETTINGS })
  const registrationEnabled = ref(DEFAULT_REGISTRATION_ENABLED)

  function invalidateRegistrationSettingQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminRegistrationSettingQueryKey() })
  }

  function invalidateRateLimitSettingsQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminRateLimitSettingsQueryKey() })
  }

  const saveRateLimitMutation = useMutation({
    mutationFn: updateRateLimitSettings,
    onSuccess: async (settings) => {
      rateLimitForm.value.window_seconds = settings.window_seconds
      rateLimitForm.value.max_requests = settings.max_requests
      await invalidateRateLimitSettingsQuery()
    },
  })
  const updateRegistrationSettingMutation = useMutation({
    mutationFn: updateRegistrationSetting,
    onSuccess: async (setting) => {
      registrationEnabled.value = setting.enabled
      await invalidateRegistrationSettingQuery()
    },
  })

  const rateLimitSaving = computed(() => saveRateLimitMutation.isPending.value)
  const registrationSettingLoading = computed(
    () =>
      registrationSettingQuery.isLoading.value ||
      registrationSettingQuery.isFetching.value ||
      updateRegistrationSettingMutation.isPending.value,
  )

  watch(
    () => registrationSettingQuery.error.value || rateLimitSettingsQuery.error.value,
    (error) => {
      if (error) {
        options.message.error(formatDisplayErrorMessage(error, '加载后台配置失败'))
      }
    },
  )

  watch(
    () => rateLimitSettingsQuery.data.value,
    (response) => {
      const settings = response?.data
      if (!settings || saveRateLimitMutation.isPending.value) return
      rateLimitForm.value.window_seconds = settings.window_seconds
      rateLimitForm.value.max_requests = settings.max_requests
    },
    { immediate: true },
  )

  watch(
    () => registrationSettingQuery.data.value,
    (response) => {
      const setting = response?.data
      if (!setting || updateRegistrationSettingMutation.isPending.value) return
      registrationEnabled.value = setting.enabled
    },
    { immediate: true },
  )

  async function handleSaveRateLimit() {
    const validationError = validateRateLimitSettings(rateLimitForm.value)
    if (validationError) {
      options.message.error(validationError)
      return
    }

    try {
      await saveRateLimitMutation.mutateAsync({
        window_seconds: Number(rateLimitForm.value.window_seconds),
        max_requests: Number(rateLimitForm.value.max_requests),
      })
      options.message.success('限流配置已保存')
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '保存失败'))
    }
  }

  async function handleToggleRegistration(newValue: boolean) {
    const fallbackEnabled = registrationSettingQuery.data.value?.data?.enabled ?? !newValue
    try {
      await updateRegistrationSettingMutation.mutateAsync(newValue)
      options.message.success(newValue ? '已开放注册' : '已关闭注册')
    } catch (error) {
      registrationEnabled.value = fallbackEnabled
      options.message.error(formatDisplayErrorMessage(error, '更新注册状态失败'))
    }
  }

  return {
    rateLimitForm,
    rateLimitSaving,
    registrationEnabled,
    registrationSettingLoading,
    handleSaveRateLimit,
    handleToggleRegistration,
  }
}

export function validateRateLimitSettings(settings: AdminUserRateLimitForm) {
  const maxRequests = Number(settings.max_requests)
  const windowSeconds = Number(settings.window_seconds)
  if (!Number.isInteger(maxRequests) || maxRequests < 1) {
    return '最大请求需为大于 0 的整数'
  }
  if (maxRequests > RATE_LIMIT_MAX_REQUESTS) {
    return `最大请求不能超过 ${RATE_LIMIT_MAX_REQUESTS}`
  }
  if (!Number.isInteger(windowSeconds) || windowSeconds < 1) {
    return '窗口周期需为大于 0 的整数秒'
  }
  if (windowSeconds > RATE_LIMIT_MAX_WINDOW_SECONDS) {
    return `窗口周期不能超过 ${RATE_LIMIT_MAX_WINDOW_SECONDS} 秒`
  }
  return ''
}
