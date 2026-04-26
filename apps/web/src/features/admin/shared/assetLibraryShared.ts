import { extractSafeApiErrorMessage as extractApiErrorMessage } from '../../../shared/utils/errorMessages'
export { extractApiErrorMessage }

export const ADMIN_STATUS_OPTIONS = [
  { label: '已上架', value: 1 },
  { label: '已下架', value: 0 },
]

type ListResponse<T> = { code?: number; data?: { list?: T[] } | null }

export function getApiList<T>(response: ListResponse<T> | null | undefined): T[] {
  return response?.data?.list || []
}

export function splitCommaValues(input: string | null | undefined): string[] {
  if (!input) return []
  return input.split(',').map(item => item.trim()).filter(Boolean)
}

export function getChannelModelOptions(
  channels: Array<{ id: number; models?: string | null }>,
  channelId: number | null | undefined,
) {
  if (!channelId) return []
  const channel = channels.find(item => item.id === channelId)
  return splitCommaValues(channel?.models).map(model => ({ label: model, value: model }))
}
