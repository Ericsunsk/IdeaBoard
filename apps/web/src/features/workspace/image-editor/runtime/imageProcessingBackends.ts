import {
  createCpuImageAdjustmentPlanExecutor,
  createWorkerImageAdjustmentPlanExecutor,
  type ImageAdjustmentPlanExecutionEventReporter,
} from './imageAdjustmentPlanExecutor'
import {
  createMainThreadImageProcessingBackend,
  type ImageAdjustmentPlanExecutor,
} from './mainThreadImageProcessingBackend'
import type { ImageSourceCache } from './imageSourceCache'
import type { ImageProcessingBackend } from './types'

export type ImageProcessingBackendId = 'cpu-main' | 'cpu-worker'

export type CreateImageAdjustmentProcessingBackendOptions = {
  backend?: ImageProcessingBackendId
  sourceCache?: ImageSourceCache
  onPlanExecutorEvent?: ImageAdjustmentPlanExecutionEventReporter
}

export function createImageAdjustmentProcessingBackend(
  options: CreateImageAdjustmentProcessingBackendOptions = {},
): ImageProcessingBackend {
  switch (options.backend ?? 'cpu-worker') {
    case 'cpu-main':
      return createCpuMainImageProcessingBackend(options)
    case 'cpu-worker':
      return createCpuWorkerImageProcessingBackend(options)
  }
}

export function createCpuMainImageProcessingBackend(
  options: Omit<CreateImageAdjustmentProcessingBackendOptions, 'backend'> = {},
): ImageProcessingBackend {
  return createMainThreadBackendWithPlanExecutor(
    options,
    createCpuImageAdjustmentPlanExecutor({
      onPlanExecutorEvent: options.onPlanExecutorEvent,
    }),
  )
}

export function createCpuWorkerImageProcessingBackend(
  options: Omit<CreateImageAdjustmentProcessingBackendOptions, 'backend'> = {},
): ImageProcessingBackend {
  return createMainThreadBackendWithPlanExecutor(
    options,
    createWorkerImageAdjustmentPlanExecutor({
      onPlanExecutorEvent: options.onPlanExecutorEvent,
    }),
  )
}

function createMainThreadBackendWithPlanExecutor(
  options: Pick<CreateImageAdjustmentProcessingBackendOptions, 'sourceCache'>,
  planExecutor: ImageAdjustmentPlanExecutor,
) {
  return createMainThreadImageProcessingBackend({
    sourceCache: options.sourceCache,
    planExecutor,
  })
}
