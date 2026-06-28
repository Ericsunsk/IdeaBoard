import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import { createCpuImageAdjustmentPlanExecutor } from './cpuPlanExecutor'
import { createHybridImageAdjustmentPlanExecutor } from './hybridPlanExecutor'
import { createWorkerImageAdjustmentPlanExecutor } from './workerPlanExecutor'
import type { ImageAdjustmentPlanExecutionEvent } from './planExecutorTypes'

const { disposeDefaultWorkerImageProcessingClientMock, executeWorkerImageAdjustmentPlanMock } =
  vi.hoisted(() => ({
    disposeDefaultWorkerImageProcessingClientMock: vi.fn(),
    executeWorkerImageAdjustmentPlanMock: vi.fn(),
  }))

vi.mock('../backends/worker/executeWorkerImageAdjustmentPlan', () => ({
  disposeDefaultWorkerImageProcessingClient: disposeDefaultWorkerImageProcessingClientMock,
  executeWorkerImageAdjustmentPlan: executeWorkerImageAdjustmentPlanMock,
}))

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  executeWorkerImageAdjustmentPlanMock.mockReset()
  disposeDefaultWorkerImageProcessingClientMock.mockReset()
})

describe('image adjustment plan executor', () => {
  it('runs the CPU executor and reports completion diagnostics', async () => {
    const source = makeImageData([10, 20, 30, 255])
    const onPlanExecutorEvent = vi.fn<(event: ImageAdjustmentPlanExecutionEvent) => void>()
    const executor = createCpuImageAdjustmentPlanExecutor({ onPlanExecutorEvent })

    const result = await executor.execute({
      imageData: source,
      plan: createInvertPlan(),
      signal: new AbortController().signal,
    })

    expect(result).toBe(source)
    expect(pixelValues(result)).toEqual([245, 235, 225, 255])
    expect(onPlanExecutorEvent).toHaveBeenCalledWith({
      kind: 'cpu-plan-completed',
      planLength: 1,
      width: 1,
      height: 1,
    })
  })

  it('uses the worker executor and reports success diagnostics', async () => {
    const source = makeImageData([10, 20, 30, 255])
    const workerOutput = makeImageData([11, 22, 33, 255])
    const signal = new AbortController().signal
    const onProgress = vi.fn()
    const onPlanExecutorEvent = vi.fn<(event: ImageAdjustmentPlanExecutionEvent) => void>()
    executeWorkerImageAdjustmentPlanMock.mockResolvedValue(workerOutput)

    const executor = createWorkerImageAdjustmentPlanExecutor({ onPlanExecutorEvent })
    const result = await executor.execute({
      imageData: source,
      plan: createInvertPlan(),
      signal,
      onProgress,
    })

    expect(result).toBe(workerOutput)
    expect(executeWorkerImageAdjustmentPlanMock).toHaveBeenCalledWith(source, createInvertPlan(), {
      signal,
      onProgress,
    })
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(1, {
      kind: 'worker-plan-started',
      planLength: 1,
      width: 1,
      height: 1,
    })
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(2, {
      kind: 'worker-plan-completed',
      planLength: 1,
      width: 1,
      height: 1,
    })
  })

  it('uses the sync CPU executor for hybrid plans below the work threshold', async () => {
    const source = makeImageData([10, 20, 30, 255])
    const onPlanExecutorEvent = vi.fn<(event: ImageAdjustmentPlanExecutionEvent) => void>()
    const executor = createHybridImageAdjustmentPlanExecutor({
      syncMaxWorkBytes: 4,
      onPlanExecutorEvent,
    })

    const result = await executor.execute({
      imageData: source,
      plan: createInvertPlan(),
      signal: new AbortController().signal,
    })

    expect(executeWorkerImageAdjustmentPlanMock).not.toHaveBeenCalled()
    expect(result).toBe(source)
    expect(pixelValues(result)).toEqual([245, 235, 225, 255])
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(1, {
      kind: 'hybrid-plan-selected',
      backend: 'cpu-main',
      executionMode: 'sync',
      reason: 'work-threshold',
      planLength: 1,
      width: 1,
      height: 1,
      pixelCount: 1,
      workBytes: 4,
      syncThresholdWorkBytes: 4,
    })
  })

  it('uses cooperative CPU execution for hybrid plans above the work threshold', async () => {
    const source = makeImageData([10, 20, 30, 255, 40, 50, 60, 255])
    const signal = new AbortController().signal
    const onProgress = vi.fn()
    const onPlanExecutorEvent = vi.fn<(event: ImageAdjustmentPlanExecutionEvent) => void>()
    const yieldToMainThread = vi.fn(async () => {})

    const executor = createHybridImageAdjustmentPlanExecutor({
      cooperativeChunkBytes: 4,
      cooperativeFrameBudgetMs: 0,
      syncMaxWorkBytes: 0,
      onPlanExecutorEvent,
      now: () => 1,
      yieldToMainThread,
    })
    const result = await executor.execute({
      imageData: source,
      plan: createInvertPlan(),
      signal,
      onProgress,
    })

    expect(executeWorkerImageAdjustmentPlanMock).not.toHaveBeenCalled()
    expect(result).not.toBe(source)
    expect(pixelValues(result)).toEqual([245, 235, 225, 255, 215, 205, 195, 255])
    expect(pixelValues(source)).toEqual([10, 20, 30, 255, 40, 50, 60, 255])
    expect(yieldToMainThread).toHaveBeenCalledTimes(2)
    expect(onProgress).toHaveBeenCalledWith({ phase: 'render', loaded: 0, total: 8 })
    expect(onProgress).toHaveBeenCalledWith({ phase: 'render', loaded: 4, total: 8 })
    expect(onProgress).toHaveBeenCalledWith({ phase: 'render', loaded: 8, total: 8 })
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(1, {
      kind: 'hybrid-plan-selected',
      backend: 'cpu-main',
      executionMode: 'cooperative',
      reason: 'work-threshold',
      planLength: 1,
      width: 2,
      height: 1,
      pixelCount: 2,
      workBytes: 8,
      syncThresholdWorkBytes: 0,
    })
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(2, {
      kind: 'cpu-plan-completed',
      planLength: 1,
      width: 2,
      height: 1,
    })
  })

  it('falls back to the worker if cooperative CPU execution fails without cancellation', async () => {
    const source = makeImageData([10, 20, 30, 255, 40, 50, 60, 255])
    const workerOutput = makeImageData([11, 22, 33, 255, 44, 55, 66, 255])
    const signal = new AbortController().signal
    executeWorkerImageAdjustmentPlanMock.mockResolvedValue(workerOutput)

    const executor = createHybridImageAdjustmentPlanExecutor({
      cooperativeChunkBytes: 4,
      cooperativeFrameBudgetMs: 0,
      syncMaxWorkBytes: 0,
      now: () => 1,
      yieldToMainThread: () => Promise.reject(new Error('main thread failed')),
    })
    const result = await executor.execute({
      imageData: source,
      plan: createInvertPlan(),
      signal,
    })

    expect(result).toBe(workerOutput)
    expect(pixelValues(source)).toEqual([10, 20, 30, 255, 40, 50, 60, 255])
    expect(executeWorkerImageAdjustmentPlanMock).toHaveBeenCalledWith(source, createInvertPlan(), {
      signal,
      onProgress: undefined,
    })
  })

  it('does not fall back to the worker when cooperative CPU execution is cancelled', async () => {
    const controller = new AbortController()
    const source = makeImageData([10, 20, 30, 255, 40, 50, 60, 255])

    const executor = createHybridImageAdjustmentPlanExecutor({
      cooperativeChunkBytes: 4,
      cooperativeFrameBudgetMs: 0,
      syncMaxWorkBytes: 0,
      now: () => 1,
      yieldToMainThread: async () => {
        controller.abort()
      },
    })

    await expect(
      executor.execute({
        imageData: source,
        plan: createInvertPlan(),
        signal: controller.signal,
      }),
    ).rejects.toThrow('图像处理已取消')

    expect(pixelValues(source)).toEqual([10, 20, 30, 255, 40, 50, 60, 255])
    expect(executeWorkerImageAdjustmentPlanMock).not.toHaveBeenCalled()
  })

  it('uses the sync CPU executor for empty hybrid plans', async () => {
    const source = makeImageData([10, 20, 30, 255])
    const onPlanExecutorEvent = vi.fn<(event: ImageAdjustmentPlanExecutionEvent) => void>()
    const executor = createHybridImageAdjustmentPlanExecutor({
      syncMaxWorkBytes: 0,
      onPlanExecutorEvent,
    })

    const result = await executor.execute({
      imageData: source,
      plan: [],
      signal: new AbortController().signal,
    })

    expect(result).toBe(source)
    expect(executeWorkerImageAdjustmentPlanMock).not.toHaveBeenCalled()
    expect(pixelValues(result)).toEqual([10, 20, 30, 255])
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(1, {
      kind: 'hybrid-plan-selected',
      backend: 'cpu-main',
      executionMode: 'sync',
      reason: 'empty-plan',
      planLength: 0,
      width: 1,
      height: 1,
      pixelCount: 1,
      workBytes: 0,
      syncThresholdWorkBytes: 0,
    })
  })

  it('falls back to the CPU executor when worker execution fails without cancellation', async () => {
    const source = makeImageData([10, 20, 30, 255])
    const onPlanExecutorEvent = vi.fn<(event: ImageAdjustmentPlanExecutionEvent) => void>()
    executeWorkerImageAdjustmentPlanMock.mockRejectedValue(new Error('worker failed'))

    const executor = createWorkerImageAdjustmentPlanExecutor({ onPlanExecutorEvent })
    const result = await executor.execute({
      imageData: source,
      plan: createInvertPlan(),
      signal: new AbortController().signal,
    })

    expect(result).toBe(source)
    expect(pixelValues(result)).toEqual([245, 235, 225, 255])
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(1, {
      kind: 'worker-plan-started',
      planLength: 1,
      width: 1,
      height: 1,
    })
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(2, {
      kind: 'worker-plan-fallback',
      planLength: 1,
      width: 1,
      height: 1,
      errorName: 'Error',
      errorMessage: 'worker failed',
    })
    expect(onPlanExecutorEvent).toHaveBeenNthCalledWith(3, {
      kind: 'cpu-plan-completed',
      planLength: 1,
      width: 1,
      height: 1,
    })
  })

  it('does not fall back to CPU when worker execution fails after cancellation', async () => {
    const controller = new AbortController()
    const source = makeImageData([10, 20, 30, 255])
    const onPlanExecutorEvent = vi.fn<(event: ImageAdjustmentPlanExecutionEvent) => void>()
    executeWorkerImageAdjustmentPlanMock.mockImplementation(() => {
      controller.abort()
      return Promise.reject(new Error('cancelled'))
    })

    const executor = createWorkerImageAdjustmentPlanExecutor({ onPlanExecutorEvent })
    await expect(
      executor.execute({
        imageData: source,
        plan: createInvertPlan(),
        signal: controller.signal,
      }),
    ).rejects.toThrow('cancelled')

    expect(pixelValues(source)).toEqual([10, 20, 30, 255])
    expect(onPlanExecutorEvent).toHaveBeenCalledOnce()
    expect(onPlanExecutorEvent).toHaveBeenCalledWith({
      kind: 'worker-plan-started',
      planLength: 1,
      width: 1,
      height: 1,
    })
  })
})

function makeImageData(values: number[]): ImageData {
  class TestImageDataImpl {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }
  return new TestImageDataImpl(new Uint8ClampedArray(values), values.length / 4, 1) as ImageData
}

function pixelValues(imageData: ImageData) {
  return Array.from(imageData.data)
}

function createInvertPlan(): ImageAdjustmentPlan {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = 255 - index
  }
  return [{ kind: 'channel-lut', luts: createRgbChannelLutsForTest(lut) }]
}

function createRgbChannelLutsForTest(lut: Uint8ClampedArray) {
  return {
    red: lut,
    green: lut,
    blue: lut,
  }
}
