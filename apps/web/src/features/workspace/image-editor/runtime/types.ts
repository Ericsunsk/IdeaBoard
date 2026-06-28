import type { PreviewMimeType } from '../../utils/displayImagePreviewEncoding'
import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'

export type ImageProcessingTaskKind = 'preview' | 'final'

export type ImageProcessingTaskStatus = 'completed' | 'stale' | 'cancelled' | 'failed'

export type ImageProcessingErrorCode =
  | 'backend-failed'
  | 'cancelled'
  | 'encode-failed'
  | 'invalid-source'
  | 'stale'
  | 'unknown'

export type ImageProcessingError = {
  code: ImageProcessingErrorCode
  message: string
  cause?: unknown
}

export type ImageProcessingSource = {
  url: string
  title?: string
  mimeType?: string
  width?: number
  height?: number
  cacheKey?: string
}

export type ImageProcessingPlan = ImageAdjustmentPlan

export type ImageProcessingProgress = {
  phase: 'queued' | 'decode' | 'render' | 'encode' | 'complete'
  loaded?: number
  total?: number
}

export type ImageProcessingOutputEncoding = {
  mimeType: PreviewMimeType
  quality?: number
}

export type ImageProcessingOutputPolicyInput = {
  kind: ImageProcessingTaskKind
  sourceMimeType: string
  hasTransparentPixels: boolean | null
}

export type ImageProcessingOutputFileNameInput = {
  title?: string
  mimeType: string
  suffix?: string
  fallbackBaseName?: string
}

export type ImageProcessingOutputPolicy = {
  resolveEncoding(input: ImageProcessingOutputPolicyInput): ImageProcessingOutputEncoding
  resolveFileName(input: ImageProcessingOutputFileNameInput): string
}

export type RenderPreviewInput = {
  source: ImageProcessingSource
  plan: ImageProcessingPlan
  taskId?: string
  debounceMs?: number
  maxSide?: number
  onProgress?: (progress: ImageProcessingProgress) => void
}

export type RenderFinalInput = {
  source: ImageProcessingSource
  plan: ImageProcessingPlan
  taskId?: string
  latestOnly?: boolean
  onProgress?: (progress: ImageProcessingProgress) => void
}

export type RenderPreviewOutput = {
  objectUrl: string | null
  width?: number
  height?: number
  mimeType?: string
}

export type RenderFinalOutput = {
  file: File
  width?: number
  height?: number
  mimeType?: string
}

export type ImageProcessingCompletedResult<TOutput> = {
  status: 'completed'
  taskId: string
  revision: number
  output: TOutput
}

export type ImageProcessingSkippedResult = {
  status: 'cancelled' | 'stale'
  taskId: string
  revision: number
  error: ImageProcessingError
}

export type ImageProcessingFailedResult = {
  status: 'failed'
  taskId: string
  revision: number
  error: ImageProcessingError
}

export type ImageProcessingTaskResult<TOutput> =
  | ImageProcessingCompletedResult<TOutput>
  | ImageProcessingSkippedResult
  | ImageProcessingFailedResult

export type RenderPreviewResult = ImageProcessingTaskResult<RenderPreviewOutput>

export type RenderFinalResult = ImageProcessingTaskResult<RenderFinalOutput>

export type ImageProcessingBackendTaskContext = {
  taskId: string
  revision: number
  signal: AbortSignal
  outputPolicy: ImageProcessingOutputPolicy
  reportProgress: (progress: ImageProcessingProgress) => void
}

export type ImageProcessingBackendPreviewInput = Omit<
  RenderPreviewInput,
  'debounceMs' | 'onProgress' | 'taskId'
> & {
  context: ImageProcessingBackendTaskContext
}

export type ImageProcessingBackendFinalInput = Omit<
  RenderFinalInput,
  'latestOnly' | 'onProgress' | 'taskId'
> & {
  context: ImageProcessingBackendTaskContext
}

export type ImageProcessingBackend = {
  renderPreview(input: ImageProcessingBackendPreviewInput): Promise<RenderPreviewOutput>
  renderFinal(input: ImageProcessingBackendFinalInput): Promise<RenderFinalOutput>
  cancel?(taskId?: string): void
  dispose?(): void
}

export type ImageProcessingRuntime = {
  renderPreview(input: RenderPreviewInput): Promise<RenderPreviewResult>
  renderFinal(input: RenderFinalInput): Promise<RenderFinalResult>
  cancel(taskId?: string): void
  dispose(): void
}
