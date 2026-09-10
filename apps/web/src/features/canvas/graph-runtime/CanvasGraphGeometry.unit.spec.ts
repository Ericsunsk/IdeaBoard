import { describe, expect, it } from 'vitest'
import { createEmptyBoardSnapshot } from '../../board/domain'
import {
  readCanvasGraphNodeBounds,
  resolveCanvasGraphNodeDimensions,
  resolveCanvasGraphNodesBounds,
} from './canvasGraphGeometry'
import { createCanvasGraphStore } from './canvasGraphStore'

describe('CanvasGraphGeometry', () => {
  it('uses the canonical image ratio and header policy', () => {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.imageNodes = [
      {
        id: 1,
        type: 'image-node',
        x: 40,
        y: 60,
        width: 280,
        sourceImageUrl: 'https://example.com/source.png',
        sourceImageRatio: 2,
        outputStatus: 'ready',
        outputImages: [],
      },
    ]
    const store = createCanvasGraphStore(snapshot)

    const imageNode = snapshot.imageNodes[0]
    if (!imageNode) throw new Error('expected image node fixture')
    expect(resolveCanvasGraphNodeDimensions('image', imageNode)).toEqual({
      width: 280,
      height: 582,
    })
    expect(readCanvasGraphNodeBounds(store, 'image-1')).toEqual({
      left: 40,
      top: 60,
      right: 320,
      bottom: 642,
      width: 280,
      height: 582,
    })
  })

  it('merges canonical node bounds with explicit group padding', () => {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.promptNodes = [
      {
        id: 1,
        type: 'prompt',
        x: 40,
        y: 40,
        width: 280,
        text: '',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
      {
        id: 2,
        type: 'prompt',
        x: 400,
        y: 80,
        width: 280,
        text: '',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ]
    const store = createCanvasGraphStore(snapshot)

    expect(resolveCanvasGraphNodesBounds(store, ['prompt-1', 'prompt-2'], 10)).toEqual({
      left: 30,
      top: 30,
      right: 690,
      bottom: 392,
      width: 660,
      height: 362,
    })
  })
})
