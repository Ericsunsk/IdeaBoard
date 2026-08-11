import { describe, expect, it, vi } from 'vitest'
import type {
  PendingImageResultExecution,
  PendingImageResultRunSummary,
  RunPendingImageResultsParams,
} from '../result-nodes/usePendingImageResultNodes'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'
import { CanvasBillingPreflightError } from '../task-runner/canvasBillingPreflight'
import { createCanvasExecutionRuntime } from './CanvasExecutionRuntime'

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
    ...overrides,
  }
}

function createPendingResults(
  runPendingResults: (
    params: RunPendingImageResultsParams,
  ) => Promise<PendingImageResultRunSummary>,
): Pick<PendingImageResultExecution, 'runPendingResults'> {
  return { runPendingResults: vi.fn(runPendingResults) }
}

function createRunner(
  pendingResults: Pick<PendingImageResultExecution, 'runPendingResults'>,
  overrides: Partial<Parameters<typeof createPendingImageWorkflowRunner>[0]> = {},
) {
  const executionRuntime = createCanvasExecutionRuntime()
  const attachment = executionRuntime.attachBoard('board-a')
  const onSuccess = vi.fn()
  const onWarning = vi.fn()
  const runner = createPendingImageWorkflowRunner({
    pendingResults,
    executionRuntime,
    onSuccess,
    onWarning,
    ...overrides,
  })
  return { attachment, executionRuntime, onSuccess, onWarning, runner }
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
    activationId: 'canvas:activation-1',
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
      params.onReserved?.(['image-2'])
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

    expect(pendingResults.runPendingResults).not.toHaveBeenCalled()
    expect(executionRuntime.nodeStates.value.size).toBe(0)
    expect(onWarning).toHaveBeenCalledWith('生图服务暂不可用，请稍后重试')
  })

  it('runs prepared results through one attachment-scoped execution lease', async () => {
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2'])
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'running', taskId: 'task-1' },
      })
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'succeeded', taskId: 'task-1' },
      })
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

  it('rejects old completion after the same Board opens a new attachment epoch', async () => {
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2'])
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'running', taskId: 'task-old' },
      })
      return makeRunSummary()
    })
    const { attachment, executionRuntime, onSuccess, onWarning, runner } =
      createRunner(pendingResults)
    const run = runner.run(makeRunParams(attachment))
    executionRuntime.attachBoard('board-a')
    await run

    expect(executionRuntime.getNodeState('image-2')).toBeNull()
    expect(onSuccess).not.toHaveBeenCalled()
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('keeps multi-output execution leases isolated', async () => {
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2', 'image-3'])
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'succeeded', taskId: 'task-2' },
      })
      params.onExecutionStateChange?.({
        nodeId: 'image-3',
        state: { status: 'failed', taskId: 'task-3' },
      })
      return makeRunSummary({ totalCount: 2, successCount: 1, failureCount: 1 })
    })
    const { attachment, executionRuntime, runner } = createRunner(pendingResults)

    await runner.run(makeRunParams(attachment, { outputCount: 2 }))

    expect(executionRuntime.getNodeState('image-2')).toEqual({
      status: 'succeeded',
      taskId: 'task-2',
    })
    expect(executionRuntime.getNodeState('image-3')).toEqual({
      status: 'failed',
      taskId: 'task-3',
    })
  })

  it('routes typed billing failures through the error port', async () => {
    const insufficient = new CanvasBillingPreflightError('insufficient-credits', 'Idea不足')
    const onError = vi.fn(() => true)
    const pendingResults = createPendingResults(async () =>
      makeRunSummary({
        totalCount: 2,
        successCount: 1,
        failureCount: 1,
        firstFailure: insufficient,
        failures: [insufficient],
      }),
    )
    const { attachment, onWarning, runner } = createRunner(pendingResults, { onError })

    await runner.run(makeRunParams(attachment, { outputCount: 2 }))

    expect(onError).toHaveBeenCalledWith(insufficient)
    expect(onWarning).not.toHaveBeenCalled()
  })
})
