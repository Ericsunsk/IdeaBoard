import { describe, expect, it } from 'vitest'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../canvas-board/domain'
import { CanvasGraphStore, toCanvasGraphEdgeId, toCanvasGraphNodeId } from '../../graph-runtime'
import { materializeResolvedImageResults } from './resultMaterializer'

function createPromptNode(id: number): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x: 0,
    y: 0,
    width: 280,
    title: `Prompt ${id}`,
    text: `prompt ${id}`,
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  }
}

function createImageNode(id: number, overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x: id * 40,
    y: 120,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: `task-${id}`,
    outputStatus: 'running',
    outputImages: [],
    ...overrides,
  }
}

function createImageInputConnection(
  id: number,
  toNodeId: number,
  overrides: Partial<CanvasConnection> = {},
): CanvasConnection {
  return {
    id,
    fromNodeType: 'prompt',
    fromNodeId: 1,
    fromPort: 'image',
    toNodeType: 'image',
    toNodeId,
    toPort: 'image',
    ...overrides,
  }
}

function createSnapshot(): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.promptNodes = [createPromptNode(1)]
  snapshot.imageNodes = [createImageNode(1), createImageNode(2)]
  snapshot.connections = [createImageInputConnection(1, 1), createImageInputConnection(2, 2)]
  return snapshot
}

describe('result materializer', () => {
  it('materializes multiple pending image nodes as one Graph Runtime transaction', () => {
    const store = CanvasGraphStore.fromSnapshot(createSnapshot())

    const result = materializeResolvedImageResults({
      store,
      results: [
        {
          resultNodeId: toCanvasGraphNodeId('image', 1),
          taskId: 'task-1',
          url: 'https://example.com/result-1.png',
          ratio: 1,
          width: 400,
          height: 200,
          mediaAssetId: 42,
        },
        {
          resultNodeId: toCanvasGraphNodeId('image', 2),
          taskId: 'task-2',
          url: 'https://example.com/result-2.png',
          ratio: 0.75,
        },
      ],
    })

    expect(result.summary).toMatchObject({
      reason: 'result-materialize',
      didChange: true,
      hasDomainChanges: true,
      changedNodeIds: [toCanvasGraphNodeId('image', 1), toCanvasGraphNodeId('image', 2)],
      changedEdgeIds: [],
      shouldRecordHistory: false,
      shouldMarkDirty: true,
    })
    expect(result.materializedNodeIds).toEqual([
      toCanvasGraphNodeId('image', 1),
      toCanvasGraphNodeId('image', 2),
    ])
    expect(result.skipped).toEqual([])
    expect(result.projection.updatedEdges).toEqual([
      { id: toCanvasGraphEdgeId(1), animated: false },
      { id: toCanvasGraphEdgeId(2), animated: false },
    ])

    const snapshot = store.exportSnapshot()
    expect(snapshot.imageNodes[0]).toMatchObject({
      id: 1,
      sourceImageUrl: 'https://example.com/result-1.png',
      sourceMediaAssetId: 42,
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
      outputImages: [],
      width: 520,
    })
    expect(snapshot.imageNodes[0]?.pendingTaskId).toBeUndefined()
    expect(snapshot.imageNodes[1]).toMatchObject({
      id: 2,
      sourceImageUrl: 'https://example.com/result-2.png',
      sourceImageRatio: 0.75,
      outputStatus: 'ready',
      outputImages: [],
      width: 280,
    })
    expect(snapshot.imageNodes[1]?.pendingTaskId).toBeUndefined()
  })

  it('skips missing, stale, non-image, and detached targets while materializing valid results', () => {
    const snapshot = createSnapshot()
    snapshot.imageNodes.push(
      createImageNode(3, {
        pendingTaskId: 'current-task',
      }),
      createImageNode(4, {
        pendingTaskId: 'task-4',
      }),
    )
    snapshot.connections.push(createImageInputConnection(3, 3))
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const result = materializeResolvedImageResults({
      store,
      isAttached: (item) => item.resultNodeId !== toCanvasGraphNodeId('image', 4),
      results: [
        {
          resultNodeId: toCanvasGraphNodeId('image', 1),
          taskId: 'task-1',
          url: 'https://example.com/valid.png',
          ratio: 1,
        },
        {
          resultNodeId: toCanvasGraphNodeId('image', 99),
          taskId: 'task-99',
          url: 'https://example.com/missing.png',
          ratio: 1,
        },
        {
          resultNodeId: toCanvasGraphNodeId('image', 3),
          taskId: 'old-task',
          url: 'https://example.com/stale.png',
          ratio: 1,
        },
        {
          resultNodeId: toCanvasGraphNodeId('prompt', 1),
          taskId: 'task-prompt',
          url: 'https://example.com/non-image.png',
          ratio: 1,
        },
        {
          resultNodeId: toCanvasGraphNodeId('image', 4),
          taskId: 'task-4',
          url: 'https://example.com/detached.png',
          ratio: 1,
        },
      ],
    })

    expect(result.materializedNodeIds).toEqual([toCanvasGraphNodeId('image', 1)])
    expect(result.skipped).toEqual([
      { resultNodeId: toCanvasGraphNodeId('image', 99), reason: 'missing-node' },
      { resultNodeId: toCanvasGraphNodeId('image', 3), reason: 'stale-task' },
      { resultNodeId: toCanvasGraphNodeId('prompt', 1), reason: 'non-image-node' },
      { resultNodeId: toCanvasGraphNodeId('image', 4), reason: 'detached-node' },
    ])
    expect(result.summary).toMatchObject({
      reason: 'result-materialize',
      didChange: true,
      changedNodeIds: [toCanvasGraphNodeId('image', 1)],
      shouldRecordHistory: false,
      shouldMarkDirty: true,
    })
    expect(result.projection.updatedEdges).toEqual([
      { id: toCanvasGraphEdgeId(1), animated: false },
    ])

    const exported = store.exportSnapshot()
    expect(exported.imageNodes.find((node) => node.id === 1)).toMatchObject({
      sourceImageUrl: 'https://example.com/valid.png',
      outputStatus: 'ready',
    })
    expect(exported.imageNodes.find((node) => node.id === 3)).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'current-task',
    })
    expect(exported.imageNodes.find((node) => node.id === 4)).toMatchObject({
      sourceImageUrl: '',
      outputStatus: 'running',
      pendingTaskId: 'task-4',
    })
  })

  it('does not require an already projected edge to materialize a valid result', () => {
    const snapshot = createSnapshot()
    snapshot.imageNodes.push(createImageNode(4, { pendingTaskId: 'task-4' }))
    const store = CanvasGraphStore.fromSnapshot(snapshot)

    const result = materializeResolvedImageResults({
      store,
      results: [
        {
          resultNodeId: toCanvasGraphNodeId('image', 4),
          taskId: 'task-4',
          url: 'https://example.com/edge-later.png',
          ratio: 1,
        },
      ],
    })

    expect(result.materializedNodeIds).toEqual([toCanvasGraphNodeId('image', 4)])
    expect(result.skipped).toEqual([])
    expect(result.projection.updatedEdges).toEqual([])
    expect(store.exportSnapshot().imageNodes.find((node) => node.id === 4)).toMatchObject({
      sourceImageUrl: 'https://example.com/edge-later.png',
      outputStatus: 'ready',
    })
    expect(
      store.exportSnapshot().imageNodes.find((node) => node.id === 4)?.pendingTaskId,
    ).toBeUndefined()
  })
})
