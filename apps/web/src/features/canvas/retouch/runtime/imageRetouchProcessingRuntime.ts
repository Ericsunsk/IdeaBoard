import { createImageProcessingRuntime } from './ImageProcessingRuntime'
import { createImageRetouchProcessingBackend } from './imageProcessingBackends'
import { createBrowserPlanExecutorDiagnosticsReporter } from './imageRetouchRuntimeDiagnostics'
import type { ImageProcessingRuntime } from './types'

let imageRetouchProcessingRuntime: ImageProcessingRuntime | null = null

export function getImageRetouchProcessingRuntime() {
  if (!imageRetouchProcessingRuntime) {
    imageRetouchProcessingRuntime = createImageProcessingRuntime({
      backend: createImageRetouchProcessingBackend({
        onPlanExecutorEvent: createBrowserPlanExecutorDiagnosticsReporter(),
      }),
    })
  }
  return imageRetouchProcessingRuntime
}

export function disposeImageRetouchProcessingRuntime() {
  imageRetouchProcessingRuntime?.dispose()
  imageRetouchProcessingRuntime = null
}
