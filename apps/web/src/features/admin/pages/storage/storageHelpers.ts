import type {
  StorageProfile,
  StorageProfilePayload,
  StorageProvider,
  StorageProviderField,
  StorageProviderMeta,
} from '../../shared/api/storage'

export interface StorageCard {
  id: string
  name: string
  provider: StorageProvider
  enabled: boolean
  isCurrent: boolean
  builtin: boolean
  secretConfigured: boolean
  config: Record<string, unknown>
}

export type ProviderMetaMap = Partial<Record<StorageProvider, StorageProviderMeta>>

export function toProviderMetaMap(providers: StorageProviderMeta[]): ProviderMetaMap {
  return Object.fromEntries(
    providers.map((provider) => [provider.provider, provider]),
  ) as ProviderMetaMap
}

export function providerLabel(provider: StorageProvider, providerMap: ProviderMetaMap) {
  return providerMap[provider]?.label || provider
}

export function providerSupportsBucketInfo(
  provider: StorageProvider,
  providerMap: ProviderMetaMap,
) {
  return Boolean(providerMap[provider]?.supports_bucket_info)
}

export function providerSupportsTest(provider: StorageProvider, providerMap: ProviderMetaMap) {
  return Boolean(providerMap[provider]?.supports_test)
}

export function normalizeConfig(
  provider: StorageProvider,
  config: Record<string, unknown> | undefined,
  providerMap: ProviderMetaMap,
) {
  const fields = providerMap[provider]?.fields || []
  const next: Record<string, unknown> = {}

  for (const field of fields) {
    const raw = config?.[field.key]
    if (field.type === 'switch') {
      next[field.key] = Boolean(raw)
    } else {
      next[field.key] = typeof raw === 'string' ? raw : raw == null ? '' : String(raw)
    }
  }

  for (const [key, value] of Object.entries(config || {})) {
    if (!(key in next)) {
      next[key] = value
    }
  }

  return next
}

export function createEmptyProfile(
  provider: StorageProvider,
  providerMap: ProviderMetaMap,
): StorageCard {
  return {
    id: '',
    name: `${providerLabel(provider, providerMap)} 存储`,
    provider,
    enabled: provider === 'local',
    isCurrent: false,
    builtin: false,
    secretConfigured: false,
    config: normalizeConfig(provider, {}, providerMap),
  }
}

export function normalizeProfile(
  profile: StorageProfile,
  providerMap: ProviderMetaMap,
): StorageCard {
  const config = normalizeConfig(profile.provider, profile.config, providerMap)
  const secretFields = (providerMap[profile.provider]?.fields || []).filter((field) => field.secret)
  for (const field of secretFields) {
    config[field.key] = ''
  }

  return {
    id: profile.id,
    name: profile.name,
    provider: profile.provider,
    enabled: profile.enabled,
    isCurrent: profile.is_current,
    builtin: profile.builtin,
    secretConfigured: profile.secret_configured,
    config,
  }
}

export function configText(storage: StorageCard, key: string) {
  const value = storage.config[key]
  return typeof value === 'string' ? value : value == null ? '' : String(value)
}

export function configBool(storage: StorageCard, key: string) {
  return Boolean(storage.config[key])
}

function requiredFieldsFor(storage: StorageCard, providerMap: ProviderMetaMap) {
  const fields = providerMap[storage.provider]?.fields || []
  return fields.filter((field) => field.required)
}

function fieldSatisfied(storage: StorageCard, field: StorageProviderField) {
  if (field.type === 'switch') return true
  const raw = storage.config[field.key]
  const value = typeof raw === 'string' ? raw.trim() : raw
  if (field.secret) {
    return Boolean(value || storage.secretConfigured)
  }
  return Boolean(value)
}

export function hasConfiguredProfile(storage: StorageCard, providerMap: ProviderMetaMap) {
  if (storage.provider === 'local') return true
  return requiredFieldsFor(storage, providerMap).every((field) => fieldSatisfied(storage, field))
}

export function canApply(storage: StorageCard, providerMap: ProviderMetaMap) {
  if (storage.provider === 'local') return true
  return storage.enabled && hasConfiguredProfile(storage, providerMap)
}

export function canTest(storage: StorageCard, providerMap: ProviderMetaMap) {
  return (
    providerSupportsTest(storage.provider, providerMap) &&
    hasConfiguredProfile(storage, providerMap)
  )
}

export function statusText(storage: StorageCard, providerMap: ProviderMetaMap) {
  if (storage.isCurrent) return '当前存储'
  if (hasConfiguredProfile(storage, providerMap)) return '已配置'
  return '未配置'
}

export function statusClass(storage: StorageCard, providerMap: ProviderMetaMap) {
  if (storage.isCurrent) return 'is-current'
  if (hasConfiguredProfile(storage, providerMap)) return 'is-enabled'
  return 'is-empty'
}

export function storageSubtitle(storage: StorageCard) {
  if (storage.provider === 'local') {
    return '服务器本地 /uploads 目录'
  }
  return configText(storage, 'bucket_name') || '未配置 Bucket'
}

export function storageMetaItems(storage: StorageCard) {
  if (storage.provider === 'local') {
    return [
      { label: '目录', value: '/uploads' },
      { label: '接入方式', value: '服务器本地' },
      { label: '状态', value: storage.enabled ? '开启' : '关闭' },
    ]
  }

  if (storage.provider === 's3-compatible') {
    return [
      { label: '访问域名', value: configText(storage, 'access_domain') || '未设置' },
      { label: 'Endpoint', value: configText(storage, 'endpoint') || '未设置' },
      { label: '地域', value: configText(storage, 'region_id') || '未设置' },
    ]
  }

  return [{ label: '类型', value: storage.provider }]
}

export function storageFeatureChips(storage: StorageCard) {
  if (storage.provider === 'local') {
    return [`状态 ${storage.enabled ? '开启' : '关闭'}`, '模式 本地目录']
  }

  if (storage.provider === 's3-compatible') {
    return [
      `状态 ${storage.enabled ? '开启' : '关闭'}`,
      `Bucket ${configText(storage, 'bucket_name') || '未配置'}`,
      `Path Style ${configBool(storage, 'force_path_style') ? '开启' : '关闭'}`,
      `密钥 ${storage.secretConfigured ? '已配置' : '未配置'}`,
    ]
  }

  return [`状态 ${storage.enabled ? '开启' : '关闭'}`]
}

export function buildPayload(
  profile: StorageCard,
  providerMap: ProviderMetaMap,
): StorageProfilePayload {
  const fields = providerMap[profile.provider]?.fields || []
  const config: Record<string, unknown> = {}

  for (const field of fields) {
    const value = profile.config[field.key]
    config[field.key] =
      field.type === 'switch'
        ? Boolean(value)
        : typeof value === 'string'
          ? value.trim()
          : value == null
            ? ''
            : String(value)
  }

  return {
    name: profile.name.trim() || `${providerLabel(profile.provider, providerMap)} 存储`,
    provider: profile.provider,
    enabled: profile.enabled,
    config,
  }
}
