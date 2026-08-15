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
  ProviderEndpointFormData,
  ProviderEndpointRecord,
} from './providerConfigTypes'

export const DEFAULT_ADMIN_PROVIDER_ENDPOINT_API_TYPE = ProviderAPIType['openai-compatible']
export const GOOGLE_ADMIN_PROVIDER_ENDPOINT_API_TYPE = ProviderAPIType['google-ai']
export const GOOGLE_CLOUD_ADMIN_PROVIDER_ENDPOINT_API_TYPE = ProviderAPIType['google-cloud']
export const DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG = '{}'
export const DEFAULT_ADMIN_ACCOUNT_WEIGHT = 100
export const DEFAULT_ADMIN_ACCOUNT_MODEL_WEIGHT = 1

export const ADMIN_PROVIDER_ENDPOINT_API_TYPE_OPTIONS: Array<{
  label: string
  value: ProviderApiTypeValue
}> = [
  { label: 'OpenAI Compatible', value: DEFAULT_ADMIN_PROVIDER_ENDPOINT_API_TYPE },
  { label: 'Google AI Studio', value: GOOGLE_ADMIN_PROVIDER_ENDPOINT_API_TYPE },
  {
    label: 'Gemini Enterprise Agent Platform',
    value: GOOGLE_CLOUD_ADMIN_PROVIDER_ENDPOINT_API_TYPE,
  },
]

const adminProviderEndpointApiTypeLabelByValue = new Map(
  ADMIN_PROVIDER_ENDPOINT_API_TYPE_OPTIONS.map((item) => [item.value, item.label]),
)

export function getAdminProviderEndpointApiTypeLabel(value: string) {
  return (
    adminProviderEndpointApiTypeLabelByValue.get(value as ProviderApiTypeValue) || value || '未配置'
  )
}

export function createEmptyProviderEndpointForm(): ProviderEndpointFormData {
  return {
    apiType: DEFAULT_ADMIN_PROVIDER_ENDPOINT_API_TYPE,
    name: '',
    baseUrl: '',
    status: 1,
  }
}

export function createProviderEndpointSavePayload(form: ProviderEndpointFormData) {
  return {
    api_type: form.apiType,
    name: form.name,
    base_url: form.baseUrl,
    status: form.status,
  }
}

export function inferProviderEndpointApiTypeFromUrl(
  apiUrl: string,
  fallback: ProviderApiTypeValue = DEFAULT_ADMIN_PROVIDER_ENDPOINT_API_TYPE,
): ProviderApiTypeValue {
  const normalized = apiUrl.trim().toLowerCase()
  if (normalized.includes('generativelanguage.googleapis.com')) {
    return GOOGLE_ADMIN_PROVIDER_ENDPOINT_API_TYPE
  }
  if (normalized.includes('aiplatform.googleapis.com')) {
    return GOOGLE_CLOUD_ADMIN_PROVIDER_ENDPOINT_API_TYPE
  }
  if (normalized.includes('api.openai.com')) {
    return DEFAULT_ADMIN_PROVIDER_ENDPOINT_API_TYPE
  }
  return fallback
}

export function createAccountCreatePayload(input: {
  form: ProviderAccountForm
  providerEndpoint: ProviderEndpointRecord | null
  modelValues: string[]
  modelCapabilities?: ProviderAccountForm['modelCapabilities']
  secret: string
}): AdminAccountCreateRequest {
  const providerEndpoint = input.providerEndpoint
  const name = input.form.name.trim() || providerEndpoint?.name || 'Unknown'

  return {
    models: accountModelInputsFromValues(input.modelValues, [], input.modelCapabilities),
    max_concurrent: input.form.maxConcurrent,
    timeout: input.form.timeoutSeconds,
    name,
    provider_endpoint_id: input.form.providerEndpointId,
    secret: input.secret,
    extra_config: DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
  }
}

export function createAccountUpdatePayload(input: {
  form: ProviderAccountForm
  providerEndpoint: ProviderEndpointRecord | null
  existingAccount?: ProviderAccountRecord
  modelValues: string[]
  secret?: string
  modelCapabilities?: ProviderAccountForm['modelCapabilities']
}): AdminAccountUpdateRequest {
  const providerEndpoint = input.providerEndpoint
  const existingAccount = input.existingAccount
  const name =
    input.form.name.trim() || existingAccount?.name || providerEndpoint?.name || 'Unknown'

  return {
    models: accountModelInputsFromValues(
      input.modelValues,
      existingAccount?.models,
      input.modelCapabilities,
    ),
    max_concurrent: input.form.maxConcurrent,
    timeout: input.form.timeoutSeconds,
    name,
    weight: existingAccount?.weight ?? DEFAULT_ADMIN_ACCOUNT_WEIGHT,
    extra_config: existingAccount?.extraConfig || DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
    ...(input.secret ? { secret: input.secret } : {}),
  }
}

export function accountModelInputsFromValues(
  values: string[],
  existingModels: ProviderAccountModelRecord[] = [],
  capabilities: ProviderAccountForm['modelCapabilities'] = {},
): AdminAccountModelInput[] {
  const existingByKey = new Map(
    existingModels.map((model) => [normalizeModelKey(model.name), model]),
  )
  return uniqueTrimmedModelValues(values).map((name) => {
    const existing = existingByKey.get(normalizeModelKey(name))
    const configured = capabilities[normalizeModelKey(name)]
    return {
      name,
      enabled: existing?.enabled ?? true,
      weight: existing?.weight ?? DEFAULT_ADMIN_ACCOUNT_MODEL_WEIGHT,
      max_concurrent: existing?.maxConcurrent,
      supports_2k: configured?.supports2K ?? existing?.supports2K ?? false,
      supports_4k: configured?.supports4K ?? existing?.supports4K ?? false,
    }
  })
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
