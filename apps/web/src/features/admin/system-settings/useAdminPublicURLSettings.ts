import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'
import {
  getGetAdminPublicURLSettingQueryKey,
  updateAdminPublicURLSetting,
  useGetAdminPublicURLSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { PublicURLSetting } from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'
import type { AdminPublicURLSettingsFeedback } from './useAdminSystemSettingsFeedback'

export const ADMIN_PUBLIC_URL_MAX_LENGTH = 2048

export const DEFAULT_ADMIN_PUBLIC_URL_SETTING: PublicURLSetting = {
  public_base_url: '',
  public_asset_base_url: '',
}

const selectPublicURLSetting = createApiDataSelector<PublicURLSetting>('加载链接与域名失败')

export function useAdminPublicURLSettings(options: { feedback: AdminPublicURLSettingsFeedback }) {
  const queryClient = useQueryClient()
  const query = useGetAdminPublicURLSetting<PublicURLSetting>({
    query: { select: selectPublicURLSetting },
  })
  const setting = computed(() => query.data.value)
  const form = ref<PublicURLSetting>(
    cloneSetting(DEFAULT_ADMIN_PUBLIC_URL_SETTING),
  ) as Ref<PublicURLSetting>
  const lastSyncedForm = ref<PublicURLSetting>(
    cloneSetting(DEFAULT_ADMIN_PUBLIC_URL_SETTING),
  ) as Ref<PublicURLSetting>
  const validationError = ref('')
  const loading = computed(() => query.isLoading.value || query.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: PublicURLSetting) =>
      resolveApiData(updateAdminPublicURLSetting(draft), '保存链接与域名失败'),
    onSuccess: async (savedSetting) => {
      hydrate(savedSetting)
      writePublicURLQueryCache(queryClient, savedSetting)
      await queryClient.invalidateQueries({ queryKey: getGetAdminPublicURLSettingQueryKey() })
      if (query.data.value) hydrate(query.data.value)
    },
  })
  const saving = computed(() => saveMutation.isPending.value)
  const isDirty = computed(() => !isSamePublicURLSetting(form.value, lastSyncedForm.value))

  function hydrate(savedSetting: Partial<PublicURLSetting> | null | undefined) {
    const nextForm = normalizeAdminPublicURLSetting(savedSetting)
    lastSyncedForm.value = cloneSetting(nextForm)
    form.value = cloneSetting(nextForm)
    validationError.value = ''
  }

  async function save(): Promise<boolean> {
    const validationMessage = validateAdminPublicURLSetting(form.value)
    if (validationMessage) {
      validationError.value = validationMessage
      options.feedback.validationFailed(validationMessage)
      return false
    }
    try {
      await saveMutation.mutateAsync(normalizeAdminPublicURLSetting(form.value))
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
    setting,
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

function writePublicURLQueryCache(queryClient: QueryClient, setting: PublicURLSetting) {
  queryClient.setQueryData(getGetAdminPublicURLSettingQueryKey(), {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}

function cloneSetting(setting: PublicURLSetting): PublicURLSetting {
  return structuredClone(toRaw(setting))
}
