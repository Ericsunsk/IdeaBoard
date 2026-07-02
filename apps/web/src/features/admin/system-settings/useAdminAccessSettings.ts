import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getGetAdminRateLimitSettingsQueryKey,
  getGetAdminRegistrationSettingQueryKey,
  useGetAdminRateLimitSettings,
  useGetAdminRegistrationSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseRateLimitSetting,
  ApiResponseRegistrationSetting,
  ApiResponseRegistrationSettingData,
  RateLimitSetting,
} from '../../../shared/api/generated-query/model'
import type { AdminAccessSettingsFeedback } from '../feedback/useAdminFeedback'
import {
  updateRateLimitSettings,
  updateRegistrationSetting,
} from './systemSettingsCommands'
import { useSettingFormLifecycle } from './useSettingFormLifecycle'

export const RATE_LIMIT_MAX_REQUESTS = 10_000
export const RATE_LIMIT_MAX_WINDOW_SECONDS = 3_600

export type AdminAccessRateLimitForm = {
  window_seconds: number | null
  max_requests: number | null
}

const DEFAULT_RATE_LIMIT_SETTING: RateLimitSetting = {
  window_seconds: 2,
  max_requests: 120,
}
const DEFAULT_RATE_LIMIT_FORM: AdminAccessRateLimitForm = {
  ...DEFAULT_RATE_LIMIT_SETTING,
}
const DEFAULT_REGISTRATION_ENABLED = false

export function useAdminAccessSettings(options: { feedback: AdminAccessSettingsFeedback }) {
  const queryClient = useQueryClient()
  const registrationSettingQuery = useGetAdminRegistrationSetting()
  const rateLimitSettingsQuery = useGetAdminRateLimitSettings()
  const rateLimitForm = ref<AdminAccessRateLimitForm>({ ...DEFAULT_RATE_LIMIT_FORM })
  const registrationEnabled = ref(DEFAULT_REGISTRATION_ENABLED)
  const lastSyncedRateLimitSettings = ref<RateLimitSetting>({
    ...DEFAULT_RATE_LIMIT_SETTING,
  })

  function invalidateRegistrationSettingQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminRegistrationSettingQueryKey() })
  }

  function invalidateRateLimitSettingsQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminRateLimitSettingsQueryKey() })
  }

  function writeRateLimitSettingsQueryCache(settings: RateLimitSetting) {
    queryClient.setQueryData<ApiResponseRateLimitSetting>(
      getGetAdminRateLimitSettingsQueryKey(),
      (previous) => ({
        code: previous?.code ?? 0,
        msg: previous?.msg ?? 'ok',
        data: settings,
      }),
    )
  }

  function writeRegistrationSettingQueryCache(setting: ApiResponseRegistrationSettingData) {
    queryClient.setQueryData<ApiResponseRegistrationSetting>(
      getGetAdminRegistrationSettingQueryKey(),
      (previous) => ({
        code: previous?.code ?? 0,
        msg: previous?.msg ?? 'ok',
        data: setting,
      }),
    )
  }

  const saveRateLimitMutation = useMutation({
    mutationFn: updateRateLimitSettings,
    onSuccess: async (settings) => {
      hydrateRateLimitForm(settings)
      writeRateLimitSettingsQueryCache(settings)
      await invalidateRateLimitSettingsQuery()
    },
  })
  const updateRegistrationSettingMutation = useMutation({
    mutationFn: updateRegistrationSetting,
    onSuccess: async (setting) => {
      registrationEnabled.value = setting.enabled
      writeRegistrationSettingQueryCache(setting)
      await invalidateRegistrationSettingQuery()
    },
  })

  const rateLimitSaving = computed(() => saveRateLimitMutation.isPending.value)
  const rateLimitLoading = computed(
    () => rateLimitSettingsQuery.isLoading.value || rateLimitSettingsQuery.isFetching.value,
  )
  const registrationSettingLoading = computed(
    () =>
      registrationSettingQuery.isLoading.value ||
      registrationSettingQuery.isFetching.value ||
      updateRegistrationSettingMutation.isPending.value,
  )
  const rateLimitSource = computed(() => rateLimitSettingsQuery.data.value?.data)

  const rateLimitLifecycle = useSettingFormLifecycle<
    RateLimitSetting,
    AdminAccessRateLimitForm,
    RateLimitSetting
  >({
    error: rateLimitSettingsQuery.error,
    source: rateLimitSource,
    current: rateLimitForm,
    synced: lastSyncedRateLimitSettings,
    isSaving: saveRateLimitMutation.isPending,
    feedback: {
      loadFailed: options.feedback.loadFailed,
      validationFailed: options.feedback.rateLimitValidationFailed,
      saved: options.feedback.rateLimitSaved,
      saveFailed: options.feedback.rateLimitSaveFailed,
    },
    hydrate: hydrateRateLimitForm,
    isSame: isSameRateLimitSetting,
    reset: (synced) => {
      rateLimitForm.value = { ...synced }
    },
  })

  watch(
    () => registrationSettingQuery.error.value,
    (error) => {
      if (error) {
        options.feedback.loadFailed(error)
      }
    },
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
    await rateLimitLifecycle.save({
      validate: () => validateRateLimitSettings(rateLimitForm.value),
      run: () =>
        saveRateLimitMutation.mutateAsync({
          window_seconds: Number(rateLimitForm.value.window_seconds),
          max_requests: Number(rateLimitForm.value.max_requests),
        }),
    })
  }

  function hydrateRateLimitForm(settings: RateLimitSetting) {
    lastSyncedRateLimitSettings.value = {
      window_seconds: settings.window_seconds,
      max_requests: settings.max_requests,
    }
    rateLimitForm.value.window_seconds = settings.window_seconds
    rateLimitForm.value.max_requests = settings.max_requests
  }

  function resetRateLimitForm() {
    rateLimitLifecycle.reset()
  }

  async function handleToggleRegistration(newValue: boolean) {
    const fallbackEnabled = registrationSettingQuery.data.value?.data?.enabled ?? !newValue
    try {
      await updateRegistrationSettingMutation.mutateAsync(newValue)
      options.feedback.registrationUpdated(newValue)
    } catch (error) {
      registrationEnabled.value = fallbackEnabled
      options.feedback.registrationUpdateFailed(error)
    }
  }

  return {
    rateLimitForm,
    rateLimitLoading,
    rateLimitSaving,
    registrationEnabled,
    registrationSettingLoading,
    handleSaveRateLimit,
    handleToggleRegistration,
    resetRateLimitForm,
  }
}

export function validateRateLimitSettings(settings: AdminAccessRateLimitForm) {
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

function isSameRateLimitSetting(
  current: AdminAccessRateLimitForm,
  synced: RateLimitSetting,
): boolean {
  return (
    current.window_seconds === synced.window_seconds &&
    current.max_requests === synced.max_requests
  )
}
