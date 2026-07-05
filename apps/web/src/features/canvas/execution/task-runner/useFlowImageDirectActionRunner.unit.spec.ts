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

function makePendingNode(nodeId: string, taskId: string): Node {
  return {
    id: nodeId,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'image',
      snapshotNode: {
        id: Number(nodeId.replace(/\D/g, '')) || 1,
        type: 'image-node',
        x: 0,
        y: 0,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'running',
        outputImages: [],
        pendingTaskId: taskId,
      },
    },
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
        await params.afterTaskIdsAttached?.({
          attachments: [
            {
              resultNodeId: 'result-1',
              taskId: submission.taskId,
            },
          ],
        })
        await params.resolveResult(submission.taskId, 'result-1')
        return createRunSummary()
      }),
      resolveAttachedPendingResult: vi.fn(),
      resolveAttachedPendingResults: vi.fn(),
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

  it('flushes attached pending task ids once for a batch', async () => {
    const pendingImageResults: PendingImageResultExecution = {
      runPendingResults: vi.fn(async (params) => {
        params.onReserved?.(['result-1', 'result-2'])
        await params.afterTaskIdsAttached?.({
          attachments: [
            { resultNodeId: 'result-1', taskId: 'task-1' },
            { resultNodeId: 'result-2', taskId: 'task-2' },
          ],
        })
        return createRunSummary({
          resultNodeIds: ['result-1', 'result-2'],
          totalCount: 2,
          successCount: 2,
        })
      }),
      resolveAttachedPendingResult: vi.fn(),
      resolveAttachedPendingResults: vi.fn(),
    }
    const flushAutosave = vi.fn(async () => undefined)
    const runner = useFlowImageDirectActionRunner({
      nodes: shallowRef([]) as Ref<Node[]>,
      pendingImageResults,
      flushAutosave,
      getCurrentBoardId: () => 'board-a',
      onSuccess: vi.fn(),
      onWarning: vi.fn(),
    })

    await runner.runPendingImageBatchWorkflow({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'image-1',
      outputCount: 2,
      boardId: 'board-a',
      submitTask: async () => ({ taskId: 'unused' }),
      resolveResult: async () => ({ url: 'https://example.com/result.png', ratio: 1 }),
      fallbackErrorMessage: '执行失败',
    })

    expect(flushAutosave).toHaveBeenCalledOnce()
    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
  })

  it('reattaches pending task results through one batch seam', async () => {
    const pendingImageResults: PendingImageResultExecution = {
      runPendingResults: vi.fn(),
      resolveAttachedPendingResult: vi.fn(),
      resolveAttachedPendingResults: vi.fn(async (params) => {
        expect(params.tasks).toEqual([
          { resultNodeId: 'image-1', taskId: 'task-1' },
          { resultNodeId: 'image-2', taskId: 'task-2' },
        ])
        return {
          successCount: 2,
          failureCount: 0,
          firstFailure: null,
        }
      }),
    }
    const flushAutosave = vi.fn(async () => undefined)
    const onSuccess = vi.fn()
    const runner = useFlowImageDirectActionRunner({
      nodes: shallowRef([
        makePendingNode('image-1', 'task-1'),
        makePendingNode('image-2', 'task-2'),
      ]) as Ref<Node[]>,
      pendingImageResults,
      flushAutosave,
      getCurrentBoardId: () => 'board-a',
      onSuccess,
      onWarning: vi.fn(),
    })

    await runner.reattachPendingImageResults()

    expect(pendingImageResults.resolveAttachedPendingResults).toHaveBeenCalledOnce()
    expect(pendingImageResults.resolveAttachedPendingResult).not.toHaveBeenCalled()
    expect(flushAutosave).toHaveBeenCalledOnce()
    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })
    expect(onSuccess).toHaveBeenCalledWith('已恢复 2 个生成结果')
    expect(runner.isNodeDirectActionRunning('image-1')).toBe(false)
    expect(runner.isNodeDirectActionRunning('image-2')).toBe(false)
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
      resolveAttachedPendingResults: vi.fn(),
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
