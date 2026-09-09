import type { ImageRetouchPlan } from '../utils/engine'

export type ImageProcessingErrorCode =
  'backend-failed' | 'cancelled' | 'encode-failed' | 'invalid-source' | 'stale' | 'unknown'

export type ImageProcessingError = {
  code: ImageProcessingErrorCode
  message: string
  cause?: unknown
}

export type ImageProcessingSource = {
  url: string
  title?: string
}

export type ImageProcessingPlan = ImageRetouchPlan

export type ImageProcessingProgress = {
  phase: 'queued' | 'decode' | 'render' | 'encode' | 'complete'
  loaded?: number
  total?: number
}

export type ImageProcessingOutputEncoding = {
  mimeType: string
  quality?: number
}

export type ImageProcessingOutputPolicyInput = {
  sourceMimeType: string
  hasTransparentPixels: boolean | null
}

export type ImageProcessingOutputFileNameInput = {
  title?: string | undefined
  mimeType: string
  suffix?: string | undefined
  fallbackBaseName?: string | undefined
}

export type ImageProcessingOutputPolicy = {
  resolveEncoding(input: ImageProcessingOutputPolicyInput): ImageProcessingOutputEncoding
  resolveFileName(input: ImageProcessingOutputFileNameInput): string
}

export type RenderFinalInput = {
  source: ImageProcessingSource
  plan: ImageProcessingPlan
  taskId?: string | undefined
  latestOnly?: boolean | undefined
  maxPixels?: number | undefined
  maxSide?: number | undefined
  fitWithinMaxPixels?: boolean | undefined
  preserveSourceMimeType?: boolean | undefined
  onProgress?: ((progress: ImageProcessingProgress) => void) | undefined
}

export type ImageProcessingPreviewSizeStrategy = {
  maxSide: number
}

export type RenderPreviewInput = {
  source: ImageProcessingSource
  plan: ImageProcessingPlan
  taskId?: string | undefined
  latestOnly?: boolean | undefined
  sizeStrategy?: ImageProcessingPreviewSizeStrategy | undefined
  onProgress?: ((progress: ImageProcessingProgress) => void) | undefined
}

export type RenderPreviewOutput = {
  sourceImageData: ImageData
  previewImageData: ImageData
  width: number
  height: number
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

export type RenderFinalResult = ImageProcessingTaskResult<RenderFinalOutput>
export type RenderPreviewResult = ImageProcessingTaskResult<RenderPreviewOutput>

export type ImageProcessingBackendTaskContext = {
  taskId: string
  revision: number
  signal: AbortSignal
  outputPolicy: ImageProcessingOutputPolicy
  reportProgress: (progress: ImageProcessingProgress) => void
}

export type ImageProcessingBackendFinalInput = Omit<
  RenderFinalInput,
  'latestOnly' | 'onProgress' | 'taskId'
> & {
  context: ImageProcessingBackendTaskContext
}

export type ImageProcessingBackendPreviewInput = Omit<
  RenderPreviewInput,
  'latestOnly' | 'onProgress' | 'taskId'
> & {
  context: ImageProcessingBackendTaskContext
}

export type ImageProcessingBackend = {
  renderFinal(input: ImageProcessingBackendFinalInput): Promise<RenderFinalOutput>
  renderPreview(input: ImageProcessingBackendPreviewInput): Promise<RenderPreviewOutput>
  cancel?: ((taskId?: string) => void) | undefined
  dispose?: (() => void) | undefined
}

export type ImageProcessingRuntime = {
  renderFinal(input: RenderFinalInput): Promise<RenderFinalResult>
  renderPreview(input: RenderPreviewInput): Promise<RenderPreviewResult>
  cancel(taskId?: string): void
  dispose(): void
}
