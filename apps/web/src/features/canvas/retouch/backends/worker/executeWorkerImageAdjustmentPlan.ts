import type { ImageAdjustmentPlan } from '../../utils/imageAdjustmentEngine'
import { cloneImageData } from '../../utils/imageDataUtils'
import type {
  WorkerImageProcessingErrorPayload,
  WorkerImageProcessingExecuteRequest,
  WorkerImageProcessingProgress,
  WorkerImageProcessingRequest,
  WorkerImageProcessingResponse,
} from './protocol'

export type WorkerImageProcessingLike = {
  postMessage(message: WorkerImageProcessingRequest, transfer?: Transferable[]): void
  addEventListener(
    type: 'message',
    listener: (event: MessageEvent<WorkerImageProcessingResponse>) => void,
  ): void
  removeEventListener(
    type: 'message',
    listener: (event: MessageEvent<WorkerImageProcessingResponse>) => void,
  ): void
  terminate(): void
}

export type WorkerImageProcessingFactory = () => WorkerImageProcessingLike

export type ExecuteWorkerImageAdjustmentPlanOptions = {
  signal?: AbortSignal | undefined
  onProgress?: ((progress: WorkerImageProcessingProgress) => void) | undefined
  requestId?: string | undefined
  workerFactory?: WorkerImageProcessingFactory | undefined
}

type WorkerImageProcessingClientRequest = {
  imageData: ImageData
  plan: ImageAdjustmentPlan
  signal?: AbortSignal | undefined
  onProgress?: ((progress: WorkerImageProcessingProgress) => void) | undefined
  requestId?: string | undefined
}

export type WorkerImageProcessingClient = {
  execute(request: WorkerImageProcessingClientRequest): Promise<ImageData>
  dispose(): void
}

export class WorkerImageProcessingError extends Error {
  readonly code: WorkerImageProcessingErrorPayload['code']

  constructor(payload: WorkerImageProcessingErrorPayload) {
    super(payload.message)
    this.name = 'WorkerImageProcessingError'
    this.code = payload.code
  }
}

export function executeWorkerImageAdjustmentPlan(
  imageData: ImageData,
  plan: ImageAdjustmentPlan,
  options: ExecuteWorkerImageAdjustmentPlanOptions = {},
): Promise<ImageData> {
  const client = options.workerFactory
    ? createWorkerImageProcessingClient(options.workerFactory)
    : getDefaultWorkerImageProcessingClient()

  const executePromise = client.execute({
    imageData,
    plan,
    signal: options.signal,
    onProgress: options.onProgress,
    requestId: options.requestId,
  })

  if (!options.workerFactory) return executePromise

  return executePromise.finally(() => {
    client.dispose()
  })
}

export function createWorkerImageProcessingClient(
  workerFactory: WorkerImageProcessingFactory = createDefaultWorkerImageProcessingWorker,
): WorkerImageProcessingClient {
  let worker: WorkerImageProcessingLike | null = null
  const pendingRequests = new Map<string, PendingWorkerImageProcessingRequest>()

  const handleMessage = (event: MessageEvent<WorkerImageProcessingResponse>) => {
    const response = event.data
    const pending = pendingRequests.get(response.requestId)
    if (!pending) return

    if (response.type === 'progress') {
      pending.onProgress?.(response.progress)
      return
    }

    if (response.type === 'success') {
      settlePendingRequest(pending, () => {
        pending.resolve(response.imageData)
      })
      return
    }

    settlePendingRequest(pending, () => {
      pending.reject(new WorkerImageProcessingError(response.error))
    })
  }

  const ensureWorker = () => {
    if (worker) return worker
    worker = workerFactory()
    worker.addEventListener('message', handleMessage)
    return worker
  }

  const removePendingRequest = (requestId: string) => {
    pendingRequests.delete(requestId)
  }

  return {
    execute(request) {
      const requestId = request.requestId ?? createWorkerImageProcessingRequestId()

      return new Promise((resolve, reject) => {
        if (request.signal?.aborted) {
          reject(createCancelledError())
          return
        }

        const activeWorker = ensureWorker()
        const executePayload = createWorkerExecutePayload(
          requestId,
          request.imageData,
          request.plan,
        )

        const pending: PendingWorkerImageProcessingRequest = {
          requestId,
          signal: request.signal,
          onProgress: request.onProgress,
          resolve,
          reject,
          remove: removePendingRequest,
          settled: false,
        }

        pending.handleAbort = () => {
          activeWorker.postMessage({ type: 'cancel', requestId })
          settlePendingRequest(pending, () => {
            reject(createCancelledError())
          })
        }

        pendingRequests.set(requestId, pending)
        request.signal?.addEventListener('abort', pending.handleAbort, { once: true })

        activeWorker.postMessage(executePayload.message, executePayload.transfer)
      })
    },

    dispose() {
      if (!worker) return

      for (const pending of pendingRequests.values()) {
        settlePendingRequest(pending, () => {
          pending.reject(createCancelledError())
        })
      }
      pendingRequests.clear()

      worker.removeEventListener('message', handleMessage)
      worker.terminate()
      worker = null
    },
  }
}

export function disposeDefaultWorkerImageProcessingClient() {
  defaultWorkerImageProcessingClient?.dispose()
  defaultWorkerImageProcessingClient = null
}

type PendingWorkerImageProcessingRequest = {
  requestId: string
  signal?: AbortSignal | undefined
  onProgress?: ((progress: WorkerImageProcessingProgress) => void) | undefined
  resolve: (imageData: ImageData) => void
  reject: (error: WorkerImageProcessingError) => void
  remove: (requestId: string) => void
  handleAbort?: () => void
  settled: boolean
}

function createDefaultWorkerImageProcessingWorker(): WorkerImageProcessingLike {
  return new Worker(new URL('./imageProcessing.worker.ts', import.meta.url), {
    type: 'module',
  })
}

let defaultWorkerImageProcessingClient: WorkerImageProcessingClient | null = null

function getDefaultWorkerImageProcessingClient() {
  defaultWorkerImageProcessingClient ??= createWorkerImageProcessingClient()
  return defaultWorkerImageProcessingClient
}

function settlePendingRequest(pending: PendingWorkerImageProcessingRequest, callback: () => void) {
  if (pending.settled) return
  pending.settled = true
  if (pending.handleAbort) {
    pending.signal?.removeEventListener('abort', pending.handleAbort)
  }
  pending.remove(pending.requestId)
  callback()
}

function createWorkerExecutePayload(
  requestId: string,
  imageData: ImageData,
  plan: ImageAdjustmentPlan,
): { message: WorkerImageProcessingExecuteRequest; transfer: Transferable[] } {
  const workerImageData = cloneImageData(imageData)
  const imageBuffer = workerImageData.data.buffer
  return {
    message: {
      type: 'execute',
      requestId,
      imageData: workerImageData,
      plan: cloneImageAdjustmentPlanForWorker(plan),
    },
    transfer: imageBuffer instanceof ArrayBuffer ? [imageBuffer] : [],
  }
}

function cloneImageAdjustmentPlanForWorker(plan: ImageAdjustmentPlan): ImageAdjustmentPlan {
  return plan.map((step) => {
    switch (step.kind) {
      case 'hsl':
        return {
          kind: 'hsl',
          params: { ...step.params },
        }
      case 'threshold':
        return {
          kind: 'threshold',
          threshold: step.threshold,
          invert: step.invert,
        }
      case 'channel-lut':
        return {
          kind: 'channel-lut',
          luts: {
            red: new Uint8ClampedArray(step.luts.red),
            green: new Uint8ClampedArray(step.luts.green),
            blue: new Uint8ClampedArray(step.luts.blue),
          },
        }
      case 'sharpen':
        return {
          kind: 'sharpen',
          params: { ...step.params },
        }
    }
  })
}

let nextWorkerImageProcessingRequestIndex = 0

function createWorkerImageProcessingRequestId() {
  nextWorkerImageProcessingRequestIndex += 1
  return `image-processing:${nextWorkerImageProcessingRequestIndex}`
}

function createCancelledError() {
  return new WorkerImageProcessingError({
    code: 'cancelled',
    message: 'Image processing cancelled',
  })
}
