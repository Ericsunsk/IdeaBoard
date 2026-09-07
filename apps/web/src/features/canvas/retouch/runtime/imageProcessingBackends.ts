import { createCpuImageRetouchPlanExecutor } from './cpuPlanExecutor'
import { createHybridImageRetouchPlanExecutor } from './hybridPlanExecutor'
import { createWorkerImageRetouchPlanExecutor } from './workerPlanExecutor'
import type {
  ImageRetouchPlanExecutionEventReporter,
  ImageRetouchPlanExecutor,
} from './planExecutorTypes'
import { createMainThreadImageProcessingBackend } from './mainThreadImageProcessingBackend'
import type { ImageSourceCache } from './imageSourceCache'
import type { ImageProcessingBackend } from './types'

export type ImageProcessingBackendId = 'cpu-main' | 'cpu-worker' | 'hybrid'

export type CreateImageRetouchProcessingBackendOptions = {
  backend?: ImageProcessingBackendId
  sourceCache?: ImageSourceCache
  onPlanExecutorEvent?: ImageRetouchPlanExecutionEventReporter
}

export function createImageRetouchProcessingBackend(
  options: CreateImageRetouchProcessingBackendOptions = {},
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
  options: Omit<CreateImageRetouchProcessingBackendOptions, 'backend'> = {},
): ImageProcessingBackend {
  return createMainThreadBackendWithPlanExecutor(
    options,
    createCpuImageRetouchPlanExecutor({
      onPlanExecutorEvent: options.onPlanExecutorEvent,
    }),
  )
}

export function createCpuWorkerImageProcessingBackend(
  options: Omit<CreateImageRetouchProcessingBackendOptions, 'backend'> = {},
): ImageProcessingBackend {
  return createMainThreadBackendWithPlanExecutor(
    options,
    createWorkerImageRetouchPlanExecutor({
      onPlanExecutorEvent: options.onPlanExecutorEvent,
    }),
  )
}

function createHybridImageProcessingBackend(
  options: Omit<CreateImageRetouchProcessingBackendOptions, 'backend'> = {},
): ImageProcessingBackend {
  return createMainThreadBackendWithPlanExecutor(
    options,
    createHybridImageRetouchPlanExecutor({
      onPlanExecutorEvent: options.onPlanExecutorEvent,
    }),
  )
}

function createMainThreadBackendWithPlanExecutor(
  options: Pick<CreateImageRetouchProcessingBackendOptions, 'sourceCache'>,
  planExecutor: ImageRetouchPlanExecutor,
) {
  return createMainThreadImageProcessingBackend({
    sourceCache: options.sourceCache,
    planExecutor,
  })
}
