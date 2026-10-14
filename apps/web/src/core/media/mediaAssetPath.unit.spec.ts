import { describe, expect, it } from 'vitest'
import { isMediaAssetDeliveryPath } from './mediaAssetPath'

const assetPath = '/api/v1/workspaces/ws_test123/media-assets/asset_abc123'

describe('media asset delivery path grammar', () => {
  it.each(['content', 'derivatives/preview', 'derivatives/large-preview'])(
    'matches the byte-delivery tail %s',
    (tail) => {
      expect(isMediaAssetDeliveryPath(`${assetPath}/${tail}`)).toBe(true)
    },
  )

  it.each([
    '',
    '/api/v1/workspaces/ws_test123/media-assets',
    '/api/v1/workspaces/ws_test123/uploads/a.png',
    assetPath,
    `${assetPath}/metadata`,
    `${assetPath}/unknown`,
    `${assetPath}/derivatives/`,
    `${assetPath}/derivatives/ `,
    `${assetPath}/derivatives/preview/extra`,
    `${assetPath}/content/extra`,
    `${assetPath}/content?download=1`,
    `${assetPath}/content#preview`,
    '/other/media-assets/asset_abc123/content',
    '/api/v1/workspaces//media-assets/asset_abc123/content',
    `${assetPath}//content`,
  ])('rejects metadata, malformed, and unknown routes: %s', (pathname) => {
    expect(isMediaAssetDeliveryPath(pathname)).toBe(false)
  })
})
