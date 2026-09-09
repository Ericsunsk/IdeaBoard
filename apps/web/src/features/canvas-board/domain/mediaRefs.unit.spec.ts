import { describe, expect, it } from 'vitest'
import { makeCanvasBoardSnapshot } from '../testing'
import { rebuildCanvasBoardMediaRefs } from './mediaRefs'

describe('rebuildCanvasBoardMediaRefs', () => {
  it('drops a prompt input ref after the input is removed', () => {
    const snapshot = makeCanvasBoardSnapshot({
      promptNodes: [
        {
          id: 1,
          type: 'prompt',
          x: 0,
          y: 0,
          width: 280,
          text: '',
          outputCount: 1,
          aspectRatio: 'auto',
          inputImages: [
            {
              id: 'external:prompt',
              sourceType: 'external',
              sourceId: 'prompt-input',
              url: '/uploads/canvas/user-uid/board-a/content/prompt-input.png',
              ratio: 1,
              mediaAssetId: 301,
            },
          ],
        },
      ],
    })

    rebuildCanvasBoardMediaRefs(snapshot)
    expect(snapshot.mediaRefs).toEqual([
      { mediaAssetId: 301, refType: 'board_node', refId: 'prompt-1', role: 'input:0' },
    ])

    snapshot.promptNodes[0]!.inputImages = []
    rebuildCanvasBoardMediaRefs(snapshot)

    expect(snapshot.mediaRefs).toBeUndefined()
  })

  it('does not retain an image source asset without a visible url', () => {
    const snapshot = makeCanvasBoardSnapshot({
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: '',
          sourceMediaAssetId: 101,
          sourceImageRatio: 1,
          outputStatus: 'idle',
          outputImages: [],
        },
      ],
    })

    rebuildCanvasBoardMediaRefs(snapshot)

    expect(snapshot.mediaRefs).toBeUndefined()
  })
})
