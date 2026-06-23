import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import {
  getGetAdminPublicURLSettingQueryKey,
  useGetAdminPublicURLSetting,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponsePublicURLSetting,
  PublicURLSetting,
} from '../../../../shared/api/generated-query/model'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import { updatePublicURLSetting } from '../queries/adminActions'

type AdminPublicURLSettingsMessage = {
  error: (message: string) => void
  success: (message: string) => void
}

export const ADMIN_PUBLIC_URL_MAX_LENGTH = 2048

export const DEFAULT_ADMIN_PUBLIC_URL_SETTING: PublicURLSetting = {
  public_base_url: '',
  public_asset_base_url: '',
}

export function useAdminPublicURLSettings(options: {
  message: AdminPublicURLSettingsMessage
}) {
  const queryClient = useQueryClient()
  const query = useGetAdminPublicURLSetting()
  const form = ref<PublicURLSetting>({ ...DEFAULT_ADMIN_PUBLIC_URL_SETTING })
  const lastSyncedForm = ref<PublicURLSetting>({ ...DEFAULT_ADMIN_PUBLIC_URL_SETTING })

  function invalidatePublicURLQuery() {
    return queryClient.invalidateQueries({ queryKey: getGetAdminPublicURLSettingQueryKey() })
  }

  function writePublicURLQueryCache(setting: PublicURLSetting) {
    queryClient.setQueryData<ApiResponsePublicURLSetting>(
      getGetAdminPublicURLSettingQueryKey(),
      (previous) => ({
        code: previous?.code ?? 0,
        msg: previous?.msg ?? 'ok',
        data: setting,
      }),
    )
  }

  const saveMutation = useMutation({
    mutationFn: updatePublicURLSetting,
    onSuccess: async (setting) => {
      const normalizedSetting = hydratePublicURLForm(setting)
      writePublicURLQueryCache(normalizedSetting)
      await invalidatePublicURLQuery()
    },
  })

  const loading = computed(() => query.isLoading.value || query.isFetching.value)
  const saving = computed(() => saveMutation.isPending.value)
  const formDirty = computed(() => !isSamePublicURLSetting(form.value, lastSyncedForm.value))

  watch(
    () => query.error.value,
    (error) => {
      if (error) {
        options.message.error(formatDisplayErrorMessage(error, '加载链接与域名失败'))
      }
    },
  )

  watch(
    () => query.data.value,
    (response) => {
      if (!response?.data || saveMutation.isPending.value || formDirty.value) return
      hydratePublicURLForm(response.data)
    },
    { immediate: true },
  )

  async function handleSavePublicURLSettings() {
    const validationError = validateAdminPublicURLSetting(form.value)
    if (validationError) {
      options.message.error(validationError)
      return
    }

    try {
      await saveMutation.mutateAsync(normalizeAdminPublicURLSetting(form.value))
      options.message.success('链接与域名已保存')
    } catch (error) {
      options.message.error(formatDisplayErrorMessage(error, '保存链接与域名失败'))
    }
  }

  function hydratePublicURLForm(setting: Partial<PublicURLSetting> | null | undefined) {
    const normalizedSetting = normalizeAdminPublicURLSetting(setting)
    lastSyncedForm.value = { ...normalizedSetting }
    form.value = { ...normalizedSetting }
    return normalizedSetting
  }

  return {
    form,
    handleSavePublicURLSettings,
    loading,
    saving,
  }
}

function isSamePublicURLSetting(current: PublicURLSetting, synced: PublicURLSetting): boolean {
  return (
    current.public_base_url === synced.public_base_url &&
    current.public_asset_base_url === synced.public_asset_base_url
  )
}

export function normalizeAdminPublicURLSetting(
  setting: Partial<PublicURLSetting> | null | undefined,
): PublicURLSetting {
  return {
    public_base_url: normalizePublicURL(setting?.public_base_url),
    public_asset_base_url: normalizePublicURL(setting?.public_asset_base_url),
  }
}

export function validateAdminPublicURLSetting(setting: PublicURLSetting): string {
  const publicBaseURL = setting.public_base_url.trim()
  const publicAssetBaseURL = setting.public_asset_base_url.trim()

  if (publicBaseURL.length > ADMIN_PUBLIC_URL_MAX_LENGTH) {
    return `前端访问地址不能超过 ${ADMIN_PUBLIC_URL_MAX_LENGTH} 个字符`
  }
  if (publicAssetBaseURL.length > ADMIN_PUBLIC_URL_MAX_LENGTH) {
    return `素材访问地址不能超过 ${ADMIN_PUBLIC_URL_MAX_LENGTH} 个字符`
  }
  if (!isValidAdminPublicBaseURL(publicBaseURL)) {
    return '前端访问地址必须为空或 http(s) URL'
  }
  if (!isValidAdminPublicBaseURL(publicAssetBaseURL)) {
    return '素材访问地址必须为空或 http(s) URL'
  }
  return ''
}

export function isValidAdminPublicBaseURL(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) return true

  try {
    const parsed = new URL(trimmed)
    return (
      (parsed.protocol === 'http:' || parsed.protocol === 'https:') &&
      !parsed.username &&
      !parsed.password &&
      !parsed.search &&
      !parsed.hash
    )
  } catch {
    return false
  }
}

function normalizePublicURL(value: unknown): string {
  if (typeof value !== 'string') return ''

  const trimmed = value.trim()
  if (!trimmed) return ''

  try {
    const parsed = new URL(trimmed)
    if (parsed.pathname === '/') {
      parsed.pathname = ''
    }
    return parsed.toString().replace(/\/+$/, '')
  } catch {
    return trimmed
  }
}
