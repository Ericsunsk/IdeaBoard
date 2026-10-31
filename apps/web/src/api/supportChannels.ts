// Support channels transport (API boundary: core must not import generated DTOs).
//
// Thin `api/*` wrapper owning the public support-channels transport plus the
// transport-DTO -> domain translation. `core/composables/useSupportChannels`
// keeps Vue wiring and the pure normalize helpers only.
import {
  getGetSupportChannelsQueryKey,
  getSupportChannels,
} from './generated-query/public/user/user'
import type { SupportChannel as GeneratedSupportChannel } from './generated-query/public/model/supportChannel'

/** @externalContract Public support-channel response fields are preserved across the API adapter. */
export type SupportChannel = {
  id: string
  name: string
  kind: string
  url: string
  enabled: boolean
  sort_order: number
}

export const SUPPORT_CHANNELS_QUERY_KEY = getGetSupportChannelsQueryKey()

function toSupportChannel(dto: GeneratedSupportChannel): SupportChannel {
  return {
    id: dto.id,
    name: dto.name,
    kind: dto.kind,
    url: dto.url,
    enabled: dto.enabled,
    sort_order: dto.sort_order,
  }
}

function fetchSupportChannelsEnvelope(signal?: AbortSignal) {
  return getSupportChannels({
    ...(signal == null ? {} : { signal }),
    authFailure: 'silent',
  })
}

export async function fetchSupportChannels(signal?: AbortSignal): Promise<SupportChannel[]> {
  const envelope = await fetchSupportChannelsEnvelope(signal)
  const channels = (envelope as { data?: { channels?: unknown } })?.data?.channels
  if (!Array.isArray(channels)) return []
  return channels
    .filter(
      (channel): channel is GeneratedSupportChannel => !!channel && typeof channel === 'object',
    )
    .map(toSupportChannel)
}
