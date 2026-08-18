export type StorageProvider = string
export type StorageFieldType = string

export interface StorageProviderField {
  key: string
  label: string
  type: StorageFieldType
  placeholder?: string
  hint?: string
  required?: boolean
  secret?: boolean
}

export interface StorageProviderMeta {
  provider: StorageProvider
  label: string
  description: string
  builtin: boolean
  create_allowed: boolean
  supports_test: boolean
  supports_bucket_info: boolean
  fields: StorageProviderField[]
}

export interface StorageProfile {
  id: string
  name: string
  provider: StorageProvider
  enabled: boolean
  is_current: boolean
  builtin: boolean
  secret_configured: boolean
  config: Record<string, unknown>
}

export interface StorageProfilePayload {
  name: string
  provider: StorageProvider
  enabled: boolean
  config: Record<string, unknown>
}

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
export type StorageStatusClass = 'is-current' | 'is-enabled' | 'is-empty'

export function toProviderMetaMap(providers: StorageProviderMeta[]): ProviderMetaMap {
  return Object.fromEntries(
    providers.map((provider) => [provider.provider, provider]),
  ) as ProviderMetaMap
}

export function providerLabel(provider: StorageProvider, providerMap: ProviderMetaMap) {
  return providerMap[provider]?.label || provider
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

export function statusClass(
  storage: StorageCard,
  providerMap: ProviderMetaMap,
): StorageStatusClass {
  if (storage.isCurrent) return 'is-current'
  if (hasConfiguredProfile(storage, providerMap)) return 'is-enabled'
  return 'is-empty'
}

export function storageStatusSeverity(status: StorageStatusClass) {
  return {
    'is-current': 'success',
    'is-enabled': 'info',
    'is-empty': 'warn',
  }[status]
}

export function storageSubtitle(storage: StorageCard) {
  if (storage.provider === 'local') return ''
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

export function buildPayload(
  profile: StorageCard,
  providerMap: ProviderMetaMap,
): StorageProfilePayload {
  const fields = providerMap[profile.provider]?.fields || []
  const config: Record<string, unknown> = {}

  for (const field of fields) {
    const value = profile.config[field.key]
    const serializedValue =
      field.type === 'switch'
        ? Boolean(value)
        : typeof value === 'string'
          ? value.trim()
          : value == null
            ? ''
            : String(value)

    if (field.secret && profile.secretConfigured && serializedValue === '') {
      continue
    }

    config[field.key] = serializedValue
  }

  return {
    name: profile.name.trim() || `${providerLabel(profile.provider, providerMap)} 存储`,
    provider: profile.provider,
    enabled: profile.enabled,
    config,
  }
}
