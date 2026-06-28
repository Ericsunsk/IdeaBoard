import { describe, expect, it, vi } from 'vitest'
import type { ImageAdjustmentPlan } from '../../utils/imageAdjustmentEngine'
import { cloneImageData } from '../../utils/imageDataUtils'
import { executeCpuImageAdjustmentPlanInPlace } from '../cpu/executeCpuImageAdjustmentPlan'
import { handleWorkerImageProcessingRequest } from './imageProcessing.worker'
import {
  createWorkerImageProcessingClient,
  executeWorkerImageAdjustmentPlan,
  WorkerImageProcessingError,
  type WorkerImageProcessingLike,
} from './executeWorkerImageAdjustmentPlan'
import type { WorkerImageProcessingRequest, WorkerImageProcessingResponse } from './protocol'

function makeImageData(values: number[]) {
  class TestImageData {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }
  return new TestImageData(new Uint8ClampedArray(values), values.length / 4, 1) as ImageData
}

function pixelValues(imageData: ImageData) {
  return Array.from(imageData.data)
}

function makeGeneratedImageData(pixelCount: number) {
  const values = new Array<number>(pixelCount * 4)
  for (let pixelIndex = 0; pixelIndex < pixelCount; pixelIndex += 1) {
    const offset = pixelIndex * 4
    values[offset] = (pixelIndex * 17) % 256
    values[offset + 1] = (pixelIndex * 29) % 256
    values[offset + 2] = (pixelIndex * 43) % 256
    values[offset + 3] = 64 + (pixelIndex % 192)
  }
  return makeImageData(values)
}

function createInvertPlan(): ImageAdjustmentPlan {
  return [{ kind: 'channel-lut', lut: createInvertLut() }]
}

function createInvertLut() {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = 255 - index
  }
  return lut
}

async function waitFor(assertion: () => boolean) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    if (assertion()) return
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
  throw new Error('timed out waiting for worker response')
}

describe('worker image adjustment backend', () => {
  it('executes a CPU adjustment plan through the worker protocol', async () => {
    const worker = new FakeImageProcessingWorker()
    const source = makeImageData([10, 20, 30, 77])
    const progress = vi.fn()

    const result = await executeWorkerImageAdjustmentPlan(source, createInvertPlan(), {
      requestId: 'request:success',
      workerFactory: () => worker,
      onProgress: progress,
    })

    expect(pixelValues(result)).toEqual([245, 235, 225, 77])
    expect(pixelValues(source)).toEqual([10, 20, 30, 77])
    expect(progress).toHaveBeenCalledWith({ phase: 'render', loaded: 0, total: 4 })
    expect(progress).toHaveBeenCalledWith({ phase: 'render', loaded: 4, total: 4 })
    expect(progress).toHaveBeenCalledWith({ phase: 'complete' })
    expect(worker.responseTransfers).toHaveLength(1)
    expect(worker.responseTransfers[0]).toEqual([result.data.buffer])
    expect(worker.terminated).toBe(true)
  })

  it('matches the CPU executor when worker rendering spans multiple chunks and steps', async () => {
    const source = makeGeneratedImageData(80_000)
    const plan: ImageAdjustmentPlan = [
      { kind: 'channel-lut', lut: createInvertLut() },
      { kind: 'threshold', threshold: 128, invert: true },
      {
        kind: 'hsl',
        params: {
          hueShift: 120 / 360,
          saturationScale: 0.75,
          lightnessOffset: 0.1,
          contrastFactor: 1.2,
        },
      },
    ]
    const progress = vi.fn()
    const expected = executeCpuImageAdjustmentPlanInPlace(cloneImageData(source), plan)

    const result = await executeWorkerImageAdjustmentPlan(source, plan, {
      requestId: 'request:chunk-parity',
      workerFactory: () => new FakeImageProcessingWorker(),
      onProgress: progress,
    })

    expect(pixelValues(result)).toEqual(pixelValues(expected))
    expect(pixelValues(source)).toEqual(pixelValues(makeGeneratedImageData(80_000)))
    expect(progress).toHaveBeenCalledWith({
      phase: 'render',
      loaded: expect.any(Number),
      total: source.data.length * plan.length,
    })
  })

  it('cancels between worker render chunks and does not send success after cancellation', async () => {
    const responses: WorkerImageProcessingResponse[] = []
    const transfers: Transferable[][] = []
    const requestId = 'request:cancel-during-render'
    const source = makeGeneratedImageData(80_000)
    const plan: ImageAdjustmentPlan = [{ kind: 'channel-lut', lut: createInvertLut() }]

    handleWorkerImageProcessingRequest(
      {
        type: 'execute',
        requestId,
        imageData: source,
        plan,
      },
      (response, transfer = []) => {
        responses.push(response)
        transfers.push(transfer)
        if (
          response.type === 'progress' &&
          response.progress.phase === 'render' &&
          response.progress.loaded !== undefined &&
          response.progress.total !== undefined &&
          response.progress.loaded > 0 &&
          response.progress.loaded < response.progress.total
        ) {
          handleWorkerImageProcessingRequest({ type: 'cancel', requestId }, () => {})
        }
      },
    )

    await waitFor(() => responses.some((response) => response.type === 'error'))

    expect(responses).toContainEqual({
      type: 'error',
      requestId,
      error: {
        code: 'cancelled',
        message: '图像处理已取消',
      },
    })
    expect(responses.some((response) => response.type === 'success')).toBe(false)
    expect(transfers.every((transfer) => transfer.length === 0)).toBe(true)
  })

  it('reuses a worker while the explicit client is alive', async () => {
    const worker = new ManualImageProcessingWorker()
    const workerFactory = vi.fn(() => worker)
    const client = createWorkerImageProcessingClient(workerFactory)

    const first = client.execute({
      imageData: makeImageData([10, 20, 30, 77]),
      plan: [],
      requestId: 'request:first',
    })
    worker.emit({
      type: 'success',
      requestId: 'request:first',
      imageData: makeImageData([1, 2, 3, 4]),
    })

    await expect(first).resolves.toSatisfy((imageData: ImageData) => {
      return pixelValues(imageData).join(',') === '1,2,3,4'
    })
    expect(worker.terminated).toBe(false)

    const second = client.execute({
      imageData: makeImageData([20, 30, 40, 88]),
      plan: [],
      requestId: 'request:second',
    })
    worker.emit({
      type: 'success',
      requestId: 'request:second',
      imageData: makeImageData([5, 6, 7, 8]),
    })

    await expect(second).resolves.toSatisfy((imageData: ImageData) => {
      return pixelValues(imageData).join(',') === '5,6,7,8'
    })
    expect(workerFactory).toHaveBeenCalledTimes(1)
    expect(worker.terminated).toBe(false)

    client.dispose()
    expect(worker.terminated).toBe(true)
  })

  it('transfers only the worker image data buffer and clones reusable LUT payloads', async () => {
    const worker = new ManualImageProcessingWorker()
    const source = makeImageData([10, 20, 30, 77])
    const plan = createInvertPlan()
    const sourceBuffer = source.data.buffer
    const lut = plan[0]?.kind === 'channel-lut' ? plan[0].lut : null
    const promise = executeWorkerImageAdjustmentPlan(source, plan, {
      requestId: 'request:transfer',
      workerFactory: () => worker,
    })

    const executeMessage = worker.messages[0]
    expect(executeMessage?.type).toBe('execute')
    if (executeMessage?.type !== 'execute') throw new Error('expected execute message')

    const transfer = worker.transfers[0] ?? []
    expect(transfer).toEqual([executeMessage.imageData.data.buffer])
    expect(transfer).not.toContain(sourceBuffer)
    expect(transfer).not.toContain(lut?.buffer)
    expect(executeMessage.imageData.data.buffer).not.toBe(sourceBuffer)
    expect(pixelValues(source)).toEqual([10, 20, 30, 77])
    expect(executeMessage.plan[0]?.kind).toBe('channel-lut')
    if (executeMessage.plan[0]?.kind !== 'channel-lut') {
      throw new Error('expected channel LUT step')
    }
    expect(executeMessage.plan[0].lut).not.toBe(lut)
    expect(executeMessage.plan[0].lut.buffer).not.toBe(lut?.buffer)

    worker.emit({
      type: 'success',
      requestId: 'request:transfer',
      imageData: makeImageData([5, 6, 7, 8]),
    })

    await expect(promise).resolves.toSatisfy((imageData: ImageData) => {
      return pixelValues(imageData).join(',') === '5,6,7,8'
    })
    expect(pixelValues(source)).toEqual([10, 20, 30, 77])
  })

  it('keeps caller ImageData readable after a transferred worker request fails', async () => {
    const worker = new ManualImageProcessingWorker()
    const source = makeImageData([10, 20, 30, 77])
    const promise = executeWorkerImageAdjustmentPlan(source, createInvertPlan(), {
      requestId: 'request:transfer-error',
      workerFactory: () => worker,
    })

    const executeMessage = worker.messages[0]
    expect(executeMessage?.type).toBe('execute')
    if (executeMessage?.type !== 'execute') throw new Error('expected execute message')

    expect(worker.transfers[0]).toEqual([executeMessage.imageData.data.buffer])
    expect(worker.transfers[0]).not.toContain(source.data.buffer)
    expect(pixelValues(source)).toEqual([10, 20, 30, 77])

    worker.emit({
      type: 'error',
      requestId: 'request:transfer-error',
      error: {
        code: 'execution-failed',
        message: 'boom',
      },
    })

    await expect(promise).rejects.toBeInstanceOf(WorkerImageProcessingError)
    expect(pixelValues(source)).toEqual([10, 20, 30, 77])
  })

  it('rejects structured worker errors and cleans up the worker', async () => {
    const worker = new ManualImageProcessingWorker()
    const promise = executeWorkerImageAdjustmentPlan(makeImageData([10, 20, 30, 77]), [], {
      requestId: 'request:error',
      workerFactory: () => worker,
    })

    worker.emit({
      type: 'error',
      requestId: 'request:error',
      error: {
        code: 'execution-failed',
        message: 'boom',
      },
    })

    await expect(promise).rejects.toMatchObject({
      name: 'WorkerImageProcessingError',
      code: 'execution-failed',
      message: 'boom',
    })
    expect(worker.terminated).toBe(true)
  })

  it('aborts by sending cancel, rejecting locally, and cleaning up injected workers', async () => {
    const worker = new ManualImageProcessingWorker()
    const controller = new AbortController()

    const promise = executeWorkerImageAdjustmentPlan(makeImageData([10, 20, 30, 77]), [], {
      requestId: 'request:abort',
      workerFactory: () => worker,
      signal: controller.signal,
    })

    controller.abort()

    await expect(promise).rejects.toBeInstanceOf(WorkerImageProcessingError)
    expect(worker.messages).toContainEqual({ type: 'cancel', requestId: 'request:abort' })
    expect(worker.terminated).toBe(true)
  })

  it('does not let an aborted request pollute the next reusable worker request', async () => {
    const worker = new ManualImageProcessingWorker()
    const controller = new AbortController()
    const client = createWorkerImageProcessingClient(() => worker)

    const aborted = client.execute({
      imageData: makeImageData([10, 20, 30, 77]),
      plan: [],
      requestId: 'request:aborted',
      signal: controller.signal,
    })
    controller.abort()

    await expect(aborted).rejects.toBeInstanceOf(WorkerImageProcessingError)
    expect(worker.messages).toContainEqual({ type: 'cancel', requestId: 'request:aborted' })
    expect(worker.terminated).toBe(false)

    worker.emit({
      type: 'success',
      requestId: 'request:aborted',
      imageData: makeImageData([1, 2, 3, 4]),
    })

    const next = client.execute({
      imageData: makeImageData([20, 30, 40, 88]),
      plan: [],
      requestId: 'request:next',
    })
    worker.emit({
      type: 'success',
      requestId: 'request:next',
      imageData: makeImageData([5, 6, 7, 8]),
    })

    await expect(next).resolves.toSatisfy((imageData: ImageData) => {
      return pixelValues(imageData).join(',') === '5,6,7,8'
    })
    expect(worker.terminated).toBe(false)

    client.dispose()
  })

  it('ignores stale responses for other request ids', async () => {
    const worker = new ManualImageProcessingWorker()
    const promise = executeWorkerImageAdjustmentPlan(makeImageData([10, 20, 30, 77]), [], {
      requestId: 'request:current',
      workerFactory: () => worker,
    })

    worker.emit({
      type: 'success',
      requestId: 'request:stale',
      imageData: makeImageData([1, 2, 3, 4]),
    })
    expect(worker.terminated).toBe(false)

    worker.emit({
      type: 'success',
      requestId: 'request:current',
      imageData: makeImageData([5, 6, 7, 8]),
    })

    await expect(promise).resolves.toSatisfy((imageData: ImageData) => {
      return pixelValues(imageData).join(',') === '5,6,7,8'
    })
    expect(worker.terminated).toBe(true)
  })

  it('disposes the reusable worker and rejects pending requests', async () => {
    const worker = new ManualImageProcessingWorker()
    const client = createWorkerImageProcessingClient(() => worker)

    const pending = client.execute({
      imageData: makeImageData([10, 20, 30, 77]),
      plan: [],
      requestId: 'request:pending',
    })

    client.dispose()

    await expect(pending).rejects.toBeInstanceOf(WorkerImageProcessingError)
    expect(worker.terminated).toBe(true)
  })
})

class ManualImageProcessingWorker implements WorkerImageProcessingLike {
  readonly messages: WorkerImageProcessingRequest[] = []
  readonly transfers: Transferable[][] = []
  terminated = false
  private listeners = new Set<(event: MessageEvent<WorkerImageProcessingResponse>) => void>()

  postMessage(message: WorkerImageProcessingRequest, transfer: Transferable[] = []) {
    this.messages.push(message)
    this.transfers.push(transfer)
  }

  addEventListener(
    _type: 'message',
    listener: (event: MessageEvent<WorkerImageProcessingResponse>) => void,
  ) {
    this.listeners.add(listener)
  }

  removeEventListener(
    _type: 'message',
    listener: (event: MessageEvent<WorkerImageProcessingResponse>) => void,
  ) {
    this.listeners.delete(listener)
  }

  terminate() {
    this.terminated = true
  }

  emit(response: WorkerImageProcessingResponse) {
    for (const listener of this.listeners) {
      listener({ data: response } as MessageEvent<WorkerImageProcessingResponse>)
    }
  }
}

class FakeImageProcessingWorker extends ManualImageProcessingWorker {
  readonly responseTransfers: Transferable[][] = []

  override postMessage(message: WorkerImageProcessingRequest, transfer?: Transferable[]) {
    super.postMessage(message, transfer)
    handleWorkerImageProcessingRequest(message, (response, responseTransfer = []) => {
      if (response.type === 'success') {
        this.responseTransfers.push(responseTransfer)
      }
      this.emit(response)
    })
  }
}
