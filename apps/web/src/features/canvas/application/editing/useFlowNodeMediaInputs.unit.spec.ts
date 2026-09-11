import { describe, expect, it, vi } from 'vitest'
import {
  rebuildBoardMediaRefs,
  type CanvasNodeItem,
  type ImageNodeItem,
} from '../../../board'
import { makeBoardSnapshot } from '../testing/boardFixtures'
import { useFlowNodeMediaInputs } from './useFlowNodeMediaInputs'

describe('useFlowNodeMediaInputs', () => {
  it('releases the source media ref when resetting an image node', () => {
    const snapshot = makeBoardSnapshot({
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: '/uploads/canvas/user-uid/board-a/content/source.png',
          sourceMediaAssetId: 101,
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [],
        },
      ],
      mediaRefs: [
        {
          mediaAssetId: 101,
          refType: 'board_node',
          refId: 'image-1',
          role: 'source',
        },
      ],
    })
    const updateSnapshotNode = vi.fn(
      (_nodeId: string, patcher: (prev: CanvasNodeItem) => CanvasNodeItem) => {
        snapshot.imageNodes[0] = patcher(snapshot.imageNodes[0]!) as ImageNodeItem
      },
    )
    const markFlowExecutionNodeDirty = vi.fn()
    const inputs = useFlowNodeMediaInputs({
      pickImageFile: vi.fn(async () => null),
      readDroppedFile: vi.fn(() => null),
      applyImageFileToImageNode: vi.fn(async () => true),
      updateSnapshotNode,
      markFlowExecutionNodeDirty,
    })

    inputs.onFlowImageReset('image-1')
    rebuildBoardMediaRefs(snapshot)

    expect(snapshot.imageNodes[0]?.sourceImageUrl).toBe('')
    expect(snapshot.imageNodes[0]?.sourceMediaAssetId).toBeUndefined()
    expect(snapshot.mediaRefs).toBeUndefined()
    expect(markFlowExecutionNodeDirty).toHaveBeenCalledWith('image-1')
  })
})
