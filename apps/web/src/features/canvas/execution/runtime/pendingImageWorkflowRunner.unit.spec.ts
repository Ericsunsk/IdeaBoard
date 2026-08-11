import { describe, expect, it, vi } from 'vitest'
import type {
  PendingImageResultExecution,
  PendingImageResultRunSummary,
  RunReservedPendingImageResultsParams,
} from '../result-nodes/usePendingImageResultNodes'
import { CanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'
import { createCanvasExecutionRuntime } from './CanvasExecutionRuntime'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
}

type RunnerPendingResults = Pick<
  PendingImageResultExecution,
  'reservePendingResults' | 'runReservedPendingResults' | 'cancelPendingResults'
>

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve
  })
  return { promise, resolve }
}

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function makeRunSummary(
  overrides: Partial<PendingImageResultRunSummary> = {},
): PendingImageResultRunSummary {
  return {
    resultNodeIds: ['image-2'],
    totalCount: 1,
    successCount: 1,
    pendingCount: 0,
    failureCount: 0,
    firstFailure: null,
    failures: [],
    outcomes: [{ resultNodeId: 'image-2', status: 'succeeded', taskId: 'task-1' }],
    ...overrides,
  }
}

function createPendingResults(
  runReservedPendingResults: (
    params: RunReservedPendingImageResultsParams,
  ) => Promise<PendingImageResultRunSummary>,
  resultNodeIds: string[] = ['image-2'],
): RunnerPendingResults {
  return {
    reservePendingResults: vi.fn(() => ({ resultNodeIds })),
    runReservedPendingResults: vi.fn(runReservedPendingResults),
    cancelPendingResults: vi.fn(),
  }
}

function createRunner(
  pendingResults: RunnerPendingResults,
  overrides: Partial<Parameters<typeof createPendingImageWorkflowRunner>[0]> = {},
) {
  const executionRuntime = createCanvasExecutionRuntime()
  const attachment = executionRuntime.attachBoard('board-a')
  const cancelTasks = vi.fn()
  const onSuccess = vi.fn()
  const onWarning = vi.fn()
  const runner = createPendingImageWorkflowRunner({
    pendingResults,
    executionRuntime,
    cancelTasks,
    onSuccess,
    onWarning,
    ...overrides,
  })
  return { attachment, cancelTasks, executionRuntime, onSuccess, onWarning, runner }
}

function makeRunParams(
  attachment: ReturnType<typeof createCanvasExecutionRuntime>['attachment']['value'],
  overrides: Record<string, unknown> = {},
) {
  if (!attachment) throw new Error('test attachment is required')
  return {
    source: { url: 'https://example.com/source.png', ratio: 1 },
    sourceNodeId: 'image-1',
    attachment,
    outputCount: 1,
    submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
    resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
    fallbackErrorMessage: '生成失败',
    ...overrides,
  }
}

describe('pendingImageWorkflowRunner', () => {
  it('retries one network transport failure with the same idempotency key and reservation', async () => {
    const submittedClientActionIds: string[] = []
    const pendingResults = createPendingResults(async (params) => {
      await params.submitTask('image-2')
      return makeRunSummary()
    })
    const { attachment, runner } = createRunner(pendingResults, {
      createClientActionId: vi.fn(() => 'client-action-1'),
    })

    await runner.run(
      makeRunParams(attachment, {
        submitTask: vi.fn(async (_resultNodeId: string, clientActionId: string) => {
          submittedClientActionIds.push(clientActionId)
          if (submittedClientActionIds.length === 1) {
            throw new CanvasBillingPreflightError('network', '网络错误，请重试')
          }
          return { taskId: 'task-1' }
        }),
      }),
    )

    expect(submittedClientActionIds).toEqual(['client-action-1', 'client-action-1'])
    expect(pendingResults.reservePendingResults).toHaveBeenCalledTimes(1)
  })

  it('does not reserve execution state when readiness fails', async () => {
    const pendingResults = createPendingResults(async () => makeRunSummary())
    const { attachment, executionRuntime, onWarning, runner } = createRunner(pendingResults)

    await runner.run(
      makeRunParams(attachment, {
        assertReady: vi.fn(async () => {
          throw new Error('生图服务暂不可用，请稍后重试')
        }),
      }),
    )

    expect(pendingResults.reservePendingResults).not.toHaveBeenCalled()
    expect(pendingResults.runReservedPendingResults).not.toHaveBeenCalled()
    expect(executionRuntime.nodeStates.value.size).toBe(0)
    expect(onWarning).toHaveBeenCalledWith('生图服务暂不可用，请稍后重试')
  })

  it('does not reserve after the Board switches while readiness is in flight', async () => {
    const readiness = createDeferred<void>()
    const pendingResults = createPendingResults(async () => makeRunSummary())
    const { attachment, executionRuntime, onSuccess, onWarning, runner } =
      createRunner(pendingResults)

    const run = runner.run(
      makeRunParams(attachment, {
        assertReady: vi.fn(() => readiness.promise),
      }),
    )
    await flushPromises()
    executionRuntime.attachBoard('board-b')
    readiness.resolve(undefined)
    await run

    expect(pendingResults.reservePendingResults).not.toHaveBeenCalled()
    expect(pendingResults.runReservedPendingResults).not.toHaveBeenCalled()
    expect(onSuccess).not.toHaveBeenCalled()
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('applies reserved result outcomes through one attachment-scoped execution lease', async () => {
    const pendingResults = createPendingResults(async (params) => {
      await params.submitTask('image-2')
      return makeRunSummary()
    })
    const { attachment, executionRuntime, onSuccess, runner } = createRunner(pendingResults)

    await runner.run(makeRunParams(attachment))

    expect(executionRuntime.getNodeState('image-2')).toEqual({
      status: 'succeeded',
      taskId: 'task-1',
    })
    expect(onSuccess).toHaveBeenCalledWith('生成完成')
  })

  it('rejects old outcomes after the same Board opens a new attachment epoch', async () => {
    const outcome = createDeferred<PendingImageResultRunSummary>()
    const pendingResults = createPendingResults(async (params) => {
      await params.submitTask('image-2')
      return outcome.promise
    })
    const { attachment, executionRuntime, onSuccess, onWarning, runner } =
      createRunner(pendingResults)

    const run = runner.run(makeRunParams(attachment))
    await flushPromises()
    executionRuntime.attachBoard('board-a')
    outcome.resolve(makeRunSummary())
    await run

    expect(executionRuntime.getNodeState('image-2')).toBeNull()
    expect(onSuccess).not.toHaveBeenCalled()
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('keeps multi-output execution outcomes isolated', async () => {
    const pendingResults = createPendingResults(
      async (params) => {
        await Promise.all(params.resultNodeIds.map((nodeId) => params.submitTask(nodeId)))
        return makeRunSummary({
          resultNodeIds: ['image-2', 'image-3'],
          totalCount: 2,
          successCount: 1,
          failureCount: 1,
          outcomes: [
            { resultNodeId: 'image-2', status: 'succeeded', taskId: 'task-image-2' },
            { resultNodeId: 'image-3', status: 'failed', taskId: 'task-image-3' },
          ],
        })
      },
      ['image-2', 'image-3'],
    )
    const { attachment, executionRuntime, runner } = createRunner(pendingResults)

    await runner.run(
      makeRunParams(attachment, {
        outputCount: 2,
        submitTask: vi.fn(async (resultNodeId: string) => ({ taskId: `task-${resultNodeId}` })),
      }),
    )

    expect(executionRuntime.getNodeState('image-2')).toEqual({
      status: 'succeeded',
      taskId: 'task-image-2',
    })
    expect(executionRuntime.getNodeState('image-3')).toEqual({
      status: 'failed',
      taskId: 'task-image-3',
    })
  })

  it('cancels a late submitted task when its lease was superseded', async () => {
    const submission = createDeferred<{ taskId: string }>()
    const pendingResults = createPendingResults(async (params) => {
      await params.submitTask('image-2')
      return makeRunSummary()
    })
    const { attachment, cancelTasks, executionRuntime, onWarning, runner } =
      createRunner(pendingResults)

    const run = runner.run(
      makeRunParams(attachment, {
        submitTask: vi.fn(() => submission.promise),
      }),
    )
    await flushPromises()
    const replacementLease = executionRuntime.beginSubmission(attachment, 'image-2')
    submission.resolve({ taskId: 'task-late' })
    await run

    expect(replacementLease).not.toBeNull()
    expect(cancelTasks).toHaveBeenCalledWith(['task-late'])
    expect(executionRuntime.getNodeState('image-2')).toEqual({ status: 'submitting' })
    expect(onWarning).toHaveBeenCalledWith('Canvas execution is no longer attached')
  })

  it('drops a late completion after delete cancels its exact lease', async () => {
    const completion = createDeferred<void>()
    const pendingResults = createPendingResults(async (params) => {
      await params.submitTask('image-2')
      await completion.promise
      const isCurrent = params.isResultExecutionCurrent('image-2', 'task-1')
      return makeRunSummary({
        successCount: isCurrent ? 1 : 0,
        outcomes: [
          {
            resultNodeId: 'image-2',
            status: isCurrent ? 'succeeded' : 'cancelled',
            taskId: 'task-1',
          },
        ],
      })
    })
    const { attachment, executionRuntime, onSuccess, onWarning, runner } =
      createRunner(pendingResults)

    const run = runner.run(makeRunParams(attachment))
    await flushPromises()
    const leases = executionRuntime.captureActiveNodeExecutions(['image-2'])
    expect(executionRuntime.cancelNodeExecutions(leases)).toEqual(['task-1'])
    completion.resolve(undefined)
    await run

    expect(executionRuntime.getNodeState('image-2')).toEqual({
      status: 'cancelled',
      taskId: 'task-1',
    })
    expect(onSuccess).not.toHaveBeenCalled()
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('routes typed billing failures from outcomes through the error port', async () => {
    const insufficient = new CanvasBillingPreflightError('insufficient-credits', 'Idea不足')
    const onError = vi.fn(() => true)
    const pendingResults = createPendingResults(async (params) => {
      await params.submitTask('image-2')
      return makeRunSummary({
        totalCount: 2,
        successCount: 1,
        failureCount: 1,
        firstFailure: insufficient,
        failures: [insufficient],
      })
    })
    const { attachment, onWarning, runner } = createRunner(pendingResults, { onError })

    await runner.run(makeRunParams(attachment, { outputCount: 2 }))

    expect(onError).toHaveBeenCalledWith(insufficient)
    expect(onWarning).not.toHaveBeenCalled()
  })
})
