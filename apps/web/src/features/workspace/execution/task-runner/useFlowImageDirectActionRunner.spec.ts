import type { Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { shallowRef, type Ref } from 'vue'
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

describe('useFlowImageDirectActionRunner', () => {
  it('runs a prepared single-image task through pending node lifecycle', async () => {
    const submitTask = createDeferred<{ taskId: string }>()
    const resolveResult = createDeferred<{ url: string; ratio: number }>()
    const pendingImageResultNodes = {
      appendPendingImageResultNode: vi.fn(() => 'result-1'),
      appendPendingImageResultNodes: vi.fn(),
      markPendingImageResultTask: vi.fn(),
      updatePendingImageResultNode: vi.fn(),
      removePendingImageResultNode: vi.fn(),
    }
    const flushAutosave = vi.fn(async () => undefined)
    const onSuccess = vi.fn()
    const onWarning = vi.fn()

    const runner = useFlowImageDirectActionRunner({
      nodes: shallowRef([]) as Ref<Node[]>,
      pendingImageResultNodes,
      flushAutosave,
      getCurrentBoardId: () => 'board-a',
      onSuccess,
      onWarning,
    })

    const run = runner.runPendingImageGeneration({
      source: {
        url: 'https://example.com/source.png',
        ratio: 1,
      },
      sourceNodeId: 'image-1',
      boardId: 'board-a',
      submitTask: () => submitTask.promise,
      resolveResult: () => resolveResult.promise,
      successMessage: '处理完成',
      fallbackErrorMessage: '执行失败',
    })
    await flushPromises()

    expect(pendingImageResultNodes.appendPendingImageResultNode).toHaveBeenCalledWith(
      { url: 'https://example.com/source.png', ratio: 1 },
      'image-1',
      undefined,
    )
    expect(runner.isNodeDirectActionRunning('result-1')).toBe(true)

    submitTask.resolve({ taskId: 'task-1' })
    await flushPromises()

    expect(pendingImageResultNodes.markPendingImageResultTask).toHaveBeenCalledWith(
      'result-1',
      'task-1',
    )
    expect(flushAutosave).toHaveBeenCalledWith({ silent: true })

    resolveResult.resolve({ url: 'https://example.com/result.png', ratio: 1 })
    await run

    expect(pendingImageResultNodes.updatePendingImageResultNode).toHaveBeenCalledWith(
      'result-1',
      { url: 'https://example.com/result.png', ratio: 1 },
    )
    expect(runner.isNodeDirectActionRunning('result-1')).toBe(false)
    expect(onSuccess).toHaveBeenCalledWith('处理完成')
    expect(onWarning).not.toHaveBeenCalled()
  })
})
