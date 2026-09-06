import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../canvas-board/domain'
import { CanvasGraphSession, stageCanvasGraphNodeDragIntent } from '../graph-runtime'
import { bindCanvasGraphSessionProjection } from './bindCanvasGraphSessionProjection'

describe('bindCanvasGraphSessionProjection', () => {
  it('projects a committed drag into the Vue Flow renderer', () => {
    const snapshot = createEmptyCanvasBoardSnapshot()
    snapshot.imageNodes = [
      {
        id: 1,
        type: 'image-node',
        x: 40,
        y: 20,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'idle',
        outputImages: [],
        outputCount: 1,
        outputResolution: '1k',
      },
    ]
    const session = CanvasGraphSession.hydrate('board-1', snapshot)
    const applyIncrementalProjection = vi.fn()
    const unsubscribe = bindCanvasGraphSessionProjection({
      session,
      graphAdapter: { applyIncrementalProjection, replaceFullProjection: vi.fn() },
      readCurrent: () => ({
        nodes: [{ id: 'image-1', position: { x: 40, y: 20 }, data: {} }],
        edges: [],
      }),
      projectFull: () => ({ nodes: [], edges: [] }),
      isCurrentAttachment: () => true,
    })

    const transaction = session.store.beginTransaction('node-drag')
    stageCanvasGraphNodeDragIntent(transaction, {
      nodeId: 'image-1',
      position: { x: 120, y: 80 },
    })
    transaction.commit()

    expect(applyIncrementalProjection).toHaveBeenCalledWith(
      expect.objectContaining({ changedNodeIds: ['image-1'] }),
    )
    unsubscribe()
  })

  it('contains renderer failures and schedules a full projection from canonical facts', () => {
    const snapshot = createEmptyCanvasBoardSnapshot()
    snapshot.imageNodes = [
      {
        id: 1,
        type: 'image-node',
        x: 40,
        y: 20,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'idle',
        outputImages: [],
        outputCount: 1,
        outputResolution: '1k',
      },
    ]
    const session = CanvasGraphSession.hydrate('board-1', snapshot)
    const applyIncrementalProjection = vi.fn().mockImplementationOnce(() => {
      throw new Error('renderer unavailable')
    })
    const replaceFullProjection = vi.fn()
    const scheduled: Array<() => void> = []
    const unsubscribe = bindCanvasGraphSessionProjection({
      session,
      graphAdapter: { applyIncrementalProjection, replaceFullProjection },
      readCurrent: () => ({
        nodes: [{ id: 'image-1', position: { x: 40, y: 20 }, data: {} }],
        edges: [],
      }),
      projectFull: () => ({
        nodes: [{ id: 'image-1', position: { x: 40, y: 20 }, data: {} }],
        edges: [],
      }),
      isCurrentAttachment: () => true,
      scheduleMicrotask: (task) => scheduled.push(task),
    })

    const tx = session.store.beginTransaction('node-update')
    tx.updateNode('image-1', { sourceImageUrl: 'https://example.com/image.png' })
    expect(() => tx.commit()).not.toThrow()
    expect(session.store.getNode('image-1')?.snapshot).toMatchObject({
      sourceImageUrl: 'https://example.com/image.png',
    })
    expect(scheduled).toHaveLength(1)

    scheduled[0]()
    expect(replaceFullProjection).toHaveBeenCalledWith(
      expect.objectContaining({ nodes: expect.any(Array) }),
    )
    unsubscribe()
  })
})
