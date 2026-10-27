import { describe, expect, it } from 'vitest'
import { normalizeTaskImageResults } from './taskResult'

describe('Task asset result normalization', () => {
  it('normalizes explicit asset identity and metadata without Workspace selection', () => {
    expect(
      normalizeTaskImageResults([
        {
          workspaceId: ' ws_task123 ',
          assetId: ' asset_42 ',
          width: '1024',
          height: 768,
          mimeType: ' image/png ',
          resultIndex: 2,
        },
      ]),
    ).toEqual([
      {
        workspaceId: 'ws_task123',
        assetId: 'asset_42',
        width: 1024,
        height: 768,
        mimeType: 'image/png',
        resultIndex: 2,
      },
    ])
  })

  it('drops results without a scoped asset identity, including remote refs', () => {
    expect(
      normalizeTaskImageResults([
        { assetId: 'asset_42' },
        { workspaceId: 'ws_task123' },
        { workspaceId: ' ', assetId: 'asset_42' },
        { url: 'https://example.com/provider.png' },
      ]),
    ).toEqual([])
  })
})
