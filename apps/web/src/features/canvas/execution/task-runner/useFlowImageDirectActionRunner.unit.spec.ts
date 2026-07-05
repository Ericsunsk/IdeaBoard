import type { Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { shallowRef, type Ref } from 'vue'
import type { PendingImageResultExecution } from '../result-nodes/usePendingImageResultNodes'
import { useFlowImageDirectActionRunner } from './useFlowImageDirectActionRunner'

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

function createRunSummary(
  overrides: Partial<Awaited<ReturnType<PendingImageResultExecution['runPendingResults']>>> = {},
) {
  return {
    resultNodeIds: ['result-1'],
    totalCount: 1,
    successCount: 1,
    pendingCount: 0,
    failureCount: 0,
    firstFailure: null,
    ...overrides,
  }
}

describe('useFlowImageDirectActionRunner', () => {
  it('runs direct image work through the pending result execution seam', async () => {
    const submitTask = createDeferred<{ taskId: string }>()
    const resolveResult = createDeferred<{ url: string; ratio: number }>()
    const pendingImageResults: PendingImageResultExecution = {
      runPendingResults: vi.fn(async (params) => {
        params.onReserved?.(['result-1'])
        const submission = await params.submitTask('result-1')
        await params.afterTaskIdAttached?.({
          resultNodeId: 'result-1',
          taskId: submission.taskId,
        })
        await params.resolveResult(submission.taskId, 'result-1')
        return createRunSummary()
      }),
      resolveAttachedPendingResult: vi.fn(),
    }
    const flushAutosave = vi.fn(async () => undefined)
    const onSuccess = vi.fn()
    const onWarning = vi.fn()

    const runner = useFlowImageDirectActionRunner({
      nodes: shallowRef([]) as Ref<Node[]>,
      pendingImageResults,
      flushAutosave,
      getCurrentBoardId: () => 'board-a',
      onSuccess,
      onWarning,
    })

    const run = runner.runPendingImageBatchWorkflow({
      source: {
        url: 'https://example.com/source.png',
        ratio: 1,
      },
      sourceNodeId: 'image-1',
      outputCount: 1,
      boardId: 'board-a',
      submitTask: () => submitTask.promise,
      resolveResult: () => resolveResult.promise,
      successMessage: '处理完成',
      fallbackErrorMessage: '执行失败',
    })
    await flushPromises()

    expect(pendingImageResults.runPendingResults).toHaveBeenCalledWith(
      expect.objectContaining({
        source: { url: 'https://example.com/source.png', ratio: 1 },
        sourceNodeId: 'image-1',
        outputCount: 1,
        resultOptions: undefined,
      }),
    )
    expect(runner.isNodeDirectActionRunning('result-1')).toBe(true)

    submitTask.resolve({ taskId: 'task-1' })
    await flushPromises()

    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })

    resolveResult.resolve({ url: 'https://example.com/result.png', ratio: 1 })
    await run

    expect(runner.isNodeDirectActionRunning('result-1')).toBe(false)
    expect(onSuccess).toHaveBeenCalledWith('处理完成')
    expect(onWarning).not.toHaveBeenCalled()
  })

  it('keeps single-image pending timeout copy from the underlying pending error', async () => {
    const pendingError = new Error('任务进度推送超时，请稍后查看结果')
    const pendingImageResults: PendingImageResultExecution = {
      runPendingResults: vi.fn(async (params) => {
        params.onReserved?.(['result-1'])
        return createRunSummary({
          successCount: 0,
          pendingCount: 1,
          firstFailure: pendingError,
        })
      }),
      resolveAttachedPendingResult: vi.fn(),
    }
    const onWarning = vi.fn()

    const runner = useFlowImageDirectActionRunner({
      nodes: shallowRef([]) as Ref<Node[]>,
      pendingImageResults,
      getCurrentBoardId: () => 'board-a',
      onSuccess: vi.fn(),
      onWarning,
    })

    await runner.runPendingImageBatchWorkflow({
      source: {
        url: 'https://example.com/source.png',
        ratio: 1,
      },
      sourceNodeId: 'image-1',
      outputCount: 1,
      boardId: 'board-a',
      submitTask: async () => ({ taskId: 'task-1' }),
      resolveResult: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      fallbackErrorMessage: '执行失败',
    })

    expect(onWarning).toHaveBeenCalledWith('任务进度推送超时，请稍后查看结果')
    expect(runner.isNodeDirectActionRunning('result-1')).toBe(false)
  })
})
