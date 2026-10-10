import { describe, expect, it } from 'vitest'
import { normalizeSupportChannels } from './useSupportChannels'

describe('support channel public input', () => {
  const channel = {
    id: 'help',
    name: 'Help',
    kind: 'custom',
    url: 'https://example.com/help',
    enabled: true,
    sort_order: 1,
  }
  it('discards disabled, unknown, credentialed and unsafe channels', () => {
    expect(
      normalizeSupportChannels([
        channel,
        { ...channel, enabled: false },
        { ...channel, kind: 'unknown' },
        { ...channel, url: 'javascript:alert(1)' },
        { ...channel, url: 'https://user:pass@example.com' },
        null,
      ]),
    ).toEqual([channel])
    expect(normalizeSupportChannels({ channels: [channel] })).toEqual([])
  })
})
