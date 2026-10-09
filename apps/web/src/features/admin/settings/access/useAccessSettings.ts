import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getControlRateLimitSetting,
  getControlRegistrationSetting,
  updateControlRateLimitSetting,
  updateControlRegistrationSetting,
} from '../../../../shared/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../../shared/api/contractKernel'
import type { AdminAccessSettingsFeedback } from '../shared/useSettingsFeedback'
import { useSettingsLifecycle } from '../shared/useSettingsLifecycle'
import type { AdminConfirmPort } from '../../shared/composables/useAdminConfirm'
import { confirmSettingsSave } from '../../shared/lib/confirmSettingsSave'
import { withControlCopy } from '../../shared/lib/controlCopy'
import {
  buildControlMutationHeaders,
  requireOperatorReason,
} from '../../shared/lib/controlMutation'

// Admin access (rate limit + registration) boundary (W2 transports). Reads
// and writes go through the Control Plane with the contract envelope
// (`{data}`); every mutation carries a header-only Idempotency-Key plus an
// operator reason collected through the shared confirm dialog, and emits a
// control audit record. The deleted `/api/admin/settings/*` routes are
// gone; this module is the single transport.
export type RateLimitSetting = {
  window_seconds: number
  max_requests: number
}

export type RegistrationSettingData = {
  enabled: boolean
}

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

export function useAccessSettings(options: {
  feedback: AdminAccessSettingsFeedback
  confirm: AdminConfirmPort
}) {
  const queryClient = useQueryClient()
  const registrationSettingQuery = useQuery<RegistrationSettingData, Error>({
    queryKey: ADMIN_REGISTRATION_QUERY_KEY,
    queryFn: ({ signal }) => fetchRegistrationSetting(signal),
  })
  const rateLimitSettingsQuery = useQuery<RateLimitSetting, Error>({
    queryKey: ADMIN_RATE_LIMIT_QUERY_KEY,
    queryFn: ({ signal }) => fetchRateLimitSetting(signal),
  })
  const registrationEnabled = ref(DEFAULT_REGISTRATION_ENABLED)
  const rateLimitSource = computed(() => rateLimitSettingsQuery.data.value)

  function invalidateRegistrationSettingQuery() {
    return queryClient.invalidateQueries({ queryKey: ADMIN_REGISTRATION_QUERY_KEY })
  }

  function writeRegistrationSettingQueryCache(setting: RegistrationSettingData) {
    queryClient.setQueryData<RegistrationSettingData>(ADMIN_REGISTRATION_QUERY_KEY, setting)
  }

  const updateRegistrationSettingMutation = useMutation({
    mutationFn: (input: { enabled: boolean; operatorReason: string }) =>
      saveRegistrationSetting(input.enabled, input.operatorReason),
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
  // The lifecycle save runs inside saveRateLimit below with the reason
  // collected first; the ref bridges the two without changing the
  // lifecycle contract.
  const pendingRateLimitReason = ref<string | null>(null)
  const saveRateLimitMutation = useMutation({
    mutationFn: (draft: AdminAccessRateLimitForm) =>
      saveRateLimitSetting(
        {
          window_seconds: Number(draft.window_seconds),
          max_requests: Number(draft.max_requests),
        },
        pendingRateLimitReason.value ?? '',
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

  async function saveRateLimit(): Promise<boolean> {
    const operatorReason = await confirmSettingsSave(options.confirm, {
      title: '保存访问限流',
      message: '确定保存 API 访问限流配置吗？该操作将记录操作人与原因。',
      confirmLabel: '保存',
    })
    if (!operatorReason) return false
    pendingRateLimitReason.value = operatorReason
    try {
      return await rateLimitLifecycle.save()
    } finally {
      pendingRateLimitReason.value = null
    }
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

  async function handleToggleRegistration(newValue: boolean) {
    const fallbackEnabled = registrationSettingQuery.data.value?.enabled ?? !newValue
    const operatorReason = await confirmSettingsSave(options.confirm, {
      title: newValue ? '开放注册' : '关闭注册',
      message: newValue
        ? '确定允许新用户自行注册账号吗？该操作将记录操作人与原因。'
        : '确定关闭开放注册吗？关闭后仅系统管理员可创建用户。该操作将记录操作人与原因。',
      confirmLabel: newValue ? '开放注册' : '关闭注册',
    })
    if (!operatorReason) {
      registrationEnabled.value = fallbackEnabled
      return
    }
    try {
      await updateRegistrationSettingMutation.mutateAsync({
        enabled: newValue,
        operatorReason,
      })
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
    saveRateLimit,
  }
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

async function saveRegistrationSetting(
  enabled: boolean,
  operatorReason: string,
): Promise<RegistrationSettingData> {
  const reason = requireOperatorReason(operatorReason)
  try {
    const body = await updateControlRegistrationSetting(
      { enabled },
      { headers: buildControlMutationHeaders({ operatorReason: reason }) },
    )
    return requireContractData<RegistrationSettingData>(body, '更新注册状态失败')
  } catch (error) {
    throw withControlCopy(error, '更新注册状态失败')
  }
}

async function saveRateLimitSetting(
  draft: RateLimitSetting,
  operatorReason: string,
): Promise<RateLimitSetting> {
  const reason = requireOperatorReason(operatorReason)
  try {
    const body = await updateControlRateLimitSetting(
      { window_seconds: draft.window_seconds, max_requests: draft.max_requests },
      { headers: buildControlMutationHeaders({ operatorReason: reason }) },
    )
    return requireContractData<RateLimitSetting>(body, '保存访问限流失败')
  } catch (error) {
    throw withControlCopy(error, '保存访问限流失败')
  }
}

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.

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
