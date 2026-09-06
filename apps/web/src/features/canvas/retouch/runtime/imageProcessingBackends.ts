import { createCpuImageAdjustmentPlanExecutor } from './cpuPlanExecutor'
import { createHybridImageAdjustmentPlanExecutor } from './hybridPlanExecutor'
import { createWorkerImageAdjustmentPlanExecutor } from './workerPlanExecutor'
import type {
  ImageAdjustmentPlanExecutionEventReporter,
  ImageAdjustmentPlanExecutor,
} from './planExecutorTypes'
import { createMainThreadImageProcessingBackend } from './mainThreadImageProcessingBackend'
import type { ImageSourceCache } from './imageSourceCache'
import type { ImageProcessingBackend } from './types'

export type ImageProcessingBackendId = 'cpu-main' | 'cpu-worker' | 'hybrid'

export type CreateImageAdjustmentProcessingBackendOptions = {
  backend?: ImageProcessingBackendId
  sourceCache?: ImageSourceCache
  onPlanExecutorEvent?: ImageAdjustmentPlanExecutionEventReporter
}

export function createImageAdjustmentProcessingBackend(
  options: CreateImageAdjustmentProcessingBackendOptions = {},
): ImageProcessingBackend {
  switch (options.backend ?? 'hybrid') {
    case 'cpu-main':
      return createCpuMainImageProcessingBackend(options)
    case 'cpu-worker':
      return createCpuWorkerImageProcessingBackend(options)
    case 'hybrid':
      return createHybridImageProcessingBackend(options)
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

function createHybridImageProcessingBackend(
  options: Omit<CreateImageAdjustmentProcessingBackendOptions, 'backend'> = {},
): ImageProcessingBackend {
  return createMainThreadBackendWithPlanExecutor(
    options,
    createHybridImageAdjustmentPlanExecutor({
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
