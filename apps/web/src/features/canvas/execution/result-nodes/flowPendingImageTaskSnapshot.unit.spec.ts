import type { Node } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import {
  collectPendingImageTasks,
  hasRenderableImageResult,
  isPendingImageTaskSnapshot,
  readPendingImageSnapshot,
} from './flowPendingImageTaskSnapshot'

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputStatus: 'running',
    outputImages: [],
    pendingTaskId: 'task-1',
    ...overrides,
  }
}

function makeImageNode(id: string, snapshot: ImageNodeItem): Node {
  return {
    id,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Image',
      snapshotNode: snapshot,
    },
  }
}

describe('flowPendingImageTaskSnapshot', () => {
  it('detects renderable image results from source or outputs', () => {
    expect(hasRenderableImageResult(makeImageSnapshot({ sourceImageUrl: ' out.png ' }))).toBe(true)
    expect(
      hasRenderableImageResult(
        makeImageSnapshot({ outputImages: [{ url: ' out.png', ratio: 1 }] }),
      ),
    ).toBe(true)
    expect(hasRenderableImageResult(makeImageSnapshot())).toBe(false)
  })

  it('only treats running image snapshots without renderable results as pending tasks', () => {
    expect(isPendingImageTaskSnapshot(makeImageSnapshot(), 'task-1')).toBe(true)
    expect(isPendingImageTaskSnapshot(makeImageSnapshot({ outputStatus: 'ready' }), 'task-1')).toBe(
      false,
    )
    expect(
      isPendingImageTaskSnapshot(makeImageSnapshot({ sourceImageUrl: 'out.png' }), 'task-1'),
    ).toBe(false)
    expect(isPendingImageTaskSnapshot(makeImageSnapshot(), 'other-task')).toBe(false)
  })

  it('collects pending tasks with normalized fallback ratio', () => {
    const nodes = [
      makeImageNode('image-1', makeImageSnapshot({ sourceImageRatio: 1.5 })),
      makeImageNode('image-2', makeImageSnapshot({ pendingTaskId: 'task-2', sourceImageRatio: 0 })),
      makeImageNode('image-3', makeImageSnapshot({ pendingTaskId: '', sourceImageRatio: 2 })),
    ]

    expect(collectPendingImageTasks(nodes)).toEqual([
      { nodeId: 'image-1', taskId: 'task-1', fallbackRatio: 1.5 },
      { nodeId: 'image-2', taskId: 'task-2', fallbackRatio: 1 },
    ])
  })

  it('reads image snapshots by node id only for image nodes', () => {
    const snapshot = makeImageSnapshot()
    const nodes = [
      makeImageNode('image-1', snapshot),
      { id: 'prompt-1', type: 'canvas', position: { x: 0, y: 0 }, data: { kind: 'prompt' } },
    ] satisfies Node[]

    expect(readPendingImageSnapshot(nodes, 'image-1')).toBe(snapshot)
    expect(readPendingImageSnapshot(nodes, 'prompt-1')).toBeNull()
  })
})
