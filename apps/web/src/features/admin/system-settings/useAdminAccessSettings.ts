import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'
import {
  getGetAdminRateLimitSettingsQueryKey,
  getGetAdminRegistrationSettingQueryKey,
  updateAdminRateLimitSettings,
  updateAdminRegistrationSetting,
  useGetAdminRateLimitSettings,
  useGetAdminRegistrationSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseRegistrationSetting,
  ApiResponseRegistrationSettingData,
  RateLimitSetting,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'
import type { AdminAccessSettingsFeedback } from './useAdminSystemSettingsFeedback'

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
  const rateLimitForm = ref<AdminAccessRateLimitForm>(
    cloneRateLimitForm(DEFAULT_RATE_LIMIT_FORM),
  ) as Ref<AdminAccessRateLimitForm>
  const lastSyncedRateLimitForm = ref<AdminAccessRateLimitForm>(
    cloneRateLimitForm(DEFAULT_RATE_LIMIT_FORM),
  ) as Ref<AdminAccessRateLimitForm>
  const rateLimitValidationError = ref('')

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
    mutationFn: (enabled: boolean) =>
      resolveApiData(updateAdminRegistrationSetting({ enabled }), '更新注册状态失败'),
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
        updateAdminRateLimitSettings({
          window_seconds: Number(draft.window_seconds),
          max_requests: Number(draft.max_requests),
        }),
        '保存访问限流失败',
      ),
    onSuccess: async (savedSetting) => {
      hydrateRateLimit(savedSetting)
      writeRateLimitSettingQueryCache(queryClient, savedSetting)
      await queryClient.invalidateQueries({ queryKey: getGetAdminRateLimitSettingsQueryKey() })
      if (rateLimitSettingsQuery.data.value) hydrateRateLimit(rateLimitSettingsQuery.data.value)
    },
  })
  const rateLimitSaving = computed(() => saveRateLimitMutation.isPending.value)
  const rateLimitLoading = computed(
    () => rateLimitSettingsQuery.isLoading.value || rateLimitSettingsQuery.isFetching.value,
  )
  const rateLimitDirty = computed(
    () => !isSameRateLimitSetting(rateLimitForm.value, lastSyncedRateLimitForm.value),
  )

  function hydrateRateLimit(savedSetting: Partial<AdminAccessRateLimitForm> | null | undefined) {
    const nextForm = normalizeRateLimitSetting(savedSetting)
    lastSyncedRateLimitForm.value = cloneRateLimitForm(nextForm)
    rateLimitForm.value = cloneRateLimitForm(nextForm)
    rateLimitValidationError.value = ''
  }

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

  async function saveRateLimit(): Promise<boolean> {
    const validationMessage = validateRateLimitSettings(rateLimitForm.value)
    if (validationMessage) {
      rateLimitValidationError.value = validationMessage
      options.feedback.rateLimitValidationFailed(validationMessage)
      return false
    }
    try {
      await saveRateLimitMutation.mutateAsync(normalizeRateLimitSetting(rateLimitForm.value))
      options.feedback.rateLimitSaved()
      return true
    } catch (error) {
      options.feedback.rateLimitSaveFailed(error)
      return false
    }
  }

  function resetRateLimit() {
    rateLimitForm.value = cloneRateLimitForm(lastSyncedRateLimitForm.value)
    rateLimitValidationError.value = ''
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

  watch(rateLimitForm, () => (rateLimitValidationError.value = ''), {
    deep: true,
    flush: 'sync',
  })
  watch(
    () => rateLimitSettingsQuery.error.value,
    (error) => {
      if (error) options.feedback.loadFailed(error)
    },
  )
  watch(
    rateLimitSource,
    (savedSetting) => {
      if (!savedSetting || rateLimitSaving.value || rateLimitDirty.value) return
      hydrateRateLimit(savedSetting)
    },
    { immediate: true },
  )

  return {
    rateLimitDirty,
    rateLimitForm,
    rateLimitLoading,
    rateLimitSaving,
    rateLimitValidationError,
    registrationEnabled,
    registrationSettingLoading,
    handleToggleRegistration,
    resetRateLimit,
    saveRateLimit,
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
  queryClient.setQueryData(getGetAdminRateLimitSettingsQueryKey(), {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}

function cloneRateLimitForm(form: AdminAccessRateLimitForm): AdminAccessRateLimitForm {
  return structuredClone(toRaw(form))
}
