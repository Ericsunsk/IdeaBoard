import { createImageProcessingRuntime } from './ImageProcessingRuntime'
import { createImageAdjustmentProcessingBackend } from './imageProcessingBackends'
import { createBrowserPlanExecutorDiagnosticsReporter } from './imageAdjustmentRuntimeDiagnostics'
import type { ImageProcessingRuntime } from './types'

let imageAdjustmentProcessingRuntime: ImageProcessingRuntime | null = null

export function getImageAdjustmentProcessingRuntime() {
  if (!imageAdjustmentProcessingRuntime) {
    imageAdjustmentProcessingRuntime = createImageProcessingRuntime({
      backend: createImageAdjustmentProcessingBackend({
        onPlanExecutorEvent: createBrowserPlanExecutorDiagnosticsReporter(),
      }),
    })
  }
  return imageAdjustmentProcessingRuntime
}

export function disposeImageAdjustmentProcessingRuntime() {
  imageAdjustmentProcessingRuntime?.dispose()
  imageAdjustmentProcessingRuntime = null
}
