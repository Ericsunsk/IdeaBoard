import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getControlLoginLockoutSetting,
  getControlRateLimitSetting,
  getControlRegistrationSetting,
  updateControlLoginLockoutSetting,
  updateControlRateLimitSetting,
  updateControlRegistrationSetting,
} from '@/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '@/api/contractKernel'
import type { AdminAccessSettingsFeedback } from '../useSettingsFeedback'
import { useSettingsLifecycle } from '../useSettingsLifecycle'
import { withControlCopy } from '../../../shared/index'
import { buildControlMutationHeaders } from '../../../shared/index'

type RateLimitSetting = {
  window_seconds: number
  max_requests: number
}

type RegistrationSettingData = {
  enabled: boolean
}

type LoginLockoutSetting = {
  threshold: number
  duration_seconds: number
}

const ADMIN_RATE_LIMIT_QUERY_KEY = ['admin', 'settings', 'rate-limit'] as const
const ADMIN_REGISTRATION_QUERY_KEY = ['admin', 'settings', 'registration'] as const
const ADMIN_LOGIN_LOCKOUT_QUERY_KEY = ['admin', 'settings', 'login-lockout'] as const

export const RATE_LIMIT_MAX_REQUESTS = 10_000
export const RATE_LIMIT_MAX_WINDOW_SECONDS = 3_600

export const LOGIN_LOCKOUT_MIN_THRESHOLD = 3
export const LOGIN_LOCKOUT_MAX_THRESHOLD = 100
export const LOGIN_LOCKOUT_MIN_DURATION_SECONDS = 60
export const LOGIN_LOCKOUT_MAX_DURATION_SECONDS = 86_400

type AdminAccessRateLimitForm = {
  window_seconds: number | null
  max_requests: number | null
}

type AdminAccessLoginLockoutForm = {
  threshold: number | null
  duration_seconds: number | null
}

const DEFAULT_RATE_LIMIT_SETTING: RateLimitSetting = {
  window_seconds: 2,
  max_requests: 120,
}
const DEFAULT_RATE_LIMIT_FORM: AdminAccessRateLimitForm = {
  ...DEFAULT_RATE_LIMIT_SETTING,
}
const DEFAULT_LOGIN_LOCKOUT_SETTING: LoginLockoutSetting = {
  threshold: 10,
  duration_seconds: 900,
}
const DEFAULT_LOGIN_LOCKOUT_FORM: AdminAccessLoginLockoutForm = {
  ...DEFAULT_LOGIN_LOCKOUT_SETTING,
}
const DEFAULT_REGISTRATION_ENABLED = false

export function useAccessSettings(options: { feedback: AdminAccessSettingsFeedback }) {
  const queryClient = useQueryClient()
  const registration = useRegistrationSetting(queryClient, options)
  const rateLimit = useRateLimitSetting(queryClient, options)
  const loginLockout = useLoginLockoutSetting(queryClient, options)

  return {
    rateLimitDirty: rateLimit.lifecycle.isDirty,
    rateLimitForm: rateLimit.lifecycle.form,
    rateLimitLoading: rateLimit.lifecycle.isLoading,
    rateLimitSaving: rateLimit.lifecycle.isSaving,
    rateLimitValidationError: rateLimit.lifecycle.validationError,
    loginLockoutDirty: loginLockout.lifecycle.isDirty,
    loginLockoutForm: loginLockout.lifecycle.form,
    loginLockoutLoading: loginLockout.lifecycle.isLoading,
    loginLockoutSaving: loginLockout.lifecycle.isSaving,
    loginLockoutValidationError: loginLockout.lifecycle.validationError,
    registrationEnabled: registration.enabled,
    registrationSettingLoading: registration.isLoading,
    handleToggleRegistration: registration.handleToggle,
    resetRateLimit: rateLimit.lifecycle.reset,
    saveRateLimit: rateLimit.save,
    resetLoginLockout: loginLockout.lifecycle.reset,
    saveLoginLockout: loginLockout.save,
  }
}

function useRegistrationSetting(
  queryClient: QueryClient,
  options: { feedback: AdminAccessSettingsFeedback },
) {
  const registrationSettingQuery = useQuery<RegistrationSettingData, Error>({
    queryKey: ADMIN_REGISTRATION_QUERY_KEY,
    queryFn: ({ signal }) => fetchRegistrationSetting(signal),
  })
  const registrationEnabled = ref(DEFAULT_REGISTRATION_ENABLED)

  function invalidateRegistrationSettingQuery() {
    return queryClient.invalidateQueries({ queryKey: ADMIN_REGISTRATION_QUERY_KEY })
  }

  function writeRegistrationSettingQueryCache(setting: RegistrationSettingData) {
    queryClient.setQueryData<RegistrationSettingData>(ADMIN_REGISTRATION_QUERY_KEY, setting)
  }

  const updateRegistrationSettingMutation = useMutation({
    mutationFn: (input: { enabled: boolean }) => saveRegistrationSetting(input.enabled),
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
      await updateRegistrationSettingMutation.mutateAsync({
        enabled: newValue,
      })
      options.feedback.registrationUpdated(newValue)
    } catch (error) {
      registrationEnabled.value = fallbackEnabled
      options.feedback.registrationUpdateFailed(error)
    }
  }

  return {
    enabled: registrationEnabled,
    isLoading: registrationSettingLoading,
    handleToggle: handleToggleRegistration,
  }
}

function useRateLimitSetting(
  queryClient: QueryClient,
  options: { feedback: AdminAccessSettingsFeedback },
) {
  const rateLimitSettingsQuery = useQuery<RateLimitSetting, Error>({
    queryKey: ADMIN_RATE_LIMIT_QUERY_KEY,
    queryFn: ({ signal }) => fetchRateLimitSetting(signal),
  })
  const rateLimitSource = computed(() => rateLimitSettingsQuery.data.value)

  const saveRateLimitMutation = useMutation({
    mutationFn: (draft: AdminAccessRateLimitForm) =>
      saveRateLimitSetting({
        window_seconds: Number(draft.window_seconds),
        max_requests: Number(draft.max_requests),
      }),
  })
  const rateLimitSaving = computed(() => saveRateLimitMutation.isPending.value)
  const rateLimitLoading = computed(
    () => rateLimitSettingsQuery.isLoading.value || rateLimitSettingsQuery.isFetching.value,
  )
  const rateLimitLifecycle = useSettingsLifecycle<AdminAccessRateLimitForm>({
    defaultValue: DEFAULT_RATE_LIMIT_FORM,
    source: rateLimitSource,
    loadError: rateLimitSettingsQuery.error,
    isLoading: rateLimitLoading,
    isSaving: rateLimitSaving,
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

  async function saveRateLimit(): Promise<boolean> {
    return rateLimitLifecycle.save()
  }

  return { lifecycle: rateLimitLifecycle, save: saveRateLimit }
}

function useLoginLockoutSetting(
  queryClient: QueryClient,
  options: { feedback: AdminAccessSettingsFeedback },
) {
  const loginLockoutSettingsQuery = useQuery<LoginLockoutSetting, Error>({
    queryKey: ADMIN_LOGIN_LOCKOUT_QUERY_KEY,
    queryFn: ({ signal }) => fetchLoginLockoutSetting(signal),
  })
  const loginLockoutSource = computed(() => loginLockoutSettingsQuery.data.value)

  // Login lockout mirrors the rate-limit lifecycle with an independent
  // query/mutation pair and its own dirty tracking; the two forms never
  // share a draft.
  const saveLoginLockoutMutation = useMutation({
    mutationFn: (draft: AdminAccessLoginLockoutForm) =>
      saveLoginLockoutSetting({
        threshold: Number(draft.threshold),
        duration_seconds: Number(draft.duration_seconds),
      }),
  })
  const loginLockoutSaving = computed(() => saveLoginLockoutMutation.isPending.value)
  const loginLockoutLoading = computed(
    () => loginLockoutSettingsQuery.isLoading.value || loginLockoutSettingsQuery.isFetching.value,
  )
  const loginLockoutLifecycle = useSettingsLifecycle<AdminAccessLoginLockoutForm>({
    defaultValue: DEFAULT_LOGIN_LOCKOUT_FORM,
    source: loginLockoutSource,
    loadError: loginLockoutSettingsQuery.error,
    isLoading: loginLockoutLoading,
    isSaving: loginLockoutSaving,
    normalize: normalizeLoginLockoutSetting,
    isSame: isSameLoginLockoutSetting,
    validate: validateLoginLockoutSettings,
    save: async (draft) =>
      normalizeLoginLockoutSetting(
        await saveLoginLockoutMutation.mutateAsync({
          threshold: Number(draft.threshold),
          duration_seconds: Number(draft.duration_seconds),
        }),
      ),
    synchronizeCache: async (savedSetting) => {
      writeLoginLockoutSettingQueryCache(queryClient, {
        threshold: Number(savedSetting.threshold),
        duration_seconds: Number(savedSetting.duration_seconds),
      })
      await queryClient.invalidateQueries({ queryKey: ADMIN_LOGIN_LOCKOUT_QUERY_KEY })
    },
    feedback: {
      loadFailed: options.feedback.loadFailed,
      validationFailed: options.feedback.loginLockoutValidationFailed,
      saved: options.feedback.loginLockoutSaved,
      saveFailed: options.feedback.loginLockoutSaveFailed,
    },
  })

  async function saveLoginLockout(): Promise<boolean> {
    return loginLockoutLifecycle.save()
  }

  return { lifecycle: loginLockoutLifecycle, save: saveLoginLockout }
}

async function fetchRegistrationSetting(signal?: AbortSignal): Promise<RegistrationSettingData> {
  try {
    const body = await getControlRegistrationSetting({ signal })
    return requireContractData<RegistrationSettingData>(body, '加载注册设置失败')
  } catch (error) {
    throw withControlCopy(error, '加载注册设置失败')
  }
}

async function fetchRateLimitSetting(signal?: AbortSignal): Promise<RateLimitSetting> {
  try {
    const body = await getControlRateLimitSetting({ signal })
    return requireContractData<RateLimitSetting>(body, '加载访问限流失败')
  } catch (error) {
    throw withControlCopy(error, '加载访问限流失败')
  }
}

async function fetchLoginLockoutSetting(signal?: AbortSignal): Promise<LoginLockoutSetting> {
  try {
    const body = await getControlLoginLockoutSetting({ signal })
    return requireContractData<LoginLockoutSetting>(body, '加载登录锁定失败')
  } catch (error) {
    throw withControlCopy(error, '加载登录锁定失败')
  }
}

async function saveRegistrationSetting(enabled: boolean): Promise<RegistrationSettingData> {
  try {
    const body = await updateControlRegistrationSetting(
      { enabled },
      { headers: buildControlMutationHeaders() },
    )
    return requireContractData<RegistrationSettingData>(body, '更新注册状态失败')
  } catch (error) {
    throw withControlCopy(error, '更新注册状态失败')
  }
}

async function saveRateLimitSetting(draft: RateLimitSetting): Promise<RateLimitSetting> {
  try {
    const body = await updateControlRateLimitSetting(
      { window_seconds: draft.window_seconds, max_requests: draft.max_requests },
      { headers: buildControlMutationHeaders() },
    )
    return requireContractData<RateLimitSetting>(body, '保存访问限流失败')
  } catch (error) {
    throw withControlCopy(error, '保存访问限流失败')
  }
}

async function saveLoginLockoutSetting(draft: LoginLockoutSetting): Promise<LoginLockoutSetting> {
  try {
    const body = await updateControlLoginLockoutSetting(
      { threshold: draft.threshold, duration_seconds: draft.duration_seconds },
      { headers: buildControlMutationHeaders() },
    )
    return requireContractData<LoginLockoutSetting>(body, '保存登录锁定失败')
  } catch (error) {
    throw withControlCopy(error, '保存登录锁定失败')
  }
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.

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

function isSameRateLimitSetting(
  current: AdminAccessRateLimitForm,
  synced: AdminAccessRateLimitForm,
): boolean {
  return (
    current.window_seconds === synced.window_seconds && current.max_requests === synced.max_requests
  )
}

function writeRateLimitSettingQueryCache(queryClient: QueryClient, setting: RateLimitSetting) {
  queryClient.setQueryData(ADMIN_RATE_LIMIT_QUERY_KEY, setting)
}

function validateLoginLockoutSettings(settings: AdminAccessLoginLockoutForm) {
  const threshold = Number(settings.threshold)
  const durationSeconds = Number(settings.duration_seconds)
  if (!Number.isInteger(threshold)) {
    return `锁定阈值需为 ${LOGIN_LOCKOUT_MIN_THRESHOLD} 到 ${LOGIN_LOCKOUT_MAX_THRESHOLD} 之间的整数`
  }
  if (!Number.isInteger(durationSeconds)) {
    return `锁定时长需为 ${LOGIN_LOCKOUT_MIN_DURATION_SECONDS} 到 ${LOGIN_LOCKOUT_MAX_DURATION_SECONDS} 秒之间的整数`
  }
  return ''
}

function normalizeLoginLockoutSetting(
  setting: Partial<AdminAccessLoginLockoutForm> | null | undefined,
): AdminAccessLoginLockoutForm {
  return {
    threshold: clampBoundedInt(
      setting?.threshold,
      LOGIN_LOCKOUT_MIN_THRESHOLD,
      LOGIN_LOCKOUT_MAX_THRESHOLD,
      DEFAULT_LOGIN_LOCKOUT_SETTING.threshold,
    ),
    duration_seconds: clampBoundedInt(
      setting?.duration_seconds,
      LOGIN_LOCKOUT_MIN_DURATION_SECONDS,
      LOGIN_LOCKOUT_MAX_DURATION_SECONDS,
      DEFAULT_LOGIN_LOCKOUT_SETTING.duration_seconds,
    ),
  }
}

function isSameLoginLockoutSetting(
  current: AdminAccessLoginLockoutForm,
  synced: AdminAccessLoginLockoutForm,
): boolean {
  return (
    current.threshold === synced.threshold && current.duration_seconds === synced.duration_seconds
  )
}

function clampBoundedInt(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.min(max, Math.max(min, Math.trunc(value)))
}

function writeLoginLockoutSettingQueryCache(
  queryClient: QueryClient,
  setting: LoginLockoutSetting,
) {
  queryClient.setQueryData(ADMIN_LOGIN_LOCKOUT_QUERY_KEY, setting)
}
