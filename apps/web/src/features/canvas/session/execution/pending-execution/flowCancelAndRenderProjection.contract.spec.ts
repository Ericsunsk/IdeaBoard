import { describe, expect, it } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type ImageNodeItem,
} from '../../../../board'
import { CanvasGraphSession } from '../../../core/graph'
import { BoardGraphSession, type PendingImageResultNodeId } from './boardGraphSession'
import {
  createPendingImageExecutionCore,
  type PendingImageExecutionCore,
  type PendingImageExecutionLease,
  type PendingImageExecutionStartResult,
} from './core'
import { projectPendingImageExecutionEdges } from './edgeProjection'
import type { BoardAttachment } from '../board-attachment'

function imageNodeId(id: number): PendingImageResultNodeId {
  return `image-${id}`
}

function createPendingImageNode(id: number, taskId: string): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x: id * 40,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: taskId,
    pendingResultIndex: id - 1,
    pendingClientActionId: `action-${taskId}`,
    outputStatus: 'running',
    outputImages: [],
  }
}

function attachCore(taskIds: readonly string[]): {
  core: PendingImageExecutionCore
  attachment: BoardAttachment
} {
  const snapshot: BoardSnapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = taskIds.map((taskId, index) => createPendingImageNode(index + 1, taskId))
  const store = CanvasGraphSession.hydrate('board-a', snapshot)
  const core = createPendingImageExecutionCore()
  const attachment = core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
  return { core, attachment }
}

function requireLease(result: PendingImageExecutionStartResult): PendingImageExecutionLease {
  if (result.kind !== 'started') throw new Error(`Expected started, received ${result.reason}`)
  return result.lease
}

function startRunning(
  core: PendingImageExecutionCore,
  attachment: BoardAttachment,
  nodeId: PendingImageResultNodeId,
  taskId: string,
): PendingImageExecutionLease {
  const lease = requireLease(core.begin(attachment, nodeId))
  expect(core.markSubmitted(lease, taskId, `action-${taskId}`)).toMatchObject({ kind: 'applied' })
  return lease
}

function imageInputEdge(id: string, target: PendingImageResultNodeId) {
  return { id, source: 'prompt-1', target, targetHandle: 'in:image' }
}

describe('flow cancel and render projection', () => {
  it('performs the returned cancellation and projects render edges from immutable facts', () => {
    const { core, attachment } = attachCore(['task-1', 'task-2'])
    const firstLease = startRunning(core, attachment, imageNodeId(1), 'task-1')
    startRunning(core, attachment, imageNodeId(2), 'task-2')

    const baseEdges = [imageInputEdge('edge-1', imageNodeId(1)), imageInputEdge('edge-2', imageNodeId(2))]
    for (const edge of baseEdges) Object.freeze(edge)
    Object.freeze(baseEdges)

    const running = projectPendingImageExecutionEdges(baseEdges, core.snapshot())
    expect(running.edges[0]).toMatchObject({ animated: true })
    expect(running.edges[1]).toMatchObject({ animated: true })

    const cancellation = core.cancel([firstLease])
    expect(cancellation).toEqual({
      outcomes: [{ resultNodeId: imageNodeId(1), kind: 'cancelled' }],
      cancellableTaskIds: ['task-1'],
    })
    // The Flow runtime performs cancellation with exactly the returned task ids.
    expect([...cancellation.cancellableTaskIds]).toEqual(['task-1'])

    const snapshot = core.snapshot()
    expect(Object.isFrozen(snapshot)).toBe(true)
    expect(Object.isFrozen(snapshot.executions)).toBe(true)

    const render = projectPendingImageExecutionEdges(baseEdges, snapshot)
    expect(render).toMatchObject({ didChange: true })
    expect(render.edges[0]).toMatchObject({ animated: false })
    expect(render.edges[1]).toMatchObject({ animated: true })

    // No duplicate mutable edge store: the immutable base edges are untouched,
    // re-projection derives equal edges, and projecting rendered edges again is
    // a stable no-op returning the identical array.
    expect(baseEdges[0]).not.toHaveProperty('animated')
    const repeat = projectPendingImageExecutionEdges(baseEdges, snapshot)
    expect(repeat.edges).toEqual(render.edges)
    const idempotent = projectPendingImageExecutionEdges(render.edges, snapshot)
    expect(idempotent).toMatchObject({ didChange: false })
    expect(idempotent.edges).toBe(render.edges)
  })

  it('returns the shared task for cancellation only after its final slot releases', () => {
    const { core, attachment } = attachCore(['task-shared', 'task-shared'])
    const firstLease = startRunning(core, attachment, imageNodeId(1), 'task-shared')
    const secondLease = startRunning(core, attachment, imageNodeId(2), 'task-shared')

    expect(core.cancel([firstLease])).toMatchObject({ cancellableTaskIds: [] })
    const release = core.release([secondLease])
    expect(release).toMatchObject({
      outcomes: [{ resultNodeId: imageNodeId(2), kind: 'released' }],
      cancellableTaskIds: ['task-shared'],
    })

    const render = projectPendingImageExecutionEdges(
      [imageInputEdge('edge-1', imageNodeId(1)), imageInputEdge('edge-2', imageNodeId(2))],
      core.snapshot(),
    )
    expect(render.edges[0]).toMatchObject({ animated: false })
    expect(render.edges[1]).toMatchObject({ animated: false })
  })
})
