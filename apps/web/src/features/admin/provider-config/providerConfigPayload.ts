import { ProviderAPIType } from '../../../shared/api/generated-query/model'
import type {
  AdminAccountCreateRequest,
  AdminAccountModelInput,
  AdminAccountUpdateRequest,
} from '../../../shared/api/generated-query/model'
import type {
  ProviderAccountModelRecord,
  ProviderAccountForm,
  ProviderAccountRecord,
  ProviderApiTypeValue,
  ProviderChannelFormData,
  ProviderChannelRecord,
} from './providerConfigTypes'

export const DEFAULT_ADMIN_CHANNEL_API_TYPE = ProviderAPIType['openai-compatible']
export const GOOGLE_ADMIN_CHANNEL_API_TYPE = ProviderAPIType['google-ai']
export const DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG = '{}'
export const DEFAULT_ADMIN_ACCOUNT_WEIGHT = 100
export const DEFAULT_ADMIN_ACCOUNT_MODEL_WEIGHT = 1

export const ADMIN_CHANNEL_API_TYPE_OPTIONS: Array<{
  label: string
  value: ProviderApiTypeValue
}> = [
  { label: 'OpenAI Compatible', value: DEFAULT_ADMIN_CHANNEL_API_TYPE },
  { label: 'Google', value: GOOGLE_ADMIN_CHANNEL_API_TYPE },
]

const adminChannelApiTypeLabelByValue = new Map(
  ADMIN_CHANNEL_API_TYPE_OPTIONS.map((item) => [item.value, item.label]),
)

export function getAdminChannelApiTypeLabel(value: string) {
  return adminChannelApiTypeLabelByValue.get(value as ProviderApiTypeValue) || value || '未配置'
}

export function createEmptyChannelForm(): ProviderChannelFormData {
  return {
    apiType: DEFAULT_ADMIN_CHANNEL_API_TYPE,
    name: '',
    baseUrl: '',
  }
}

export function createChannelSavePayload(form: ProviderChannelFormData) {
  return {
    type: form.apiType,
    name: form.name,
    api_url: form.baseUrl,
  }
}

export function inferChannelApiTypeFromUrl(
  apiUrl: string,
  fallback: ProviderApiTypeValue = DEFAULT_ADMIN_CHANNEL_API_TYPE,
): ProviderApiTypeValue {
  const normalized = apiUrl.trim().toLowerCase()
  if (normalized.includes('generativelanguage.googleapis.com')) {
    return GOOGLE_ADMIN_CHANNEL_API_TYPE
  }
  if (normalized.includes('api.openai.com')) {
    return DEFAULT_ADMIN_CHANNEL_API_TYPE
  }
  return fallback
}

export function createAccountCreatePayload(input: {
  form: ProviderAccountForm
  channel: ProviderChannelRecord | null
  modelValues: string[]
  secret: string
}): AdminAccountCreateRequest {
  const channel = input.channel
  const apiType = channel?.apiType || DEFAULT_ADMIN_CHANNEL_API_TYPE

  return {
    models: accountModelInputsFromValues(input.modelValues),
    max_concurrent: input.form.maxConcurrent,
    timeout: input.form.timeoutSeconds,
    name: channel?.name || 'Unknown',
    platform: apiType,
    api_type: apiType,
    proxy_url: channel?.baseUrl || '',
    channel_id: input.form.channelId,
    secret: input.secret,
    extra_config: DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
  }
}

export function createAccountUpdatePayload(input: {
  form: ProviderAccountForm
  channel: ProviderChannelRecord | null
  existingAccount?: ProviderAccountRecord
  modelValues: string[]
  secret?: string
}): AdminAccountUpdateRequest {
  const channel = input.channel
  const existingAccount = input.existingAccount
  const apiType = channel?.apiType || existingAccount?.apiType || DEFAULT_ADMIN_CHANNEL_API_TYPE

  return {
    models: accountModelInputsFromValues(input.modelValues, existingAccount?.models),
    max_concurrent: input.form.maxConcurrent,
    timeout: input.form.timeoutSeconds,
    name: channel?.name || existingAccount?.name || 'Unknown',
    platform: channel?.apiType || existingAccount?.platform || DEFAULT_ADMIN_CHANNEL_API_TYPE,
    api_type: apiType,
    proxy_url: channel?.baseUrl || existingAccount?.baseUrl || '',
    weight: existingAccount?.weight ?? DEFAULT_ADMIN_ACCOUNT_WEIGHT,
    extra_config: existingAccount?.extraConfig || DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
    ...(input.secret ? { secret: input.secret } : {}),
  }
}

export function createAccountUpdatePayloadFromAccount(
  account: ProviderAccountRecord,
): AdminAccountUpdateRequest {
  return {
    name: account.name,
    platform: account.platform,
    api_type: account.apiType,
    models: accountModelInputsFromModels(account.models),
    proxy_url: account.baseUrl,
    max_concurrent: account.maxConcurrent,
    timeout: account.timeoutSeconds,
    weight: account.weight,
    extra_config: account.extraConfig || DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
  }
}

export function accountModelInputsFromValues(
  values: string[],
  existingModels: ProviderAccountModelRecord[] = [],
): AdminAccountModelInput[] {
  const existingByKey = new Map(
    existingModels.map((model) => [normalizeModelKey(model.name), model]),
  )
  return uniqueTrimmedModelValues(values).map((name) => {
    const existing = existingByKey.get(normalizeModelKey(name))
    return {
      name,
      enabled: existing?.enabled ?? true,
      weight: existing?.weight ?? DEFAULT_ADMIN_ACCOUNT_MODEL_WEIGHT,
      max_concurrent: existing?.maxConcurrent,
      feature_slugs: existing?.featureSlugs,
    }
  })
}

export function accountModelInputsFromModels(
  models: ProviderAccountModelRecord[],
): AdminAccountModelInput[] {
  return models.map((model) => ({
    name: model.name,
    enabled: model.enabled,
    weight: model.weight,
    max_concurrent: model.maxConcurrent,
    feature_slugs: model.featureSlugs,
  }))
}

export function uniqueTrimmedModelValues(values: string[]) {
  const seen = new Set<string>()
  const result: string[] = []
  values.forEach((value) => {
    const trimmed = value.trim()
    if (!trimmed) return
    const key = normalizeModelKey(trimmed)
    if (seen.has(key)) return
    seen.add(key)
    result.push(trimmed)
  })
  return result
}

export function normalizeModelKey(model: string) {
  return model.trim().toLowerCase()
}
