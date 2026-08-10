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
  return {
    runPendingResults: vi.fn(runPendingResults),
  }
}

describe('pendingImageWorkflowRunner', () => {
  it('retries one network transport failure with the same idempotency key and reservation', async () => {
    const submittedClientActionIds: string[] = []
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2'])
      const task = await params.submitTask('image-2')
      expect(task).toEqual({ taskId: 'task-1' })
      return makeRunSummary({ resultNodeIds: ['image-2'] })
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime: createCanvasExecutionRuntime(),
      createClientActionId: vi.fn().mockReturnValueOnce('client-action-1'),
      onSuccess: vi.fn(),
      onWarning: vi.fn(),
    })

    const runParams = {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      outputCount: 1,
      activationId: 'canvas:activation-1',
      submitTask: vi.fn(async (_resultNodeId: string, clientActionId?: string) => {
        submittedClientActionIds.push(clientActionId ?? '')
        if (submittedClientActionIds.length === 1) {
          throw new CanvasBillingPreflightError('network', '网络错误，请重试')
        }
        return { taskId: 'task-1' }
      }),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    }

    await runner.run(runParams)

    expect(pendingResults.runPendingResults).toHaveBeenCalledOnce()
    expect(submittedClientActionIds).toEqual(['client-action-1', 'client-action-1'])
  })

  it('does not retry definitive billing errors', async () => {
    const submitTask = vi.fn(async () => {
      throw new CanvasBillingPreflightError('insufficient-credits', 'Idea不足')
    })
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2'])
      await params.submitTask('image-2')
      return makeRunSummary()
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime: createCanvasExecutionRuntime(),
      onSuccess: vi.fn(),
      onWarning: vi.fn(),
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-2',
      outputCount: 1,
      submitTask,
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    })

    expect(submitTask).toHaveBeenCalledOnce()
  })

  it('does not reserve or mutate result nodes when provider readiness fails', async () => {
    const pendingResults = createPendingResults(async () => makeRunSummary())
    const executionRuntime = createCanvasExecutionRuntime()
    const onWarning = vi.fn()
    const submitTask = vi.fn(async () => ({ taskId: 'task-1' }))
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime,
      onSuccess: vi.fn(),
      onWarning,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-provider-unavailable',
      outputCount: 1,
      assertReady: vi.fn(async () => {
        throw new Error('生图服务暂不可用，请稍后重试')
      }),
      submitTask,
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '执行失败',
    })

    expect(pendingResults.runPendingResults).not.toHaveBeenCalled()
    expect(submitTask).not.toHaveBeenCalled()
    expect(executionRuntime.nodeStates.value.size).toBe(0)
    expect(onWarning).toHaveBeenCalledWith('生图服务暂不可用，请稍后重试')
  })

  it('runs the prepared pending workflow through typed execution states', async () => {
    const events: string[] = []
    const afterTaskIdAttached = vi.fn(async ({ resultNodeId, taskId }) => {
      events.push(`after:${resultNodeId}:${taskId}`)
    })
    const onSuccess = vi.fn((message: string) => {
      events.push(`success:${message}`)
    })
    const onWarning = vi.fn()
    const pendingResults = createPendingResults(async (params) => {
      events.push('run')
      expect(params.source).toEqual({ url: 'https://example.com/prepared.png', ratio: 0.5 })
      expect(params.sourceNodeId).toBe('image-prepared')
      expect(params.outputCount).toBe(2)
      expect(params.resultOptions).toEqual({ targetRatio: 0.5, targetWidth: 512 })
      expect(params.isCurrent()).toBe(true)

      params.onReserved?.(['image-2'])
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'running', taskId: 'task-1' },
      })
      await params.afterTaskIdAttached?.({ resultNodeId: 'image-2', taskId: 'task-1' })
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'succeeded', taskId: 'task-1' },
      })
      return makeRunSummary({ resultNodeIds: ['image-2'], totalCount: 2, successCount: 2 })
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime: createCanvasExecutionRuntime(),
      onExecutionStateChange: ({ nodeId, state }) => {
        events.push(`state:${nodeId}:${state.status}:${state.taskId ?? ''}`)
      },
      afterTaskIdAttached,
      onSuccess,
      onWarning,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-2',
      outputCount: 2,
      resultOptions: { targetRatio: 1 },
      preflight: vi.fn(async () => {
        events.push('preflight')
      }),
      prepare: vi.fn(async () => {
        events.push('prepare')
        return {
          source: { url: 'https://example.com/prepared.png', ratio: 0.5 },
          sourceNodeId: 'image-prepared',
          resultOptions: { targetRatio: 0.5, targetWidth: 512 },
        }
      }),
      assertReady: vi.fn(async () => {
        events.push('assertReady')
      }),
      submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
      resolveResult: vi.fn(async () => ({
        url: 'https://example.com/result.png',
        ratio: 0.5,
      })),
      cleanup: vi.fn(async () => {
        events.push('cleanup')
      }),
      fallbackErrorMessage: '生成失败',
    })

    expect(events).toEqual([
      'preflight',
      'prepare',
      'assertReady',
      'run',
      'state:image-2:submitting:',
      'state:image-2:running:task-1',
      'after:image-2:task-1',
      'state:image-2:succeeded:task-1',
      'success:生成完成（2 张）',
      'cleanup',
    ])
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('skips summary reporting when the board changes before results finish', async () => {
    let isCurrent = true
    const onSuccess = vi.fn()
    const onWarning = vi.fn()
    const executionRuntime = createCanvasExecutionRuntime()
    const pendingResults = createPendingResults(async (params) => {
      expect(params.isCurrent()).toBe(true)
      params.onReserved?.(['image-2'])
      isCurrent = false
      return makeRunSummary()
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => isCurrent,
      executionRuntime,
      onSuccess,
      onWarning,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-3',
      outputCount: 1,
      submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    })

    expect(onSuccess).not.toHaveBeenCalled()
    expect(onWarning).not.toHaveBeenCalled()
    expect(executionRuntime.getNodeState('image-2')).toEqual({ status: 'cancelled' })
  })

  it('keeps multi-output execution states independent through mixed terminal outcomes', async () => {
    const executionRuntime = createCanvasExecutionRuntime()
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2', 'image-3'])
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'running', taskId: 'task-2' },
      })
      params.onExecutionStateChange?.({
        nodeId: 'image-3',
        state: { status: 'running', taskId: 'task-3' },
      })
      params.onExecutionStateChange?.({
        nodeId: 'image-2',
        state: { status: 'succeeded', taskId: 'task-2' },
      })
      params.onExecutionStateChange?.({
        nodeId: 'image-3',
        state: { status: 'failed', taskId: 'task-3' },
      })
      return makeRunSummary({
        resultNodeIds: ['image-2', 'image-3'],
        totalCount: 2,
        successCount: 1,
        failureCount: 1,
        firstFailure: new Error('output 2 failed'),
        failures: [new Error('output 2 failed')],
      })
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime,
      onSuccess: vi.fn(),
      onWarning: vi.fn(),
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-multi-output',
      outputCount: 2,
      submitTask: vi.fn(async () => ({ taskId: 'unused' })),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    })

    expect(executionRuntime.getNodeState('image-2')).toEqual({
      status: 'succeeded',
      taskId: 'task-2',
    })
    expect(executionRuntime.getNodeState('image-3')).toEqual({
      status: 'failed',
      taskId: 'task-3',
    })
  })

  it('reports pending task summaries with the underlying single-task message', async () => {
    const onSuccess = vi.fn()
    const onWarning = vi.fn()
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2'])
      return makeRunSummary({
        successCount: 0,
        pendingCount: 1,
        firstFailure: new Error('任务进度推送超时，请稍后查看结果'),
      })
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime: createCanvasExecutionRuntime(),
      onSuccess,
      onWarning,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-4',
      outputCount: 1,
      submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    })

    expect(onWarning).toHaveBeenCalledWith('任务进度推送超时，请稍后查看结果')
    expect(onSuccess).not.toHaveBeenCalled()
  })

  it('preserves custom error formatting when the typed error hook declines the error', async () => {
    const onWarning = vi.fn()
    const onError = vi.fn(() => false)
    const pendingResults = createPendingResults(async () => {
      throw new Error('raw error')
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime: createCanvasExecutionRuntime(),
      onSuccess: vi.fn(),
      onWarning,
      onError,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-5',
      outputCount: 1,
      submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
      formatErrorMessage: () => '自定义错误',
    })

    expect(onError).toHaveBeenCalledOnce()
    expect(onWarning).toHaveBeenCalledWith('自定义错误')
  })

  it('routes a typed insufficient failure even when another output succeeds', async () => {
    const insufficient = new CanvasBillingPreflightError('insufficient-credits', 'Idea不足')
    const onError = vi.fn(() => true)
    const onWarning = vi.fn()
    const pendingResults = createPendingResults(async () =>
      makeRunSummary({
        totalCount: 2,
        successCount: 1,
        failureCount: 1,
        firstFailure: insufficient,
        failures: [insufficient],
      }),
    )
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      executionRuntime: createCanvasExecutionRuntime(),
      onSuccess: vi.fn(),
      onWarning,
      onError,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      activationId: 'canvas:activation-6',
      outputCount: 2,
      submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    })

    expect(onError).toHaveBeenCalledWith(insufficient)
    expect(onWarning).not.toHaveBeenCalled()
  })
})
