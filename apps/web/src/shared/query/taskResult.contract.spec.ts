import { describe, expect, it } from 'vitest'
import {
  extractImageResultsFromTaskPayload,
  normalizeTaskImageResults,
} from './taskResult'

describe('taskResult image result helpers', () => {
  it('extracts normalized image output metadata before falling back to result json', () => {
    expect(
      extractImageResultsFromTaskPayload({
        outputs: [
          {
            role: 'image',
            url: ' https://cdn.example.com/output.png ',
            width: '1024',
            height: 768,
            mime_type: ' image/png ',
            media_asset_id: '42',
          },
        ],
        result: JSON.stringify({ image_url: 'https://cdn.example.com/fallback.png' }),
      }),
    ).toEqual([
      {
        url: 'https://cdn.example.com/output.png',
        width: 1024,
        height: 768,
        mimeType: 'image/png',
        mediaAssetId: 42,
      },
    ])
  })

  it('normalizes result objects and drops invalid entries', () => {
    expect(
      normalizeTaskImageResults([
        { url: ' https://cdn.example.com/a.png ', width: 0, height: 100 },
        { url: '   ' },
      ]),
    ).toEqual([
      {
        url: 'https://cdn.example.com/a.png',
        height: 100,
      },
    ])
  })
})
