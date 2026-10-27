import type {
  ResolvedTaskImageOutcome,
  TaskImageObservation,
  TaskImageObserver,
} from '../task-runner/taskImageOutcome'
import { TaskCancelledError } from '../task-runner/taskResultErrors'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyBoardSnapshot } from '../../../../board'
import { CanvasGraphSession } from '../../../core/graph'
import { BoardGraphSession } from '../pending-execution/boardGraphSession'
import { createPendingImageExecutionCore } from '../pending-execution/core'
import { createCanvasExecutionRecoveryCoordinator } from './canvasExecutionRecoveryCoordinator'

function pendingSnapshot(taskIds: readonly string[] = ['task-1']) {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = taskIds.map((taskId, index) => ({
    id: index + 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    pendingTaskId: taskId,
    pendingResultIndex: index,
    pendingClientActionId: 'action-1',
    outputStatus: 'running',
    outputImages: [],
  }))
  return snapshot
}

function createHarness(
  options: {
    onMaterialized?: (() => void) | undefined
    taskIds?: readonly string[] | undefined
  } = {},
) {
  const snapshot = pendingSnapshot(options.taskIds)
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
  resolveResult: TaskImageObservation['resolve'],
  extras: Partial<Parameters<typeof createCanvasExecutionRecoveryCoordinator>[0]> = {},
) {
  return createCanvasExecutionRecoveryCoordinator({
    executionCore: harness.core,
    readBoardSnapshot: () => harness.graph.exportSnapshot(harness.snapshot),
    observeTask: async () => ({ resolve: resolveResult }),
    flushAutosave: async () => undefined,
    cancelTasks: () => undefined,
    onWarning: () => undefined,
    ...extras,
  })
}

describe('Canvas Execution Recovery Coordinator', () => {
  it('observes each Task once and resolves its output slots independently', async () => {
    const harness = createHarness({ taskIds: ['task-1', 'task-1', 'task-2', 'task-2'] })
    const resolve = vi.fn<TaskImageObservation['resolve']>(async (resultIndex) => {
      if (resultIndex === 1) throw new Error('display failed')
      return {
        kind: 'completed',
        result: { url: `https://example.com/${resultIndex}.png`, ratio: 1 },
      }
    })
    const observeTask = vi.fn<TaskImageObserver>(async () => ({ resolve }))
    const coordinator = createCoordinator(harness, resolve, { observeTask })

    const result = await coordinator.recover()

    expect(observeTask.mock.calls.map(([taskId]) => taskId)).toEqual(['task-1', 'task-2'])
    expect(resolve).toHaveBeenCalledTimes(4)
    expect(result?.recoveredNodeIds).toEqual(['image-1', 'image-3', 'image-4'])
    expect(result?.failedNodeIds).toEqual(['image-2'])
  })

  it('keeps one partial output and cancels missing slots from one cancelled Task observation', async () => {
    const harness = createHarness({ taskIds: ['task-1', 'task-1', 'task-1', 'task-1'] })
    const resolve = vi.fn<TaskImageObservation['resolve']>(async (resultIndex) => ({
      kind: 'cancelled',
      result: resultIndex === 0 ? { url: 'https://example.com/partial.png', ratio: 1 } : null,
    }))
    const observeTask = vi.fn<TaskImageObserver>(async () => ({ resolve }))
    const onWarning = vi.fn()
    const coordinator = createCoordinator(harness, resolve, { observeTask, onWarning })

    const result = await coordinator.recover()

    expect(observeTask).toHaveBeenCalledOnce()
    expect(result?.recoveredNodeIds).toEqual(['image-1'])
    expect(result?.failedNodeIds).toEqual([])
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'succeeded',
      'cancelled',
      'cancelled',
      'cancelled',
    ])
    expect(onWarning).toHaveBeenCalledExactlyOnceWith(
      'Task cancelled — partial results kept on the canvas.',
    )
  })

  it('keeps a shared observation alive when only one recovered slot is superseded', async () => {
    const harness = createHarness({ taskIds: ['task-1', 'task-1'] })
    const resolve = vi.fn<TaskImageObservation['resolve']>(async () => ({
      kind: 'completed',
      result: { url: 'https://example.com/result.png', ratio: 1 },
    }))
    let finishObservation!: (observation: TaskImageObservation) => void
    const observeTask = vi.fn<TaskImageObserver>(
      () =>
        new Promise((resolveNext) => {
          finishObservation = resolveNext
        }),
    )
    const coordinator = createCoordinator(harness, resolve, { observeTask })
    const recovery = coordinator.recover()
    await vi.waitFor(() => expect(observeTask).toHaveBeenCalledOnce())
    const attachment = harness.core.currentAttachment()
    if (!attachment) throw new Error('Expected attachment')

    harness.core.begin(attachment, 'image-1')
    expect(observeTask.mock.calls[0]?.[1].signal.aborted).toBe(false)
    finishObservation({ resolve })
    const result = await recovery

    expect(resolve).toHaveBeenCalledOnce()
    expect(result?.recoveredNodeIds).toEqual(['image-2'])
    expect(harness.core.factFor('image-1')?.status).toBe('submitting')
    expect(harness.core.factFor('image-2')?.status).toBe('succeeded')
  })

  it.each(['detach', 'supersede', 'dispose'] as const)(
    'aborts the single shared recovery observation on %s',
    async (action) => {
      const harness = createHarness({ taskIds: ['task-1', 'task-1'] })
      const resolve = vi.fn<TaskImageObservation['resolve']>()
      const observeTask = vi.fn<TaskImageObserver>(() => new Promise(() => undefined))
      const onWarning = vi.fn()
      const coordinator = createCoordinator(harness, resolve, { observeTask, onWarning })
      const recovery = coordinator.recover()
      await vi.waitFor(() => expect(observeTask).toHaveBeenCalledOnce())
      const signal = observeTask.mock.calls[0]?.[1].signal
      const onAbort = vi.fn()
      signal?.addEventListener('abort', onAbort)
      if (action === 'detach') harness.core.detachBoard()
      else if (action === 'dispose') coordinator.dispose()
      else {
        const attachment = harness.core.currentAttachment()
        if (!attachment) throw new Error('Expected attachment')
        harness.core.begin(attachment, 'image-1')
        expect(signal?.aborted).toBe(false)
        harness.core.begin(attachment, 'image-2')
      }
      await recovery
      expect(onAbort).toHaveBeenCalledOnce()
      expect(resolve).not.toHaveBeenCalled()
      expect(onWarning).not.toHaveBeenCalled()
    },
  )

  it('hydrates and polls each pending task once across concurrent recovery calls', async () => {
    const harness = createHarness()
    let resolveResult!: (value: ResolvedTaskImageOutcome) => void
    const resolve = vi.fn(
      () =>
        new Promise<ResolvedTaskImageOutcome>((resolveNext) => {
          resolveResult = resolveNext
        }),
    )
    const flushAutosave = vi.fn(async () => undefined)
    const coordinator = createCoordinator(harness, resolve, { flushAutosave })

    const first = coordinator.recover()
    const duplicate = coordinator.recover()
    expect(duplicate).toBe(first)
    await vi.waitFor(() => expect(resolve).toHaveBeenCalledOnce())
    resolveResult({
      kind: 'completed',
      result: { url: 'https://example.com/result.png', ratio: 1 },
    })
    const result = await first

    expect(result?.recoveredNodeIds).toEqual(['image-1'])
    expect(flushAutosave).toHaveBeenCalledOnce()
    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'succeeded', taskId: 'task-1' })
  })

  it('aborts a hanging recovery on disposal without changing pending facts', async () => {
    const harness = createHarness()
    const resolve = vi.fn<TaskImageObservation['resolve']>(() => new Promise(() => undefined))
    const onWarning = vi.fn()
    const coordinator = createCoordinator(harness, resolve, { onWarning })
    const recovery = coordinator.recover()
    await vi.waitFor(() => expect(resolve).toHaveBeenCalledOnce())
    coordinator.dispose()
    await recovery
    expect(resolve.mock.calls[0]?.[2].signal.aborted).toBe(true)
    expect(harness.core.factFor('image-1')?.status).toBe('running')
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('ignores a result that resolves after the Board attachment changes', async () => {
    const harness = createHarness()
    let resolveResult!: (value: ResolvedTaskImageOutcome) => void
    const coordinator = createCoordinator(
      harness,
      () =>
        new Promise((resolve) => {
          resolveResult = resolve
        }),
    )
    const recovery = coordinator.recover()
    await vi.waitFor(() => expect(resolveResult).toBeTypeOf('function'))

    const nextGraph = CanvasGraphSession.hydrate('board-b', createEmptyBoardSnapshot())
    harness.core.attachBoard(BoardGraphSession.create('board-b', { graphSession: nextGraph }))
    resolveResult({ kind: 'completed', result: { url: 'https://example.com/stale.png', ratio: 1 } })
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
      async () => ({
        kind: 'completed',
        result: { url: 'https://example.com/result.png', ratio: 1 },
      }),
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
