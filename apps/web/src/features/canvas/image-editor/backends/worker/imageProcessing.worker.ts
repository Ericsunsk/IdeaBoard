import {
  createImageAdjustmentPlanStepExecutionState,
  estimateImageAdjustmentPlanStepRangeWorkUnits,
  estimateImageAdjustmentPlanWorkUnits,
  executeImageAdjustmentPlanStepRange,
} from '../../utils/imageAdjustmentEngine'
import type {
  WorkerImageProcessingErrorPayload,
  WorkerImageProcessingExecuteRequest,
  WorkerImageProcessingProgress,
  WorkerImageProcessingRequest,
  WorkerImageProcessingResponse,
} from './protocol'

export type WorkerImageProcessingPostResponse = (
  response: WorkerImageProcessingResponse,
  transfer?: Transferable[],
) => void

type WorkerImageProcessingGlobalScope = {
  readonly document?: unknown
  addEventListener(
    type: 'message',
    listener: (event: MessageEvent<WorkerImageProcessingRequest>) => void,
  ): void
  postMessage(response: WorkerImageProcessingResponse, transfer?: Transferable[]): void
}

const cancelledRequests = new Set<string>()
const WORKER_RENDER_CHUNK_BYTES = 64 * 1024

export function handleWorkerImageProcessingRequest(
  request: WorkerImageProcessingRequest,
  postResponse: WorkerImageProcessingPostResponse,
) {
  if (request.type === 'cancel') {
    cancelledRequests.add(request.requestId)
    return
  }

  void executeRequest(request, postResponse)
}

async function executeRequest(
  request: WorkerImageProcessingExecuteRequest,
  postResponse: WorkerImageProcessingPostResponse,
) {
  if (cancelledRequests.has(request.requestId)) {
    cancelledRequests.delete(request.requestId)
    postResponse({
      type: 'error',
      requestId: request.requestId,
      error: {
        code: 'cancelled',
        message: '图像处理已取消',
      },
    })
    return
  }

  try {
    postResponse({
      type: 'progress',
      requestId: request.requestId,
      progress: createRenderProgress(0, createRenderProgressTotal(request)),
    })

    const imageData = await executeWorkerImageAdjustmentPlanInChunks(request, (progress) => {
      postResponse({
        type: 'progress',
        requestId: request.requestId,
        progress,
      })
    })
    if (cancelledRequests.has(request.requestId)) {
      cancelledRequests.delete(request.requestId)
      postResponse({
        type: 'error',
        requestId: request.requestId,
        error: {
          code: 'cancelled',
          message: '图像处理已取消',
        },
      })
      return
    }

    postResponse({
      type: 'progress',
      requestId: request.requestId,
      progress: { phase: 'complete' },
    })
    postResponse({
      type: 'success',
      requestId: request.requestId,
      imageData,
    }, createWorkerSuccessResponseTransfer(imageData))
  } catch (error) {
    postResponse({
      type: 'error',
      requestId: request.requestId,
      error: toWorkerImageProcessingError(error),
    })
  } finally {
    cancelledRequests.delete(request.requestId)
  }
}

async function executeWorkerImageAdjustmentPlanInChunks(
  request: WorkerImageProcessingExecuteRequest,
  onProgress: (progress: WorkerImageProcessingProgress) => void,
) {
  const output = request.imageData
  const total = createRenderProgressTotal(request)
  let loaded = 0

  if (total === 0) return output

  for (let stepIndex = 0; stepIndex < request.plan.length; stepIndex += 1) {
    const step = request.plan[stepIndex]
    if (!step) continue

    const state = createImageAdjustmentPlanStepExecutionState(output, step)
    for (let startByte = 0; startByte < output.data.length; startByte += WORKER_RENDER_CHUNK_BYTES) {
      throwIfCancelled(request.requestId)

      const endByte = Math.min(startByte + WORKER_RENDER_CHUNK_BYTES, output.data.length)
      executeImageAdjustmentPlanStepRange(output.data, step, startByte, endByte, state)
      loaded += estimateImageAdjustmentPlanStepRangeWorkUnits(output.data, step, startByte, endByte)
      onProgress(createRenderProgress(loaded, total))

      await yieldWorkerEventLoop()
    }
  }

  throwIfCancelled(request.requestId)
  return output
}

function createRenderProgress(loaded: number, total: number): WorkerImageProcessingProgress {
  return {
    phase: 'render',
    loaded,
    total,
  }
}

function createRenderProgressTotal(request: WorkerImageProcessingExecuteRequest) {
  return estimateImageAdjustmentPlanWorkUnits(request.imageData, request.plan)
}

function throwIfCancelled(requestId: string) {
  if (!cancelledRequests.has(requestId)) return
  throw createWorkerCancelledError()
}

function createWorkerCancelledError() {
  return new WorkerImageProcessingCancelledError()
}

class WorkerImageProcessingCancelledError extends Error {
  readonly code = 'cancelled'

  constructor() {
    super('图像处理已取消')
    this.name = 'WorkerImageProcessingCancelledError'
  }
}

function yieldWorkerEventLoop() {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, 0)
  })
}

function createWorkerSuccessResponseTransfer(imageData: ImageData): Transferable[] {
  const imageBuffer = imageData.data.buffer
  return imageBuffer instanceof ArrayBuffer ? [imageBuffer] : []
}

function toWorkerImageProcessingError(error: unknown): WorkerImageProcessingErrorPayload {
  if (isWorkerImageProcessingErrorPayload(error)) return error

  if (error instanceof Error) {
    const code = resolveWorkerImageProcessingErrorCode(error)
    if (code) {
      return {
        code,
        message: error.message,
      }
    }
    return {
      code: 'execution-failed',
      message: error.message,
    }
  }

  return {
    code: 'execution-failed',
    message: 'Worker 图像处理失败',
  }
}

function isWorkerImageProcessingErrorPayload(error: unknown): error is WorkerImageProcessingErrorPayload {
  if (!error || typeof error !== 'object') return false
  if (error instanceof Error) return false
  const payload = error as Partial<WorkerImageProcessingErrorPayload>
  return (
    (payload.code === 'cancelled' ||
      payload.code === 'execution-failed' ||
      payload.code === 'invalid-message') &&
    typeof payload.message === 'string'
  )
}

function resolveWorkerImageProcessingErrorCode(
  error: Error,
): WorkerImageProcessingErrorPayload['code'] | null {
  const code = (error as Partial<WorkerImageProcessingErrorPayload>).code
  if (code === 'cancelled' || code === 'execution-failed' || code === 'invalid-message') {
    return code
  }
  return null
}

const workerGlobalScope = getWorkerImageProcessingGlobalScope()

if (workerGlobalScope) {
  workerGlobalScope.addEventListener('message', (event) => {
    handleWorkerImageProcessingRequest(event.data, (response, transfer) => {
      workerGlobalScope.postMessage(response, transfer)
    })
  })
}

function getWorkerImageProcessingGlobalScope(): WorkerImageProcessingGlobalScope | null {
  const scope = globalThis as Partial<WorkerImageProcessingGlobalScope>
  if (scope.document !== undefined) return null
  if (typeof scope.addEventListener !== 'function') return null
  if (typeof scope.postMessage !== 'function') return null
  return scope as WorkerImageProcessingGlobalScope
}
