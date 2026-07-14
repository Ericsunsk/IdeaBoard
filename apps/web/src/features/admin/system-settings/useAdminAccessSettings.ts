import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getGetAdminRateLimitSettingsQueryKey,
  getGetAdminRegistrationSettingQueryKey,
  useGetAdminRateLimitSettings,
  useGetAdminRegistrationSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseRegistrationSetting,
  ApiResponseRegistrationSettingData,
  RateLimitSetting,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector } from '../../../shared/api/apiResponse'
import type { AdminAccessSettingsFeedback } from './useAdminSystemSettingsFeedback'
import { updateRateLimitSettings, updateRegistrationSetting } from './systemSettingsCommands'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

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
const selectRateLimitSetting = createApiDataSelector<RateLimitSetting>('加载访问限流失败')
const selectRegistrationSetting =
  createApiDataSelector<ApiResponseRegistrationSettingData>('加载注册设置失败')

export function useAdminAccessSettings(options: { feedback: AdminAccessSettingsFeedback }) {
  const queryClient = useQueryClient()
  const registrationSettingQuery =
    useGetAdminRegistrationSetting<ApiResponseRegistrationSettingData>({
      query: { select: selectRegistrationSetting },
    })
  const rateLimitSettingsQuery = useGetAdminRateLimitSettings<RateLimitSetting>({
    query: { select: selectRateLimitSetting },
  })
  const registrationEnabled = ref(DEFAULT_REGISTRATION_ENABLED)
  const rateLimitSource = computed(() => rateLimitSettingsQuery.data.value)

  function invalidateRegistrationSettingQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminRegistrationSettingQueryKey() })
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

  const updateRegistrationSettingMutation = useMutation({
    mutationFn: updateRegistrationSetting,
    onSuccess: async (setting) => {
      registrationEnabled.value = setting.enabled
      writeRegistrationSettingQueryCache(setting)
      await invalidateRegistrationSettingQuery()
    },
  })
  const registrationSettingLoading = computed(
    () =>
      registrationSettingQuery.isLoading.value ||
      registrationSettingQuery.isFetching.value ||
      updateRegistrationSettingMutation.isPending.value,
  )
  const rateLimitResource = useSettingResourceLifecycle<AdminAccessRateLimitForm>({
    defaultSetting: DEFAULT_RATE_LIMIT_FORM,
    feedback: {
      loadFailed: options.feedback.loadFailed,
      validationFailed: options.feedback.rateLimitValidationFailed,
      saved: options.feedback.rateLimitSaved,
      saveFailed: options.feedback.rateLimitSaveFailed,
    },
    isSame: isSameRateLimitSetting,
    normalize: normalizeRateLimitSetting,
    query: rateLimitSettingsQuery,
    queryKey: getGetAdminRateLimitSettingsQueryKey(),
    save: saveRateLimitSetting,
    source: rateLimitSource,
    validate: validateRateLimitSettings,
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
    (setting) => {
      if (!setting || updateRegistrationSettingMutation.isPending.value) return
      registrationEnabled.value = setting.enabled
    },
    { immediate: true },
  )

  async function handleSaveRateLimit() {
    await rateLimitResource.save()
  }

  function resetRateLimitForm() {
    rateLimitResource.reset()
  }

  async function handleToggleRegistration(newValue: boolean) {
    const fallbackEnabled = registrationSettingQuery.data.value?.enabled ?? !newValue
    try {
      await updateRegistrationSettingMutation.mutateAsync(newValue)
      options.feedback.registrationUpdated(newValue)
    } catch (error) {
      registrationEnabled.value = fallbackEnabled
      options.feedback.registrationUpdateFailed(error)
    }
  }

  return {
    rateLimitForm: rateLimitResource.form,
    rateLimitLoading: rateLimitResource.loading,
    rateLimitSaving: rateLimitResource.saving,
    registrationEnabled,
    registrationSettingLoading,
    handleSaveRateLimit,
    handleToggleRegistration,
    resetRateLimitForm,
  }
}

async function saveRateLimitSetting(
  setting: AdminAccessRateLimitForm,
): Promise<AdminAccessRateLimitForm> {
  return normalizeRateLimitSetting(
    await updateRateLimitSettings({
      window_seconds: Number(setting.window_seconds),
      max_requests: Number(setting.max_requests),
    }),
  )
}

function normalizeRateLimitSetting(
  setting: Partial<AdminAccessRateLimitForm> | null | undefined,
): AdminAccessRateLimitForm {
  return {
    window_seconds:
      typeof setting?.window_seconds === 'number' && Number.isFinite(setting.window_seconds)
        ? setting.window_seconds
        : DEFAULT_RATE_LIMIT_SETTING.window_seconds,
    max_requests:
      typeof setting?.max_requests === 'number' && Number.isFinite(setting.max_requests)
        ? setting.max_requests
        : DEFAULT_RATE_LIMIT_SETTING.max_requests,
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
  synced: AdminAccessRateLimitForm,
): boolean {
  return (
    current.window_seconds === synced.window_seconds && current.max_requests === synced.max_requests
  )
}
