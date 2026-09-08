import { createImageProcessingRuntime } from './ImageProcessingRuntime'
import { createHybridImageRetouchPlanExecutor } from './hybridPlanExecutor'
import { createBrowserPlanExecutorDiagnosticsReporter } from './imageRetouchRuntimeDiagnostics'
import { createMainThreadImageProcessingBackend } from './mainThreadImageProcessingBackend'
import type { ImageProcessingRuntime } from './types'

let imageRetouchProcessingRuntime: ImageProcessingRuntime | null = null

export function getImageRetouchProcessingRuntime() {
  if (!imageRetouchProcessingRuntime) {
    imageRetouchProcessingRuntime = createImageProcessingRuntime({
      backend: createMainThreadImageProcessingBackend({
        planExecutor: createHybridImageRetouchPlanExecutor({
          onPlanExecutorEvent: createBrowserPlanExecutorDiagnosticsReporter(),
        }),
      }),
    })
  }
  return imageRetouchProcessingRuntime
}

export function disposeImageRetouchProcessingRuntime() {
  imageRetouchProcessingRuntime?.dispose()
  imageRetouchProcessingRuntime = null
}
