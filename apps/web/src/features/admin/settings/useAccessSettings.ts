import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import { api } from '../../../shared/api/client'
import {
  createApiDataSelector,
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import type { AdminAccessSettingsFeedback } from './useSettingsFeedback'
import { useSettingsLifecycle } from './useSettingsLifecycle'

// Admin access (rate limit + registration) boundary (issue #809). The
// generated `admin-settings` client is gone but the backend admin plane
// still serves `/api/admin/settings/rate-limit` and
// `/api/admin/settings/registration` with the shared envelope, so this
// composable transports through the central `api.request` with local view
// types. Control-plane mutations with operator reasons stay a residual.
export type RateLimitSetting = {
  window_seconds: number
  max_requests: number
}

export type RegistrationSettingData = {
  enabled: boolean
}

export type ApiResponseRegistrationSetting = ApiResponseWithData<RegistrationSettingData>
export type ApiResponseRegistrationSettingData = RegistrationSettingData

const ADMIN_RATE_LIMIT_QUERY_KEY = ['admin', 'settings', 'rate-limit'] as const
const ADMIN_REGISTRATION_QUERY_KEY = ['admin', 'settings', 'registration'] as const

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

export function useAccessSettings(options: { feedback: AdminAccessSettingsFeedback }) {
  const queryClient = useQueryClient()
  const registrationSettingQuery = useQuery<
    ApiResponseRegistrationSetting,
    Error,
    ApiResponseRegistrationSettingData
  >({
    queryKey: ADMIN_REGISTRATION_QUERY_KEY,
    queryFn: ({ signal }) =>
      api.request<ApiResponseRegistrationSetting>('/admin/settings/registration', {
        method: 'GET',
        ...(signal == null ? {} : { signal }),
      }),
    select: selectRegistrationSetting,
  })
  const rateLimitSettingsQuery = useQuery<
    ApiResponseWithData<RateLimitSetting>,
    Error,
    RateLimitSetting
  >({
    queryKey: ADMIN_RATE_LIMIT_QUERY_KEY,
    queryFn: ({ signal }) =>
      api.request<ApiResponseWithData<RateLimitSetting>>('/admin/settings/rate-limit', {
        method: 'GET',
        ...(signal == null ? {} : { signal }),
      }),
    select: selectRateLimitSetting,
  })
  const registrationEnabled = ref(DEFAULT_REGISTRATION_ENABLED)
  const rateLimitSource = computed(() => rateLimitSettingsQuery.data.value)

  function invalidateRegistrationSettingQuery() {
    return queryClient.invalidateQueries({ queryKey: ADMIN_REGISTRATION_QUERY_KEY })
  }

  function writeRegistrationSettingQueryCache(setting: ApiResponseRegistrationSettingData) {
    queryClient.setQueryData<ApiResponseRegistrationSetting>(
      ADMIN_REGISTRATION_QUERY_KEY,
      (previous) => ({
        code: previous?.code ?? 0,
        msg: previous?.msg ?? 'ok',
        data: setting,
      }),
    )
  }

  const updateRegistrationSettingMutation = useMutation({
    mutationFn: (enabled: boolean) =>
      resolveApiData(
        api.request<ApiResponseRegistrationSetting>('/admin/settings/registration', {
          method: 'PUT',
          body: { enabled } as unknown as Record<string, unknown>,
        }),
        '更新注册状态失败',
      ),
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
  const saveRateLimitMutation = useMutation({
    mutationFn: (draft: AdminAccessRateLimitForm) =>
      resolveApiData(
        api.request<ApiResponseWithData<RateLimitSetting>>('/admin/settings/rate-limit', {
          method: 'PUT',
          body: {
            window_seconds: Number(draft.window_seconds),
            max_requests: Number(draft.max_requests),
          } as unknown as Record<string, unknown>,
        }),
        '保存访问限流失败',
      ),
  })
  const rateLimitSaving = computed(() => saveRateLimitMutation.isPending.value)
  const rateLimitLoading = computed(
    () => rateLimitSettingsQuery.isLoading.value || rateLimitSettingsQuery.isFetching.value,
  )
  const rateLimitLifecycle = useSettingsLifecycle<AdminAccessRateLimitForm>({
    defaultValue: DEFAULT_RATE_LIMIT_FORM,
    source: rateLimitSource,
    loadError: rateLimitSettingsQuery.error,
    loading: rateLimitLoading,
    saving: rateLimitSaving,
    normalize: normalizeRateLimitSetting,
    isSame: isSameRateLimitSetting,
    validate: validateRateLimitSettings,
    save: async (draft) =>
      normalizeRateLimitSetting(
        await saveRateLimitMutation.mutateAsync({
          window_seconds: Number(draft.window_seconds),
          max_requests: Number(draft.max_requests),
        }),
      ),
    synchronizeCache: async (savedSetting) => {
      writeRateLimitSettingQueryCache(queryClient, {
        window_seconds: Number(savedSetting.window_seconds),
        max_requests: Number(savedSetting.max_requests),
      })
      await queryClient.invalidateQueries({ queryKey: ADMIN_RATE_LIMIT_QUERY_KEY })
    },
    feedback: {
      loadFailed: options.feedback.loadFailed,
      validationFailed: options.feedback.rateLimitValidationFailed,
      saved: options.feedback.rateLimitSaved,
      saveFailed: options.feedback.rateLimitSaveFailed,
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
    (setting) => {
      if (!setting || updateRegistrationSettingMutation.isPending.value) return
      registrationEnabled.value = setting.enabled
    },
    { immediate: true },
  )

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
    rateLimitDirty: rateLimitLifecycle.isDirty,
    rateLimitForm: rateLimitLifecycle.form,
    rateLimitLoading: rateLimitLifecycle.loading,
    rateLimitSaving: rateLimitLifecycle.saving,
    rateLimitValidationError: rateLimitLifecycle.validationError,
    registrationEnabled,
    registrationSettingLoading,
    handleToggleRegistration,
    resetRateLimit: rateLimitLifecycle.reset,
    saveRateLimit: rateLimitLifecycle.save,
  }
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

function writeRateLimitSettingQueryCache(queryClient: QueryClient, setting: RateLimitSetting) {
  queryClient.setQueryData(ADMIN_RATE_LIMIT_QUERY_KEY, {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}
