import { useQuery } from '@tanstack/vue-query'
import { computed, unref, type MaybeRef } from 'vue'
import {
  SUPPORT_CHANNELS_QUERY_KEY,
  fetchSupportChannels,
  type SupportChannel,
} from '@/api/supportChannels'
import { noRetry, STALE_TIME_MS } from '../query/queryDefaults'

export type { SupportChannel }
export { SUPPORT_CHANNELS_QUERY_KEY }

const SUPPORT_CHANNEL_KINDS = new Set([
  'discord',
  'telegram',
  'whatsapp',
  'line',
  'wechat',
  'custom',
])

function isSafeSupportUrl(value: unknown): value is string {
  if (typeof value !== 'string' || value.length > 2048 || /[\s\\]/.test(value)) return false
  try {
    const url = new URL(value)
    return (url.protocol === 'https:' || url.protocol === 'http:') && !url.username && !url.password
  } catch {
    return false
  }
}

export function normalizeSupportChannels(value: unknown): SupportChannel[] {
  if (!Array.isArray(value)) return []
  return value
    .filter((channel): channel is SupportChannel => {
      if (!channel || typeof channel !== 'object') return false
      const row = channel as Record<string, unknown>
      if (
        typeof row.id !== 'string' ||
        !row.id ||
        typeof row.name !== 'string' ||
        !row.name.trim() ||
        row.enabled !== true ||
        typeof row.kind !== 'string' ||
        !SUPPORT_CHANNEL_KINDS.has(row.kind) ||
        !isSafeSupportUrl(row.url) ||
        typeof row.sort_order !== 'number' ||
        !Number.isInteger(row.sort_order)
      )
        return false
      return true
    })
    .sort((a, b) => a.sort_order - b.sort_order)
}

export function useSupportChannels(enabled: MaybeRef<boolean> = true) {
  const query = useQuery({
    queryKey: SUPPORT_CHANNELS_QUERY_KEY,
    queryFn: ({ signal }) => fetchSupportChannels(signal),
    enabled: computed(() => unref(enabled)),
    staleTime: STALE_TIME_MS.statistics,
    ...noRetry(),
  })
  const channels = computed<SupportChannel[]>(() => {
    if (query.isPending.value || query.isError.value) return []
    return normalizeSupportChannels(query.data.value)
  })
  return { channels }
}
