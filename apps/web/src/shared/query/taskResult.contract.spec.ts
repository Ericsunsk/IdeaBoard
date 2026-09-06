import { describe, expect, it } from 'vitest'
import { extractImageResultsFromTaskPayload, normalizeTaskImageResults } from './taskResult'

describe('taskResult image result helpers', () => {
  it('extracts normalized image output metadata from materialized outputs', () => {
    expect(
      extractImageResultsFromTaskPayload({
        outputs: [
          {
            role: 'image',
            width: '1024',
            height: 768,
            mime_type: ' image/png ',
            asset_id: 'asset_42',
          },
        ],
      }),
    ).toEqual([
      {
        url: '/api/user/media-assets/asset_42/derivatives/preview/webp/content',
        assetId: 'asset_42',
        width: 1024,
        height: 768,
        mimeType: 'image/png',
      },
    ])
  })

  it('drops result objects that do not carry a public asset identity', () => {
    expect(normalizeTaskImageResults([{ width: 0, height: 100 }, {}])).toEqual([])
  })
})
