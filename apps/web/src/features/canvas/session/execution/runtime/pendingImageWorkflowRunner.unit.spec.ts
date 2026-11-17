import { createTaskImageObserver, type TaskImageObservation } from '../task-runner/taskImageOutcome'
import { extractWorkspaceTaskImageResults } from '../workspace/canvasTaskWaiting'
import { CANVAS_FOREGROUND_WAIT_MS, CANVAS_TOTAL_WAIT_MS } from '../canvasBudgets'
import { afterEach, describe, expect, it, test, vi } from 'vitest'
import { TaskCancelledError, TaskResultPendingError } from '../task-runner/taskResultErrors'
import { i18n } from '../../../../../core/i18n'
import { loadLocaleMessages } from '../../../../../core/i18n/userLocale'
import { createEmptyBoardSnapshot, type ImageNodeItem } from '../../../../board'
import { CanvasGraphSession } from '../../board-session'
import { BoardGraphSession } from '../pending-execution/boardGraphSession'
import { createPendingImageExecutionCore } from '../pending-execution/core'
import { CanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'
import type { PendingImageWorkflowTaskTransport } from './pendingImageWorkflowRunner'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'

function createHarness(taskIds: readonly string[] = ['task-1']) {
  const snapshot = createEmptyBoardSnapshot()
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
  const attachment = core.attachBoard(BoardGraphSession.create('board-a', { graphSession: store }))
  const pendingResults = {
    reservePendingResults: vi.fn(() => ({
      resultNodeIds: taskIds.map((_, index) => `image-${index + 1}`),
    })),
    attachPendingTaskIds: vi.fn(),
    removePendingResults: vi.fn(),
  }
  const submit = vi.fn<PendingImageWorkflowTaskTransport['submit']>(async () => ({
    taskId: 'task-1',
  }))
  const resolve = vi.fn<TaskImageObservation['resolve']>(async (resultIndex) => ({
    kind: 'completed',
    result: { url: `https://example.com/${resultIndex}.png`, ratio: 1 },
  }))
  const observe = vi.fn<PendingImageWorkflowTaskTransport['observe']>(async () => ({ resolve }))
  const transport = { submit, observe }
  const cancelTasks = vi.fn()
  const onSuccess = vi.fn()
  const onWarning = vi.fn()
  const onError = vi.fn(() => false)
  const afterTaskIdAttached = vi.fn(async () => undefined)
  const runner = createPendingImageWorkflowRunner({
    executionCore: core,
    pendingResults,
    taskTransport: transport,
    cancelTasks,
    onSuccess,
    onWarning,
    onError,
    afterTaskIdAttached,
    createClientActionId: () => 'action-1',
  })
  const intent = {
    attachment,
    source: { url: 'https://example.com/input.png', ratio: 1 },
    sourceNodeId: 'image-0',
    outputCount: taskIds.length,
    taskRequest: {} as never,
    fallbackRatio: 1,
  }
  return {
    attachment,
    afterTaskIdAttached,
    cancelTasks,
    core,
    intent,
    onSuccess,
    onWarning,
    onError,
    pendingResults,
    runner,
    store,
    transport,
    resolve,
  }
}

afterEach(() => {
  vi.useRealTimers()
  i18n.global.locale.value = 'en'
})

describe('pendingImageWorkflowRunner', () => {
  it('does not retry or delete pending nodes when an in-flight submit fails after disposal', async () => {
    const harness = createHarness()
    let rejectSubmit!: (error: unknown) => void
    harness.transport.submit.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectSubmit = reject
        }),
    )
    const execution = harness.runner.execute(harness.intent)
    await vi.waitFor(() => expect(harness.transport.submit).toHaveBeenCalledOnce())
    harness.runner.dispose()
    rejectSubmit(new CanvasBillingPreflightError('network', 'Response lost'))
    await execution
    expect(harness.transport.submit).toHaveBeenCalledOnce()
    expect(harness.core.factFor('image-1')?.status).toBe('submitting')
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
    expect(harness.cancelTasks).not.toHaveBeenCalled()
    expect(harness.onError).not.toHaveBeenCalled()
    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('persists a late confirmed submit for recovery without reading or presenting after disposal', async () => {
    const harness = createHarness()
    let resolveSubmit!: (value: { taskId: string }) => void
    harness.transport.submit.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSubmit = resolve
        }),
    )
    const execution = harness.runner.execute(harness.intent)
    await vi.waitFor(() => expect(harness.transport.submit).toHaveBeenCalledOnce())
    harness.runner.dispose()
    resolveSubmit({ taskId: 'task-1' })
    await execution
    expect(harness.core.factFor('image-1')).toMatchObject({
      status: 'running',
      taskId: 'task-1',
      clientActionId: 'action-1',
    })
    expect(harness.pendingResults.attachPendingTaskIds).toHaveBeenCalledExactlyOnceWith([
      { resultNodeId: 'image-1', resultIndex: 0, taskId: 'task-1', clientActionId: 'action-1' },
    ])
    expect(harness.afterTaskIdAttached).toHaveBeenCalledOnce()
    expect(harness.resolve).not.toHaveBeenCalled()
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
    expect(harness.cancelTasks).not.toHaveBeenCalled()
    expect(harness.onError).not.toHaveBeenCalled()
    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.onSuccess).not.toHaveBeenCalled()
  })

  it('executes one shared prepared intent through stable transport adapters optimistically', async () => {
    const harness = createHarness(['task-1', 'task-1'])
    await harness.runner.execute(harness.intent)
    expect(harness.transport.submit).toHaveBeenCalledWith(harness.intent.taskRequest, 'action-1')
    expect(harness.pendingResults.attachPendingTaskIds).toHaveBeenCalledWith([
      { resultNodeId: 'image-1', resultIndex: 0, taskId: 'task-1', clientActionId: 'action-1' },
      { resultNodeId: 'image-2', resultIndex: 1, taskId: 'task-1', clientActionId: 'action-1' },
    ])
    expect(harness.core.snapshot().executions.every((fact) => fact.status === 'succeeded')).toBe(
      true,
    )
    expect(harness.onSuccess).toHaveBeenCalledOnce()
    expect(harness.onSuccess).toHaveBeenCalledWith('Image generated')
    expect(harness.onWarning).not.toHaveBeenCalled()
    expect(harness.transport.observe).toHaveBeenCalledOnce()
    expect(harness.resolve).toHaveBeenCalledTimes(2)
  })

  it('keeps the one available output and cancels three missing slots after partial cancellation', async () => {
    const harness = createHarness(['task-1', 'task-1', 'task-1', 'task-1'])
    harness.resolve.mockImplementation(async (resultIndex) => ({
      kind: 'cancelled',
      result: resultIndex === 0 ? { url: 'https://example.com/partial.png', ratio: 0.5 } : null,
    }))
    await harness.runner.execute(harness.intent)
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'succeeded',
      'cancelled',
      'cancelled',
      'cancelled',
    ])
    expect(harness.onWarning).toHaveBeenCalledExactlyOnceWith(
      'Task cancelled — partial results kept on the canvas.',
    )
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.cancelTasks).not.toHaveBeenCalled()
    expect(harness.transport.observe).toHaveBeenCalledOnce()
  })

  test('keeps partial completed outputs and settles every missing slot without polling again', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/ws_task123/media-assets/asset_result123', () =>
        HttpResponse.json({ data: { width: 400, height: 200 } }),
      ),
    )
    const harness = createHarness(['task-1', 'task-1', 'task-1', 'task-1'])
    const results = extractWorkspaceTaskImageResults('ws_task123', [
      { kind: 'asset', asset_id: 'asset_result123', role: 'image', result_index: 0 },
    ])
    const wait = vi.fn(async () => ({ kind: 'completed' as const, results }))
    harness.transport.observe.mockImplementation(
      createTaskImageObserver(
        wait,
        async () => ({ width: 400, height: 200 }),
        async () => {},
      ),
    )
    await harness.runner.execute(harness.intent)
    expect(harness.core.snapshot().executions.map((fact) => fact.status)).toEqual([
      'succeeded',
      'failed',
      'failed',
      'failed',
    ])
    expect(harness.pendingResults.removePendingResults).toHaveBeenCalledExactlyOnceWith([
      'image-2',
      'image-3',
      'image-4',
    ])
    expect(wait).toHaveBeenCalledOnce()
    expect(harness.transport.observe).toHaveBeenCalledOnce()
    expect(harness.cancelTasks).not.toHaveBeenCalled()
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledOnce()
  })

  it.each(['completed', 'failed'] as const)(
    'continues other pending slots after a background slot %s',
    async (kind) => {
      const harness = createHarness(['task-1', 'task-1'])
      const callbacks: Array<() => void> = []
      const calls = [0, 0]
      harness.resolve.mockImplementation(async (resultIndex) => {
        if (resultIndex === undefined) throw new Error('Expected indexed output')
        calls[resultIndex] = (calls[resultIndex] ?? 0) + 1
        if (calls[resultIndex] === 1 || (resultIndex === 1 && calls[resultIndex] === 2)) {
          throw new TaskResultPendingError('timeout')
        }
        if (kind === 'failed' && resultIndex === 0) throw new Error('provider failed')
        return {
          kind: 'completed',
          result: { url: `https://example.com/${resultIndex}.png`, ratio: 1 },
        }
      })
      const runner = createPendingImageWorkflowRunner({
        executionCore: harness.core,
        pendingResults: harness.pendingResults,
        taskTransport: harness.transport,
        cancelTasks: harness.cancelTasks,
        onSuccess: harness.onSuccess,
        onWarning: harness.onWarning,
        scheduleContinuation: (callback) => {
          callbacks.push(callback)
          return () => undefined
        },
      })
      await runner.execute(harness.intent)
      expect(callbacks).toHaveLength(1)
      callbacks[0]?.()
      await vi.waitFor(() => expect(callbacks).toHaveLength(2))
      expect(harness.core.factFor('image-2')?.status).toBe('running')
      callbacks[1]?.()
      await vi.waitFor(() => expect(harness.core.factFor('image-2')?.status).toBe('succeeded'))
      expect(harness.core.factFor('image-1')?.status).toBe(
        kind === 'completed' ? 'succeeded' : 'failed',
      )
      expect(callbacks).toHaveLength(2)
      expect(harness.transport.observe).toHaveBeenCalledOnce()
      runner.dispose()
    },
  )

  it('keeps foreground and background I/O inside one total deadline, preserving pending results at expiry', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(0)
    const harness = createHarness()
    harness.resolve.mockImplementation(() => new Promise(() => undefined))
    const execution = harness.runner.execute(harness.intent)
    await vi.advanceTimersByTimeAsync(CANVAS_FOREGROUND_WAIT_MS)
    await execution
    expect(harness.resolve).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.resolve).toHaveBeenCalledTimes(2)
    expect(harness.resolve.mock.calls[0]?.[2].deadlineAt).toBe(CANVAS_FOREGROUND_WAIT_MS)
    expect(harness.resolve.mock.calls[1]?.[2].deadlineAt).toBe(CANVAS_TOTAL_WAIT_MS)
    await vi.advanceTimersByTimeAsync(CANVAS_TOTAL_WAIT_MS - CANVAS_FOREGROUND_WAIT_MS - 5_000)
    expect(Date.now()).toBe(675_000)
    expect(harness.core.factFor('image-1')?.status).toBe('running')
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
    expect(harness.cancelTasks).not.toHaveBeenCalled()
    expect(harness.resolve.mock.calls.every((call) => call[2].signal.aborted)).toBe(true)
    expect(
      harness.onWarning.mock.calls.filter(
        ([message]) => message === i18n.global.t('canvas.task.progressTimeout'),
      ),
    ).toHaveLength(1)
    expect(vi.getTimerCount()).toBe(0)
    expect(harness.transport.observe).toHaveBeenCalledOnce()
  })

  it('retries a hanging Task observation in the background under the same total deadline', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(0)
    const harness = createHarness(['task-1', 'task-1', 'task-1', 'task-1'])
    harness.transport.observe.mockImplementationOnce(() => new Promise(() => undefined))
    const execution = harness.runner.execute(harness.intent)
    await vi.advanceTimersByTimeAsync(CANVAS_FOREGROUND_WAIT_MS)
    await execution
    expect(harness.transport.observe).toHaveBeenCalledOnce()
    expect(harness.transport.observe.mock.calls[0]?.[1].signal.aborted).toBe(true)
    expect(harness.resolve).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(5_000)
    expect(harness.transport.observe).toHaveBeenCalledTimes(2)
    expect(harness.transport.observe.mock.calls[1]?.[1].deadlineAt).toBe(CANVAS_TOTAL_WAIT_MS)
    expect(harness.resolve).toHaveBeenCalledTimes(4)
    expect(harness.core.snapshot().executions.every((fact) => fact.status === 'succeeded')).toBe(
      true,
    )
    expect(vi.getTimerCount()).toBe(0)
  })

  it('keeps the shared Task observation alive for slots that have not been superseded', async () => {
    const harness = createHarness(['task-1', 'task-1'])
    let finishObservation!: (observation: TaskImageObservation) => void
    harness.transport.observe.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishObservation = resolve
        }),
    )
    const execution = harness.runner.execute(harness.intent)
    await vi.waitFor(() => expect(harness.transport.observe).toHaveBeenCalledOnce())
    harness.core.begin(harness.attachment, 'image-1')
    expect(harness.transport.observe.mock.calls[0]?.[1].signal.aborted).toBe(false)
    finishObservation({ resolve: harness.resolve })
    await execution
    expect(harness.resolve).toHaveBeenCalledOnce()
    expect(harness.resolve.mock.calls[0]?.[0]).toBe(1)
    expect(harness.core.factFor('image-1')?.status).toBe('submitting')
    expect(harness.core.factFor('image-2')?.status).toBe('succeeded')
  })

  it.each(['detach', 'supersede', 'dispose'] as const)(
    'aborts one shared Task observation on %s',
    async (action) => {
      const harness = createHarness(['task-1', 'task-1'])
      harness.transport.observe.mockImplementationOnce(() => new Promise(() => undefined))
      const execution = harness.runner.execute(harness.intent)
      await vi.waitFor(() => expect(harness.transport.observe).toHaveBeenCalledOnce())
      const signal = harness.transport.observe.mock.calls[0]?.[1].signal
      const onAbort = vi.fn()
      signal?.addEventListener('abort', onAbort)
      if (action === 'detach') harness.core.detachBoard()
      else if (action === 'dispose') harness.runner.dispose()
      else {
        harness.core.begin(harness.attachment, 'image-1')
        expect(signal?.aborted).toBe(false)
        harness.core.begin(harness.attachment, 'image-2')
      }
      await execution
      expect(onAbort).toHaveBeenCalledOnce()
      expect(harness.resolve).not.toHaveBeenCalled()
      expect(harness.onWarning).not.toHaveBeenCalled()
      expect(harness.onSuccess).not.toHaveBeenCalled()
    },
  )

  it.each(['detach', 'supersede', 'dispose'] as const)(
    'aborts active reads on %s without stale feedback',
    async (action) => {
      vi.useFakeTimers()
      const harness = createHarness()
      harness.resolve.mockImplementation(() => new Promise(() => undefined))
      const execution = harness.runner.execute(harness.intent)
      await vi.advanceTimersByTimeAsync(0)
      expect(harness.resolve).toHaveBeenCalledOnce()
      const signal = harness.resolve.mock.calls[0]?.[2].signal
      if (action === 'detach') harness.core.detachBoard()
      else if (action === 'dispose') harness.runner.dispose()
      else harness.core.begin(harness.attachment, 'image-1')
      await execution
      expect(signal?.aborted).toBe(true)
      expect(harness.onWarning).not.toHaveBeenCalled()
      expect(harness.onSuccess).not.toHaveBeenCalled()
      expect(vi.getTimerCount()).toBe(0)
    },
  )

  it('cancels an already scheduled background timer on disposal', async () => {
    vi.useFakeTimers()
    const harness = createHarness()
    harness.resolve.mockRejectedValue(new TaskResultPendingError('timeout'))
    await harness.runner.execute(harness.intent)
    expect(vi.getTimerCount()).toBe(1)
    harness.runner.dispose()
    expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(CANVAS_TOTAL_WAIT_MS)
    expect(harness.resolve).toHaveBeenCalledOnce()
  })

  it('uses the active locale for the terminal success message', async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
    const harness = createHarness()

    await harness.runner.execute(harness.intent)

    expect(harness.onSuccess).toHaveBeenCalledWith('图片已生成')
  })

  it('reports one warning and no success when some results are still pending', async () => {
    const harness = createHarness(['task-1', 'task-1'])
    harness.resolve.mockImplementation(async (resultIndex) => {
      if (resultIndex === 1) throw new TaskResultPendingError()
      return { kind: 'completed', result: { url: 'https://example.com/0.png', ratio: 1 } }
    })

    await harness.runner.execute(harness.intent)

    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onWarning).toHaveBeenCalledWith(
      'Tasks are still running. Results will appear on the canvas shortly.',
    )
  })

  it('reports one failure and no success when only some results resolve', async () => {
    const harness = createHarness(['task-1', 'task-1'])
    harness.resolve.mockImplementation(async (resultIndex) => {
      if (resultIndex === 1) throw new Error('provider failure')
      return { kind: 'completed', result: { url: 'https://example.com/0.png', ratio: 1 } }
    })

    await harness.runner.execute(harness.intent)

    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onWarning).toHaveBeenCalledWith('Image generation failed')
  })

  it('prioritizes one hard failure when other results are still pending', async () => {
    const harness = createHarness(['task-1', 'task-1', 'task-1'])
    harness.resolve.mockImplementation(async (resultIndex) => {
      if (resultIndex === 1) throw new TaskResultPendingError()
      if (resultIndex === 2) throw new Error('provider failure')
      return { kind: 'completed', result: { url: 'https://example.com/0.png', ratio: 1 } }
    })

    await harness.runner.execute(harness.intent)

    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onWarning).toHaveBeenCalledWith('Image generation failed')
  })

  it('retries transient submit with one action id and preserves pending recovery handles', async () => {
    const harness = createHarness()
    harness.transport.submit
      .mockRejectedValueOnce(new CanvasBillingPreflightError('network', '网络错误'))
      .mockResolvedValueOnce({ taskId: 'task-1' })
    harness.resolve.mockRejectedValueOnce(new TaskResultPendingError())
    await harness.runner.execute(harness.intent)
    expect(harness.transport.submit.mock.calls.map((call) => call[1])).toEqual([
      'action-1',
      'action-1',
    ])
    expect(harness.core.factFor('image-1')).toMatchObject({ status: 'running', taskId: 'task-1' })
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
  })

  it('surfaces a submit-time empty pool as the clear resolution toast with one attempt', async () => {
    // 422 service_resolution_unavailable creates no task and no hold
    // backend-side; the runner maps it to the clear copy (not the generic
    // failure) and never retries the submit.
    const harness = createHarness()
    harness.transport.submit.mockRejectedValueOnce(
      new CanvasBillingPreflightError(
        'resolution-unavailable',
        'This resolution is unavailable right now.',
      ),
    )
    await harness.runner.execute(harness.intent)
    expect(harness.transport.submit).toHaveBeenCalledTimes(1)
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onWarning).toHaveBeenCalledWith('This resolution is unavailable right now.')
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

  it('retries submit once on transient pricing failures with the shared budget', async () => {
    const harness = createHarness()
    harness.transport.submit
      .mockRejectedValueOnce(
        new CanvasBillingPreflightError(
          'pricing-unavailable',
          'Pricing is temporarily unavailable. Please try again shortly.',
        ),
      )
      .mockResolvedValueOnce({ taskId: 'task-1' })
    await harness.runner.execute(harness.intent)
    expect(harness.transport.submit).toHaveBeenCalledTimes(2)
    expect(harness.transport.submit.mock.calls.map((call) => call[1])).toEqual([
      'action-1',
      'action-1',
    ])
    expect(harness.onSuccess).toHaveBeenCalledOnce()
  })

  it('keeps the lease and recovers through background continuation after a timeout', async () => {
    const snapshot = createEmptyBoardSnapshot()
    snapshot.imageNodes = [
      {
        id: 1,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        pendingTaskId: 'task-1',
        pendingResultIndex: 0,
        outputStatus: 'running',
        outputImages: [],
      } satisfies ImageNodeItem,
    ]
    const store = CanvasGraphSession.hydrate('board-a', snapshot)
    const core = createPendingImageExecutionCore()
    const attachment = core.attachBoard(
      BoardGraphSession.create('board-a', { graphSession: store }),
    )
    const pendingResults = {
      reservePendingResults: vi.fn(() => ({ resultNodeIds: ['image-1'] })),
      attachPendingTaskIds: vi.fn(),
      removePendingResults: vi.fn(),
    }
    const observe = vi
      .fn<PendingImageWorkflowTaskTransport['observe']>()
      .mockRejectedValueOnce(new TaskResultPendingError('timeout'))
      .mockResolvedValueOnce({
        resolve: async () => ({
          kind: 'completed',
          result: { url: 'https://example.com/late.png', ratio: 1 },
        }),
      })
    const transport = {
      submit: vi.fn(async () => ({ taskId: 'task-1' })),
      observe,
    }
    const onSuccess = vi.fn()
    const onWarning = vi.fn()
    const runner = createPendingImageWorkflowRunner({
      executionCore: core,
      pendingResults,
      taskTransport: transport,
      cancelTasks: vi.fn(),
      scheduleContinuation: (callback) => {
        callback()
        return () => undefined
      },
      onSuccess,
      onWarning,
      createClientActionId: () => 'action-1',
    })
    await runner.execute({
      attachment,
      source: { url: 'https://example.com/input.png', ratio: 1 },
      sourceNodeId: 'image-0',
      outputCount: 1,
      taskRequest: {} as never,
      fallbackRatio: 1,
    })
    // 前台超时不断 lease：先报进行中，后台续跑成功后报成功且落子
    expect(onWarning).toHaveBeenCalledWith(
      'Tasks are still running. Results will appear on the canvas shortly.',
    )
    await vi.waitFor(() => expect(onSuccess).toHaveBeenCalledWith('Image generated'))
    expect(core.factFor('image-1')).toMatchObject({ status: 'succeeded', taskId: 'task-1' })
    expect(pendingResults.removePendingResults).not.toHaveBeenCalled()
  })

  it('materializes partial results with an explicit cancelled notice instead of success', async () => {
    const harness = createHarness()
    harness.resolve.mockResolvedValueOnce({
      kind: 'cancelled',
      result: { url: 'https://example.com/0.png', ratio: 1 },
    })
    await harness.runner.execute(harness.intent)
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onWarning).toHaveBeenCalledWith(
      'Task cancelled — partial results kept on the canvas.',
    )
    expect(harness.core.factFor('image-1')).toMatchObject({
      status: 'succeeded',
      taskId: 'task-1',
    })
    expect(harness.pendingResults.removePendingResults).not.toHaveBeenCalled()
  })

  it('renders precise copy for Board-denied submits instead of the generic failure', async () => {
    const harness = createHarness()
    harness.transport.submit.mockRejectedValueOnce(
      Object.assign(new Error('viewer'), { status: 403, code: 'task_forbidden' }),
    )
    await harness.runner.execute(harness.intent)
    expect(harness.onSuccess).not.toHaveBeenCalled()
    expect(harness.onWarning).toHaveBeenCalledWith('You cannot operate this task.')
  })

  it('renders precise copy for archived Boards at submit time', async () => {
    const harness = createHarness()
    harness.transport.submit.mockRejectedValueOnce(
      Object.assign(new Error('archived'), { status: 409, code: 'board_archived' }),
    )
    await harness.runner.execute(harness.intent)
    expect(harness.onWarning).toHaveBeenCalledWith(
      'This board is archived and cannot accept this operation.',
    )
  })
})
