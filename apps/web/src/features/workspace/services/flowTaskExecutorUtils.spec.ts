import { describe, expect, it } from 'vitest'
import type { ImageNodeItem } from '../../canvas-board/domain'
import { resolveFlowSourceImageRatio, resolveSourceImageFromNode } from './flowTaskExecutorUtils'

function makeImageNode(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/source.png',
    sourceImageRatio: 0.75,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

describe('flowTaskExecutorUtils', () => {
  it('uses the image node display output for connected image source data', () => {
    const snapshot = makeImageNode({
      outputImages: [
        { url: 'https://example.com/old.png', ratio: 1 },
        { url: 'https://example.com/latest.png', ratio: 1.5 },
      ],
    })

    expect(resolveSourceImageFromNode('image', snapshot)).toBe('https://example.com/latest.png')
    expect(resolveFlowSourceImageRatio('image', snapshot)).toBe(1.5)
  })

  it('falls back to source image data when an image node has no outputs', () => {
    const snapshot = makeImageNode({
      sourceImageUrl: ' https://example.com/source-only.png ',
      sourceImageRatio: 0.5,
      outputImages: [],
    })

    expect(resolveSourceImageFromNode('image', snapshot)).toBe('https://example.com/source-only.png')
    expect(resolveFlowSourceImageRatio('image', snapshot)).toBe(0.5)
  })

  it('does not treat source image fields as execution output fields for non-image nodes', () => {
    const snapshot = {
      type: 'fusion',
      sourceImageUrl: 'https://example.com/source.png',
      sourceImageRatio: 0.5,
      outputImages: [{ url: 'https://example.com/fusion.png', ratio: 1.25 }],
    }

    expect(resolveSourceImageFromNode('fusion', snapshot)).toBe('https://example.com/fusion.png')
    expect(resolveFlowSourceImageRatio('fusion', snapshot)).toBe(1.25)
  })
})
