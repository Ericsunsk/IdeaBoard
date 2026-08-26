import { describe, expect, it, vi } from 'vitest'
import { TaskCancelledError, TaskResultPendingError } from '../../../../shared/api/taskEvents'
import { createEmptyCanvasBoardSnapshot, type ImageNodeItem } from '../../../canvas-board/domain'
import { CanvasGraphSession } from '../../graph-runtime'
import { CanvasBoardGraphSession, createPendingImageExecutionCore } from '../pending-execution'
import { CanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'
import type { PendingImageWorkflowTaskTransport } from './pendingImageWorkflowRunner'

function createHarness(taskIds: readonly string[] = ['task-1']) {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.imageNodes = taskIds.map(
    (taskId, index) =>
      ({
        id: index + 1,
        type: 'image-node',
        x: index * 320,
        y: 0,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        pendingTaskId: taskId,
        pendingResultIndex: index,
        outputStatus: 'running',
        outputImages: [],
      }) satisfies ImageNodeItem,
  )
  const store = CanvasGraphSession.hydrate('board-a', snapshot)
  const core = createPendingImageExecutionCore()
  const attachment = core.attachBoard(
    CanvasBoardGraphSession.create('board-a', { graphSession: store }),
  )
  const pendingResults = {
    reservePendingResults: vi.fn(() => ({
      resultNodeIds: taskIds.map((_, index) => `image-${index + 1}`),
    })),
    attachPendingTaskIds: vi.fn(),
    removePendingResults: vi.fn(),
  }
  const assertReady = vi.fn<PendingImageWorkflowTaskTransport['assertReady']>(async () => undefined)
  const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>(async () => ({
    taskId: 'task-1',
  }))
  const resolve = vi.fn<PendingImageWorkflowTaskTransport['resolve']>(
    async (_taskId, resultIndex) => ({
      url: `https://example.com/${resultIndex}.png`,
      ratio: 1,
    }),
  )
  const transport = { assertReady, submit, resolve }
  const cancelTasks = vi.fn()
  const runner = createPendingImageWorkflowRunner({
    executionCore: core,
    pendingResults,
    taskTransport: transport,
    cancelTasks,
    onSuccess: vi.fn(),
    onWarning: vi.fn(),
    createClientActionId: () => 'action-1',
  })
  const intent = {
    attachment,
    source: { url: 'https://example.com/input.png', ratio: 1 },
    sourceNodeId: 'image-0',
    outputCount: taskIds.length,
    taskRequest: {} as never,
    readinessRequest: {} as never,
    fallbackRatio: 1,
  }
  return { attachment, cancelTasks, core, intent, pendingResults, runner, store, transport }
}

describe('pendingImageWorkflowRunner', () => {
  it('executes one shared prepared intent through stable transport adapters', async () => {
    const harness = createHarness(['task-1', 'task-1'])
    await harness.runner.execute(harness.intent)
    expect(harness.transport.assertReady).toHaveBeenCalledWith(
      harness.intent.readinessRequest,
      'Image generation failed',
    )
    expect(harness.transport.submit).toHaveBeenCalledWith(
      harness.intent.taskRequest,
      'action-1',
      'Image generation failed',
    )
    expect(harness.pendingResults.attachPendingTaskIds).toHaveBeenCalledWith([
      { resultNodeId: 'image-1', resultIndex: 0, taskId: 'task-1', clientActionId: 'action-1' },
      { resultNodeId: 'image-2', resultIndex: 1, taskId: 'task-1', clientActionId: 'action-1' },
    ])
    expect(harness.core.snapshot().executions.every((fact) => fact.status === 'succeeded')).toBe(
      true,
    )
  })

  it('does not reserve when the injected readiness transport rejects', async () => {
    const harness = createHarness()
    harness.transport.assertReady.mockRejectedValueOnce(new Error('余额不足'))
    await harness.runner.execute(harness.intent)
    expect(harness.pendingResults.reservePendingResults).not.toHaveBeenCalled()
  })

  it('retries transient submit with one action id and preserves pending recovery handles', async () => {
    const harness = createHarness()
    harness.transport.submit
      .mockRejectedValueOnce(new CanvasBillingPreflightError('network', '网络错误'))
      .mockResolvedValueOnce({ taskId: 'task-1' })
    harness.transport.resolve.mockRejectedValueOnce(new TaskResultPendingError())
    await harness.runner.execute(harness.intent)
    expect(harness.transport.submit.mock.calls.map((call) => call[1])).toEqual([
      'action-1',
      'action-1',
    ])
    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'running', taskId: 'task-1' })
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
  })

  it('does not remove a superseding slot after a stale failure or late submit', async () => {
    const harness = createHarness()
    let resolveSubmit!: (value: { taskId: string }) => void
    harness.transport.submit.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSubmit = resolve
        }),
    )
    const running = harness.runner.execute(harness.intent)
    await vi.waitFor(() =>
      expect(harness.pendingResults.reservePendingResults).toHaveBeenCalledOnce(),
    )
    const superseding = harness.core.begin(harness.attachment, 'image-1')
    if (superseding.kind !== 'started') throw new Error('Expected superseding execution')
    harness.core.markSubmitted(superseding.lease, 'task-new', 'action-task-new')
    resolveSubmit({ taskId: 'task-old' })
    await running
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'running', taskId: 'task-new' })
  })

  it('keeps pending nodes when graph materialization fails so reattach can recover them', async () => {
    const harness = createHarness()
    vi.spyOn(harness.store.store, 'beginTransaction').mockImplementation(() => {
      throw new Error('graph write failed')
    })
    await harness.runner.execute(harness.intent)
    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'failed', taskId: 'task-1' })
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
  })

  it('cancels and removes a reserved slot before a task id exists', async () => {
    const harness = createHarness()
    harness.transport.submit.mockRejectedValueOnce(new TaskCancelledError())
    await harness.runner.execute(harness.intent)
    expect(harness.pendingResults.removePendingResults).toHaveBeenCalledWith(['image-1'])
    expect(harness.cancelTasks).toHaveBeenCalledWith([])
  })
})
