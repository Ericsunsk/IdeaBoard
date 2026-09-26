import { TaskCancelledError } from '../task-runner/taskResultErrors'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../../board'
import { CanvasGraphSession } from '../../../domain/graph'
import { BoardGraphSession, createPendingImageExecutionCore } from '../pending-execution'
import { createCanvasExecutionRecoveryCoordinator } from './canvasExecutionRecoveryCoordinator'

function pendingSnapshot() {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes.push({
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: 'task-1',
    pendingResultIndex: 0,
    pendingClientActionId: 'action-1',
    outputStatus: 'running',
    outputImages: [],
  })
  return snapshot
}

function createHarness(options: { onMaterialized?: (() => void) | undefined } = {}) {
  const snapshot = pendingSnapshot()
  const graph = CanvasGraphSession.hydrate('board-a', snapshot)
  const core = createPendingImageExecutionCore()
  core.attachBoard(
    BoardGraphSession.create('board-a', {
      graphSession: graph,
      onMaterialized: options.onMaterialized,
    }),
  )
  return { snapshot, graph, core }
}

function createCoordinator(
  harness: ReturnType<typeof createHarness>,
  resolveResult: Parameters<typeof createCanvasExecutionRecoveryCoordinator>[0]['resolveResult'],
  extras: Partial<Parameters<typeof createCanvasExecutionRecoveryCoordinator>[0]> = {},
) {
  return createCanvasExecutionRecoveryCoordinator({
    executionCore: harness.core,
    readBoardSnapshot: () => harness.graph.exportSnapshot(harness.snapshot),
    resolveResult,
    flushAutosave: async () => undefined,
    cancelTasks: () => undefined,
    onWarning: () => undefined,
    ...extras,
  })
}

describe('Canvas Execution Recovery Coordinator', () => {
  it('hydrates and polls each pending task once across concurrent recovery calls', async () => {
    const harness = createHarness()
    let resolveResult!: (value: { url: string; ratio: number }) => void
    const resolve = vi.fn(
      () =>
        new Promise<{ url: string; ratio: number }>((resolveNext) => {
          resolveResult = resolveNext
        }),
    )
    const flushAutosave = vi.fn(async () => undefined)
    const coordinator = createCoordinator(harness, resolve, { flushAutosave })

    const first = coordinator.recover()
    const duplicate = coordinator.recover()
    expect(duplicate).toBe(first)
    await vi.waitFor(() => expect(resolve).toHaveBeenCalledOnce())
    resolveResult({ url: 'https://example.com/result.png', ratio: 1 })
    const result = await first

    expect(result?.recoveredNodeIds).toEqual(['image-1'])
    expect(flushAutosave).toHaveBeenCalledOnce()
    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'succeeded', taskId: 'task-1' })
  })

  it('ignores a result that resolves after the Board attachment changes', async () => {
    const harness = createHarness()
    let resolveResult!: (value: { url: string; ratio: number }) => void
    const coordinator = createCoordinator(
      harness,
      () =>
        new Promise((resolve) => {
          resolveResult = resolve
        }),
    )
    const recovery = coordinator.recover()
    await vi.waitFor(() =>
      expect(harness.core.factFor('image-1')).toMatchObject({ status: 'running' }),
    )

    const nextGraph = CanvasGraphSession.hydrate('board-b', createEmptyBoardSnapshot())
    harness.core.attachBoard(BoardGraphSession.create('board-b', { graphSession: nextGraph }))
    resolveResult({ url: 'https://example.com/stale.png', ratio: 1 })
    await recovery

    expect(harness.core.snapshot().attachment).toMatchObject({ boardId: 'board-b' })
    expect(harness.core.factFor('image-1')).toBeNull()
    const staleNode = harness.graph.store.getNode('image-1')
    if (!staleNode || staleNode.snapshot.type !== 'image-node') {
      throw new Error('Expected image node')
    }
    expect(staleNode.snapshot.pendingTaskId).toBe('task-1')
  })

  it('cancels a task when the recovery transport reports cancellation', async () => {
    const harness = createHarness()
    const cancelTasks = vi.fn()
    const coordinator = createCoordinator(
      harness,
      async () => {
        throw new TaskCancelledError()
      },
      { cancelTasks },
    )

    await coordinator.recover()

    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'cancelled', taskId: 'task-1' })
    expect(cancelTasks).toHaveBeenCalledWith(['task-1'])
  })

  it('routes materialization failures through the unified error callback', async () => {
    const failure = new Error('materialization failed')
    const harness = createHarness({
      onMaterialized: () => {
        throw failure
      },
    })
    const onError = vi.fn(() => true)
    const coordinator = createCoordinator(
      harness,
      async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      { onError },
    )

    const result = await coordinator.recover()

    expect(result?.failedNodeIds).toEqual(['image-1'])
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'PendingImageMaterializationError' }),
    )
    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'failed', taskId: 'task-1' })
  })
})
