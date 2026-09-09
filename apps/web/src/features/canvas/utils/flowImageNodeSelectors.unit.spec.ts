import { describe, expect, it } from 'vitest'
import { resolveImageNodeLatestOutput, type ImageNodeItem } from '../../canvas-board/domain'
import {
  DEFAULT_IMAGE_NODE_LAYOUT_WIDTH,
  resolveImageNodeDisplayPayload,
  resolveImageNodeLayoutAnchor,
} from './flowImageNodeSelectors'

function makeImageNode(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 12,
    y: 24,
    width: 320,
    title: 'Image',
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 0.75,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

describe('flowImageNodeSelectors', () => {
  it('prefers the latest output image for display url and ratio', () => {
    const imageNode = makeImageNode({
      sourceMediaAssetId: 100,
      outputImages: [
        { url: 'https://example.com/old.png', ratio: 1 },
        {
          url: 'https://example.com/latest.png',
          ratio: 1.4,
          mediaAssetId: 200,
          mediaRef: { assetId: 'asset_latest', derivativeTier: 'preview' },
        },
      ],
    })

    expect(resolveImageNodeLatestOutput(imageNode)).toEqual({
      url: 'https://example.com/latest.png',
      ratio: 1.4,
      mediaAssetId: 200,
      mediaRef: { assetId: 'asset_latest', derivativeTier: 'preview' },
    })
    expect(resolveImageNodeDisplayPayload(imageNode)).toEqual({
      url: '/api/user/media-assets/asset_latest/derivatives/preview/content',
      ratio: 1.4,
      mediaRef: { assetId: 'asset_latest', derivativeTier: 'preview' },
      assetId: 'asset_latest',
    })
  })

  it('falls back to source image data when no output image exists', () => {
    const imageNode = makeImageNode({
      sourceImageUrl: ' https://example.com/source-only.png ',
      sourceMediaRef: { assetId: 'asset_source', derivativeTier: 'preview' },
      sourceMediaAssetId: 100,
      sourceImageRatio: 0.5,
      outputImages: [],
    })

    expect(resolveImageNodeDisplayPayload(imageNode)).toEqual({
      url: '/api/user/media-assets/asset_source/derivatives/preview/content',
      ratio: 0.5,
      mediaRef: { assetId: 'asset_source', derivativeTier: 'preview' },
      assetId: 'asset_source',
    })
  })

  it('omits delivery identity when no media ref exists', () => {
    const imageNode = makeImageNode({
      sourceImageUrl: 'https://example.com/source-only.png',
      sourceImageRatio: 0.5,
      outputImages: [],
    })

    expect(resolveImageNodeDisplayPayload(imageNode)).toEqual({
      url: 'https://example.com/source-only.png',
      ratio: 0.5,
    })
  })

  it('returns null for required empty layout sources and keeps a fallback layout otherwise', () => {
    const imageNode = makeImageNode({
      sourceImageUrl: '',
      sourceImageRatio: 0,
      outputImages: [],
      width: 0,
    })
    const node = { position: { x: 40, y: 60 } }

    expect(resolveImageNodeLayoutAnchor(node, imageNode, { requireUrl: true })).toBeNull()
    expect(resolveImageNodeLayoutAnchor(node, imageNode)).toEqual({
      url: '',
      ratio: 1,
      x: 40,
      y: 60,
      width: DEFAULT_IMAGE_NODE_LAYOUT_WIDTH,
    })
  })
})
