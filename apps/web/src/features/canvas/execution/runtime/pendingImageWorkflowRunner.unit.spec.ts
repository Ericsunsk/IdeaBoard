import { describe, expect, it, vi } from 'vitest'
import type {
  PendingImageResultExecution,
  PendingImageResultRunSummary,
  RunPendingImageResultsParams,
} from '../result-nodes/usePendingImageResultNodes'
import { createPendingImageWorkflowRunner } from './pendingImageWorkflowRunner'

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
  it('reuses a client action id for the same pending result submission and rotates it per run', async () => {
    const submittedClientActionIds: string[] = []
    const pendingResults = createPendingResults(async (params) => {
      params.onReserved?.(['image-2'])
      await params.submitTask('image-2')
      await params.submitTask('image-2')
      return makeRunSummary({ successCount: 0 })
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      setNodeRunning: vi.fn(),
      createClientActionId: vi
        .fn()
        .mockReturnValueOnce('client-action-1')
        .mockReturnValueOnce('client-action-2'),
      onSuccess: vi.fn(),
      onWarning: vi.fn(),
    })

    const runParams = {
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      outputCount: 1,
      submitTask: vi.fn(async (_resultNodeId: string, clientActionId?: string) => {
        submittedClientActionIds.push(clientActionId ?? '')
        return { taskId: 'task-1' }
      }),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    }

    await runner.run(runParams)
    await runner.run(runParams)

    expect(submittedClientActionIds).toEqual([
      'client-action-1',
      'client-action-1',
      'client-action-2',
      'client-action-2',
    ])
  })

  it('runs the prepared pending workflow lifecycle and always clears running state', async () => {
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
      await params.afterTaskIdAttached?.({ resultNodeId: 'image-2', taskId: 'task-1' })
      return makeRunSummary({ resultNodeIds: ['image-2'], totalCount: 2, successCount: 2 })
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => true,
      setNodeRunning: (nodeId, running) => {
        events.push(`running:${nodeId}:${running}`)
      },
      afterTaskIdAttached,
      onSuccess,
      onWarning,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
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
      'running:image-2:true',
      'after:image-2:task-1',
      'success:生成完成（2 张）',
      'cleanup',
      'running:image-2:false',
    ])
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('skips summary reporting when the board changes before results finish', async () => {
    let isCurrent = true
    const onSuccess = vi.fn()
    const onWarning = vi.fn()
    const setNodeRunning = vi.fn()
    const pendingResults = createPendingResults(async (params) => {
      expect(params.isCurrent()).toBe(true)
      params.onReserved?.(['image-2'])
      isCurrent = false
      return makeRunSummary()
    })
    const runner = createPendingImageWorkflowRunner({
      pendingResults,
      isBoardCurrent: () => isCurrent,
      setNodeRunning,
      onSuccess,
      onWarning,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      outputCount: 1,
      submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    })

    expect(onSuccess).not.toHaveBeenCalled()
    expect(onWarning).not.toHaveBeenCalled()
    expect(setNodeRunning).toHaveBeenCalledWith('image-2', true)
    expect(setNodeRunning).toHaveBeenCalledWith('image-2', false)
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
      setNodeRunning: vi.fn(),
      onSuccess,
      onWarning,
    })

    await runner.run({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      outputCount: 1,
      submitTask: vi.fn(async () => ({ taskId: 'task-1' })),
      resolveResult: vi.fn(async () => ({ url: 'https://example.com/result.png', ratio: 1 })),
      fallbackErrorMessage: '生成失败',
    })

    expect(onWarning).toHaveBeenCalledWith('任务进度推送超时，请稍后查看结果')
    expect(onSuccess).not.toHaveBeenCalled()
  })
})
