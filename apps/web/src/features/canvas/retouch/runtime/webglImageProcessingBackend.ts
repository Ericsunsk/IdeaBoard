import { processingCancelledError } from './processingCancelled'
import { i18n } from '../../../../shared/i18n'
import { canvasToBlob } from '../../services/canvasMediaIO'
import { detectCanvasTransparency } from '../../utils/displayImagePreviewEncoding'
import { resolveImageExportEncoding } from './imageOutputPolicy'
import {
  createImageSourceCache,
  readImageSourceImageData,
  type ImageSourceCache,
} from './imageSourceCache'
import {
  bakeWebGLRetouchUniforms,
  createWebGLRetouchRenderer,
  type WebGLRetouchRenderer,
} from '../utils/webglEngine'
import type {
  ImageProcessingBackend,
  ImageProcessingBackendFinalInput,
  ImageProcessingBackendPreviewInput,
  RenderPreviewOutput,
  RenderFinalOutput,
} from './types'

const DEFAULT_PREVIEW_MAX_SIDE = 960

export type MainThreadImageProcessingBackendOptions = {
  sourceCache?: ImageSourceCache | undefined
  rendererFactory?: (canvas: HTMLCanvasElement) => WebGLRetouchRenderer | null
}

export function createMainThreadImageProcessingBackend(
  options: MainThreadImageProcessingBackendOptions = {},
): ImageProcessingBackend {
  return new MainThreadImageProcessingBackend(
    options.sourceCache ?? createImageSourceCache(),
    options.rendererFactory ?? createWebGLRetouchRenderer,
  )
}

class MainThreadImageProcessingBackend implements ImageProcessingBackend {
  private readonly sourceCache: ImageSourceCache
  private readonly rendererFactory: (canvas: HTMLCanvasElement) => WebGLRetouchRenderer | null

  constructor(
    sourceCache: ImageSourceCache,
    rendererFactory: (canvas: HTMLCanvasElement) => WebGLRetouchRenderer | null,
  ) {
    this.sourceCache = sourceCache
    this.rendererFactory = rendererFactory
  }

  async renderFinal(input: ImageProcessingBackendFinalInput): Promise<RenderFinalOutput> {
    if (input.plan.length === 0) throw new Error(i18n.global.t('canvas.tools.noEditableLayers'))
    if (input.context.signal.aborted) throw processingCancelledError()

    input.context.reportProgress({ phase: 'decode' })
    const source = await this.sourceCache.getFullSizeSource(input.source.url, {
      maxPixels: input.maxPixels,
      maxSide: input.maxSide,
      fitWithinMaxPixels: input.fitWithinMaxPixels ?? true,
    })
    if (!source) throw new Error(i18n.global.t('canvas.retouch.errors.readImageFailed'))

    const outputCanvas = document.createElement('canvas')
    outputCanvas.width = source.width
    outputCanvas.height = source.height
    const renderer = this.rendererFactory(outputCanvas)
    if (!renderer || renderer.isContextLost()) {
      this.sourceCache.releaseSource(source)
      throw new Error(i18n.global.t('canvas.retouch.errors.unsupportedCanvas'))
    }

    try {
      input.context.reportProgress({ phase: 'render' })
      renderer.updateSourceImage(source.canvas, source.width, source.height)
      renderer.updateUniforms(bakeWebGLRetouchUniforms(input.plan))
      renderer.render()
      if (input.context.signal.aborted) throw processingCancelledError()

      input.context.reportProgress({ phase: 'encode' })
      const hasTransparentPixels = detectCanvasTransparency(
        source.context,
        source.width,
        source.height,
      )
      const encoding = input.preserveSourceMimeType
        ? resolveImageExportEncoding(source.mimeType, hasTransparentPixels)
        : input.context.outputPolicy.resolveEncoding({
            sourceMimeType: source.mimeType,
            hasTransparentPixels,
          })
      const blob = await canvasToBlob(outputCanvas, encoding.mimeType, encoding.quality)
      const outputMime = blob.type || encoding.mimeType
      const fileName = input.context.outputPolicy.resolveFileName({
        title: input.source.title,
        mimeType: outputMime,
      })
      if (input.context.signal.aborted) throw processingCancelledError()

      input.context.reportProgress({ phase: 'complete' })
      return {
        file: new File([blob], fileName, { type: outputMime }),
        width: source.width,
        height: source.height,
        mimeType: outputMime,
      }
    } finally {
      renderer.dispose()
      this.sourceCache.releaseSource(source)
    }
  }

  async renderPreview(input: ImageProcessingBackendPreviewInput): Promise<RenderPreviewOutput> {
    if (input.context.signal.aborted) throw processingCancelledError()

    input.context.reportProgress({ phase: 'decode' })
    const source = await this.sourceCache.getFullSizeSource(input.source.url, {
      maxSide: resolvePreviewMaxSide(input.sizeStrategy),
    })
    if (!source) throw new Error(i18n.global.t('canvas.retouch.errors.readImageFailed'))

    const previewCanvas = document.createElement('canvas')
    previewCanvas.width = source.width
    previewCanvas.height = source.height
    const renderer = this.rendererFactory(previewCanvas)
    if (!renderer || renderer.isContextLost()) {
      this.sourceCache.releaseSource(source)
      throw new Error(i18n.global.t('canvas.retouch.errors.unsupportedCanvas'))
    }

    try {
      input.context.reportProgress({ phase: 'render' })
      renderer.updateSourceImage(source.canvas, source.width, source.height)
      if (input.plan.length > 0) {
        renderer.updateUniforms(bakeWebGLRetouchUniforms(input.plan))
      }
      renderer.render()
      if (input.context.signal.aborted) throw processingCancelledError()

      const previewImageData = renderer.readPixels() ?? readImageSourceImageData(source)
      const sourceImageData = readImageSourceImageData(source)

      input.context.reportProgress({ phase: 'complete' })
      return {
        sourceImageData,
        previewImageData,
        width: source.width,
        height: source.height,
      }
    } finally {
      renderer.dispose()
      this.sourceCache.releaseSource(source)
    }
  }

  dispose() {
    this.sourceCache.dispose()
  }
}

function resolvePreviewMaxSide(input: ImageProcessingBackendPreviewInput['sizeStrategy']) {
  return Math.max(1, Math.floor(input?.maxSide ?? DEFAULT_PREVIEW_MAX_SIDE))
}
