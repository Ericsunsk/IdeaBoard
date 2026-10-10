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
  RenderFinalResult,
  RenderPreviewInput,
  RenderPreviewResult,
} from './types'
import { isImageProcessingError } from './types'

export type CreateImageProcessingRuntimeOptions = {
  backend: ImageProcessingBackend
  outputPolicy?: ImageProcessingOutputPolicy
  createTaskId?: (revision: number) => string
}

class MainThreadImageProcessingRuntime implements ImageProcessingRuntime {
  private readonly backend: ImageProcessingBackend
  private readonly outputPolicy: ImageProcessingOutputPolicy
  private readonly finalScheduler: ImageProcessingScheduler
  private readonly previewScheduler: ImageProcessingScheduler

  constructor(options: CreateImageProcessingRuntimeOptions) {
    this.backend = options.backend
    this.outputPolicy = options.outputPolicy ?? createImageProcessingOutputPolicy()
    this.finalScheduler = new ImageProcessingScheduler({
      latestOnly: false,
      createTaskId: options.createTaskId,
    })
    this.previewScheduler = new ImageProcessingScheduler({
      latestOnly: true,
      createTaskId: options.createTaskId,
    })
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

  async renderPreview(input: RenderPreviewInput): Promise<RenderPreviewResult> {
    const result = await this.previewScheduler.schedule(
      (context) =>
        this.backend.renderPreview({
          source: input.source,
          plan: input.plan,
          sizeStrategy: input.sizeStrategy,
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
        latestOnly: input.latestOnly ?? true,
      },
    )
    return mapSchedulerResult(result)
  }

  cancel(taskId?: string) {
    this.finalScheduler.cancel(taskId)
    this.previewScheduler.cancel(taskId)
    this.backend.cancel?.(taskId)
  }

  dispose() {
    this.finalScheduler.dispose()
    this.previewScheduler.dispose()
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
      message: error.message || 'backend-failed',
      cause: error,
    }
  }
  return {
    code: 'unknown',
    message: 'unknown',
    cause: error,
  }
}

function ignoreProgress() {}
