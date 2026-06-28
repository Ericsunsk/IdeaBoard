import type { ImageAdjustmentPlan } from '../../utils/imageAdjustmentEngine'

export type WorkerImageProcessingRequestId = string

export type WorkerImageProcessingProgress = {
  phase: 'queued' | 'render' | 'complete'
  loaded?: number
  total?: number
}

export type WorkerImageProcessingExecuteRequest = {
  type: 'execute'
  requestId: WorkerImageProcessingRequestId
  imageData: ImageData
  plan: ImageAdjustmentPlan
}

export type WorkerImageProcessingCancelRequest = {
  type: 'cancel'
  requestId: WorkerImageProcessingRequestId
}

export type WorkerImageProcessingRequest =
  | WorkerImageProcessingExecuteRequest
  | WorkerImageProcessingCancelRequest

export type WorkerImageProcessingSuccessResponse = {
  type: 'success'
  requestId: WorkerImageProcessingRequestId
  imageData: ImageData
}

export type WorkerImageProcessingErrorResponse = {
  type: 'error'
  requestId: WorkerImageProcessingRequestId
  error: WorkerImageProcessingErrorPayload
}

export type WorkerImageProcessingProgressResponse = {
  type: 'progress'
  requestId: WorkerImageProcessingRequestId
  progress: WorkerImageProcessingProgress
}

export type WorkerImageProcessingResponse =
  | WorkerImageProcessingSuccessResponse
  | WorkerImageProcessingErrorResponse
  | WorkerImageProcessingProgressResponse

export type WorkerImageProcessingErrorCode = 'cancelled' | 'execution-failed' | 'invalid-message'

export type WorkerImageProcessingErrorPayload = {
  code: WorkerImageProcessingErrorCode
  message: string
}
