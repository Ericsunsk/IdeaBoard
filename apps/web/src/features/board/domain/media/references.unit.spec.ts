import { describe, expect, it } from 'vitest'
import { makeBoardSnapshot } from '../../testing'
import { rebuildBoardMediaRefs } from './references'

describe('rebuildBoardMediaRefs', () => {
  it('drops a prompt input ref after the input is removed', () => {
    const snapshot = makeBoardSnapshot({
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

    rebuildBoardMediaRefs(snapshot)
    expect(snapshot.mediaRefs).toEqual([
      { mediaAssetId: 301, refType: 'board_node', refId: 'prompt-1', role: 'input:0' },
    ])

    snapshot.promptNodes[0]!.inputImages = []
    rebuildBoardMediaRefs(snapshot)

    expect(snapshot.mediaRefs).toBeUndefined()
  })

  it('does not retain an image source asset without a visible url', () => {
    const snapshot = makeBoardSnapshot({
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

    rebuildBoardMediaRefs(snapshot)

    expect(snapshot.mediaRefs).toBeUndefined()
  })
})
