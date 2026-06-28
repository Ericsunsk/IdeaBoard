import {
  ImageProcessingScheduler,
  type ImageProcessingSchedulerResult,
} from './imageProcessingScheduler'
import { createImageProcessingOutputPolicy } from './imageOutputPolicy'
import type {
  ImageProcessingBackend,
  ImageProcessingError,
  ImageProcessingOutputPolicy,
  ImageProcessingRuntime,
  ImageProcessingTaskResult,
  RenderFinalInput,
  RenderFinalOutput,
  RenderFinalResult,
  RenderPreviewInput,
  RenderPreviewOutput,
  RenderPreviewResult,
} from './types'

export type CreateImageProcessingRuntimeOptions = {
  backend: ImageProcessingBackend
  outputPolicy?: ImageProcessingOutputPolicy
  previewDebounceMs?: number
  createTaskId?: (revision: number) => string
}

export class MainThreadImageProcessingRuntime implements ImageProcessingRuntime {
  private readonly backend: ImageProcessingBackend
  private readonly outputPolicy: ImageProcessingOutputPolicy
  private readonly previewScheduler: ImageProcessingScheduler
  private readonly finalScheduler: ImageProcessingScheduler

  constructor(options: CreateImageProcessingRuntimeOptions) {
    this.backend = options.backend
    this.outputPolicy = options.outputPolicy ?? createImageProcessingOutputPolicy()
    this.previewScheduler = new ImageProcessingScheduler({
      debounceMs: options.previewDebounceMs ?? 0,
      latestOnly: true,
      createTaskId: options.createTaskId,
    })
    this.finalScheduler = new ImageProcessingScheduler({
      latestOnly: false,
      createTaskId: options.createTaskId,
    })
  }

  async renderPreview(input: RenderPreviewInput): Promise<RenderPreviewResult> {
    const result = await this.previewScheduler.schedule(
      (context) =>
        this.backend.renderPreview({
          source: input.source,
          plan: input.plan,
          maxSide: input.maxSide,
          context: {
            taskId: context.taskId,
            revision: context.revision,
            signal: context.signal,
            outputPolicy: this.outputPolicy,
            reportProgress: input.onProgress ?? ignoreProgress,
          },
        }),
      {
        taskId: input.taskId,
        debounceMs: input.debounceMs,
        latestOnly: true,
      },
    )
    return mapSchedulerResult(result)
  }

  async renderFinal(input: RenderFinalInput): Promise<RenderFinalResult> {
    const result = await this.finalScheduler.schedule(
      (context) =>
        this.backend.renderFinal({
          source: input.source,
          plan: input.plan,
          context: {
            taskId: context.taskId,
            revision: context.revision,
            signal: context.signal,
            outputPolicy: this.outputPolicy,
            reportProgress: input.onProgress ?? ignoreProgress,
          },
        }),
      {
        taskId: input.taskId,
        latestOnly: input.latestOnly ?? false,
      },
    )
    return mapSchedulerResult(result)
  }

  cancel(taskId?: string) {
    this.previewScheduler.cancel(taskId)
    this.finalScheduler.cancel(taskId)
    this.backend.cancel?.(taskId)
  }

  dispose() {
    this.previewScheduler.dispose()
    this.finalScheduler.dispose()
    this.backend.dispose?.()
  }
}

export function createImageProcessingRuntime(
  options: CreateImageProcessingRuntimeOptions,
): ImageProcessingRuntime {
  return new MainThreadImageProcessingRuntime(options)
}

function mapSchedulerResult<TOutput>(
  result: ImageProcessingSchedulerResult<TOutput>,
): ImageProcessingTaskResult<TOutput> {
  switch (result.status) {
    case 'completed':
      return {
        status: 'completed',
        taskId: result.taskId,
        revision: result.revision,
        output: result.value,
      }
    case 'cancelled':
      return {
        status: 'cancelled',
        taskId: result.taskId,
        revision: result.revision,
        error: {
          code: 'cancelled',
          message: result.reason,
        },
      }
    case 'stale':
      return {
        status: 'stale',
        taskId: result.taskId,
        revision: result.revision,
        error: {
          code: 'stale',
          message: result.reason,
        },
      }
    case 'failed':
      return {
        status: 'failed',
        taskId: result.taskId,
        revision: result.revision,
        error: toImageProcessingError(result.error),
      }
  }
}

function toImageProcessingError(error: unknown): ImageProcessingError {
  if (isImageProcessingError(error)) return error
  if (error instanceof Error) {
    return {
      code: 'backend-failed',
      message: error.message || '图像处理失败',
      cause: error,
    }
  }
  return {
    code: 'unknown',
    message: '图像处理失败',
    cause: error,
  }
}

function isImageProcessingError(error: unknown): error is ImageProcessingError {
  if (!error || typeof error !== 'object') return false
  const candidate = error as Partial<ImageProcessingError>
  return isImageProcessingErrorCode(candidate.code) && typeof candidate.message === 'string'
}

function isImageProcessingErrorCode(code: unknown): code is ImageProcessingError['code'] {
  return (
    code === 'backend-failed' ||
    code === 'cancelled' ||
    code === 'encode-failed' ||
    code === 'invalid-source' ||
    code === 'stale' ||
    code === 'unknown'
  )
}

function ignoreProgress() {}

export type { RenderFinalInput, RenderFinalOutput, RenderPreviewInput, RenderPreviewOutput }
