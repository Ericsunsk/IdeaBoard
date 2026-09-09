import { createImageProcessingRuntime } from './imageProcessingRuntime'
import { createMainThreadImageProcessingBackend } from './webglImageProcessingBackend'
import type { ImageProcessingRuntime } from './types'

let imageRetouchProcessingRuntime: ImageProcessingRuntime | null = null

export function getImageRetouchProcessingRuntime() {
  if (!imageRetouchProcessingRuntime) {
    imageRetouchProcessingRuntime = createImageProcessingRuntime({
      backend: createMainThreadImageProcessingBackend(),
    })
  }
  return imageRetouchProcessingRuntime
}

export function disposeImageRetouchProcessingRuntime() {
  imageRetouchProcessingRuntime?.dispose()
  imageRetouchProcessingRuntime = null
}
