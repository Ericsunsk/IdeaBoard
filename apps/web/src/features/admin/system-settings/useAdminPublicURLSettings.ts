import { computed } from 'vue'
import {
  getGetAdminPublicURLSettingQueryKey,
  useGetAdminPublicURLSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type { PublicURLSetting } from '../../../shared/api/generated-query/model'
import { createApiDataSelector } from '../../../shared/api/apiResponse'
import type { AdminPublicURLSettingsFeedback } from './useAdminSystemSettingsFeedback'
import { updatePublicURLSetting } from './systemSettingsCommands'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

export const ADMIN_PUBLIC_URL_MAX_LENGTH = 2048

export const DEFAULT_ADMIN_PUBLIC_URL_SETTING: PublicURLSetting = {
  public_base_url: '',
  public_asset_base_url: '',
}

const selectPublicURLSetting = createApiDataSelector<PublicURLSetting>('加载链接与域名失败')

export function useAdminPublicURLSettings(options: { feedback: AdminPublicURLSettingsFeedback }) {
  const query = useGetAdminPublicURLSetting<PublicURLSetting>({
    query: { select: selectPublicURLSetting },
  })
  const sourceSetting = computed(() => query.data.value)
  const resource = useSettingResourceLifecycle<PublicURLSetting>({
    defaultSetting: DEFAULT_ADMIN_PUBLIC_URL_SETTING,
    feedback: options.feedback,
    isSame: isSamePublicURLSetting,
    normalize: normalizeAdminPublicURLSetting,
    query,
    queryKey: getGetAdminPublicURLSettingQueryKey(),
    save: updatePublicURLSetting,
    source: sourceSetting,
    validate: validateAdminPublicURLSetting,
  })

  async function handleSavePublicURLSettings() {
    await resource.save()
  }

  function resetPublicURLForm() {
    resource.reset()
  }

  return {
    form: resource.form,
    handleSavePublicURLSettings,
    loading: resource.loading,
    resetPublicURLForm,
    saving: resource.saving,
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
