import {
  ProviderAPIType,
  type ProviderAPIType as ProviderAPITypeValue,
} from '../../../../shared/api/generated-query/model'

export const DEFAULT_ADMIN_CHANNEL_API_TYPE = ProviderAPIType['openai-compatible']
export const GOOGLE_ADMIN_CHANNEL_API_TYPE = ProviderAPIType['google-ai']

export const ADMIN_CHANNEL_API_TYPE_OPTIONS: Array<{
  label: string
  value: ProviderAPITypeValue
}> = [
  { label: 'OpenAI Compatible', value: ProviderAPIType['openai-compatible'] },
  { label: 'Google', value: GOOGLE_ADMIN_CHANNEL_API_TYPE },
]

const adminChannelApiTypeLabelByValue = new Map(
  ADMIN_CHANNEL_API_TYPE_OPTIONS.map((item) => [item.value, item.label]),
)

export function getAdminChannelApiTypeLabel(value: string) {
  return adminChannelApiTypeLabelByValue.get(value as ProviderAPITypeValue) || value || '未配置'
}
