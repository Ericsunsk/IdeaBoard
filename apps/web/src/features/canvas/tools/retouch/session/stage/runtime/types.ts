import type { ImageRetouchPlan } from '../../../core/engine.ts'

/**
 * Rendered preview sampled from the DOM source (pre-GPU effects) for the
 * CPU histogram chain. Canonical home for this type (moved here from the
 * former flat model module during the session/stage migration).
 */
export type RetouchRenderedPreview = {
  type: 'rendered-image'
  imageData: ImageData
  width: number
  height: number
}

export type ImageProcessingErrorCode =
  | 'animated-image'
  | 'backend-failed'
  | 'cancelled'
  | 'decode-failed'
  | 'disposed'
  | 'empty-plan'
  | 'encode-failed'
  | 'fetch-failed'
  | 'invalid-source'
  | 'invalid-url'
  | 'not-image'
  | 'stale'
  | 'too-large'
  | 'unknown'
  | 'unsupported-canvas'

export type ImageProcessingError = {
  code: ImageProcessingErrorCode
  message: string
  cause?: unknown
}

/**
 * Structured processing failure: code is the fact, message carries the code
 * (never localized copy). Localized copy resolves at the commands/Chrome
 * layer via the existing canvas.* keys, so core/session/stage/runtime stay
 * locale-free and locale switches cannot strand baked strings.
 */
export class ImageProcessingErrorRecord extends Error {
  readonly code: ImageProcessingErrorCode

  constructor(code: ImageProcessingErrorCode, options?: { cause?: unknown }) {
    super(code)
    this.name = 'ImageProcessingError'
    this.code = code
    if (options?.cause !== undefined) {
      ;(this as { cause?: unknown }).cause = options.cause
    }
  }
}

export function isImageProcessingError(error: unknown): error is ImageProcessingError {
  if (!error || typeof error !== 'object') return false
  const candidate = error as Partial<ImageProcessingError>
  return isImageProcessingErrorCode(candidate.code) && typeof candidate.message === 'string'
}

function isImageProcessingErrorCode(code: unknown): code is ImageProcessingErrorCode {
  return (
    code === 'animated-image' ||
    code === 'backend-failed' ||
    code === 'cancelled' ||
    code === 'decode-failed' ||
    code === 'disposed' ||
    code === 'empty-plan' ||
    code === 'encode-failed' ||
    code === 'fetch-failed' ||
    code === 'invalid-source' ||
    code === 'invalid-url' ||
    code === 'not-image' ||
    code === 'stale' ||
    code === 'too-large' ||
    code === 'unknown' ||
    code === 'unsupported-canvas'
  )
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
