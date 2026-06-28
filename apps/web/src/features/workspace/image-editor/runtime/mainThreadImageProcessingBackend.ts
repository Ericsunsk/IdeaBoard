import { canvasToBlob } from '../../services/flowMediaIO'
import { detectCanvasTransparency } from '../../utils/displayImagePreviewEncoding'
import { releaseCanvas } from '../utils/imageAdjustmentMedia'
import {
  createCpuImageAdjustmentPlanExecutor,
  type ImageAdjustmentPlanExecutionEvent,
  type ImageAdjustmentPlanExecutionEventReporter,
  type ImageAdjustmentPlanExecutor,
} from './imageAdjustmentPlanExecutor'
import {
  createImageSourceCache,
  readImageSourceImageData,
  type ImageSourceCache,
} from './imageSourceCache'
import type {
  ImageProcessingBackend,
  ImageProcessingBackendFinalInput,
  ImageProcessingBackendPreviewInput,
  RenderFinalOutput,
  RenderPreviewOutput,
} from './types'

export type {
  ImageAdjustmentPlanExecutionEvent,
  ImageAdjustmentPlanExecutionEventReporter,
  ImageAdjustmentPlanExecutor,
}

export type MainThreadImageProcessingBackendOptions = {
  sourceCache?: ImageSourceCache
  planExecutor?: ImageAdjustmentPlanExecutor
}

export function createMainThreadImageProcessingBackend(
  options: MainThreadImageProcessingBackendOptions = {},
): ImageProcessingBackend {
  return new MainThreadImageProcessingBackend(
    options.sourceCache ?? createImageSourceCache(),
    options.planExecutor ?? createCpuImageAdjustmentPlanExecutor(),
  )
}

class MainThreadImageProcessingBackend implements ImageProcessingBackend {
  private readonly sourceCache: ImageSourceCache
  private readonly planExecutor: ImageAdjustmentPlanExecutor

  constructor(sourceCache: ImageSourceCache, planExecutor: ImageAdjustmentPlanExecutor) {
    this.sourceCache = sourceCache
    this.planExecutor = planExecutor
  }

  async renderPreview(input: ImageProcessingBackendPreviewInput): Promise<RenderPreviewOutput> {
    if (input.plan.length === 0 || input.context.signal.aborted) {
      return { objectUrl: null }
    }

    input.context.reportProgress({ phase: 'decode' })
    const source = await this.sourceCache.getPreviewSource(input.source.url, {
      maxSide: input.maxSide,
      failureMode: 'return-null',
    })
    if (!source || input.context.signal.aborted) return { objectUrl: null }

    let renderCanvas: HTMLCanvasElement | null = null
    try {
      input.context.reportProgress({ phase: 'render' })
      renderCanvas = createCanvas(source.width, source.height)
      const context = renderCanvas.getContext('2d')
      if (!context) return { objectUrl: null }

      const imageData = readImageSourceImageData(source)
      const adjustedImageData = await this.planExecutor.execute({
        imageData,
        plan: input.plan,
        signal: input.context.signal,
        onProgress: input.context.reportProgress,
      })
      if (input.context.signal.aborted) return { objectUrl: null }
      context.putImageData(adjustedImageData, 0, 0)

      input.context.reportProgress({ phase: 'encode' })
      const encoding = input.context.outputPolicy.resolveEncoding({
        kind: 'preview',
        sourceMimeType: source.mimeType,
        hasTransparentPixels: detectCanvasTransparency(context, source.width, source.height),
      })
      const blob = await canvasToBlob(renderCanvas, encoding.mimeType, encoding.quality)
      if (input.context.signal.aborted) return { objectUrl: null }
      const objectUrl = URL.createObjectURL(blob)
      if (input.context.signal.aborted) {
        URL.revokeObjectURL(objectUrl)
        return { objectUrl: null }
      }

      input.context.reportProgress({ phase: 'complete' })
      return {
        objectUrl,
        width: source.width,
        height: source.height,
        mimeType: blob.type || encoding.mimeType,
      }
    } finally {
      releaseCanvas(renderCanvas)
    }
  }

  async renderFinal(input: ImageProcessingBackendFinalInput): Promise<RenderFinalOutput> {
    if (input.plan.length === 0) throw new Error('没有可应用的编辑图层')
    if (input.context.signal.aborted) throw new Error('图像处理已取消')

    input.context.reportProgress({ phase: 'decode' })
    const source = await this.sourceCache.getFullSizeSource(input.source.url, {
      failureMode: 'throw',
    })
    if (!source) throw new Error('读取图片失败')

    try {
      input.context.reportProgress({ phase: 'render' })
      const imageData = readImageSourceImageData(source)
      const adjustedImageData = await this.planExecutor.execute({
        imageData,
        plan: input.plan,
        signal: input.context.signal,
        onProgress: input.context.reportProgress,
      })
      if (input.context.signal.aborted) throw new Error('图像处理已取消')
      source.context.putImageData(adjustedImageData, 0, 0)

      input.context.reportProgress({ phase: 'encode' })
      const encoding = input.context.outputPolicy.resolveEncoding({
        kind: 'final',
        sourceMimeType: source.mimeType,
        hasTransparentPixels: detectCanvasTransparency(source.context, source.width, source.height),
      })
      const blob = await canvasToBlob(source.canvas, encoding.mimeType, encoding.quality)
      const outputMime = blob.type || encoding.mimeType
      const fileName = input.context.outputPolicy.resolveFileName({
        title: input.source.title,
        mimeType: outputMime,
      })
      if (input.context.signal.aborted) throw new Error('图像处理已取消')

      input.context.reportProgress({ phase: 'complete' })
      return {
        file: new File([blob], fileName, { type: outputMime }),
        width: source.width,
        height: source.height,
        mimeType: outputMime,
      }
    } finally {
      this.sourceCache.releaseSource(source)
    }
  }

  dispose() {
    this.sourceCache.dispose()
    this.planExecutor.dispose?.()
  }
}

function createCanvas(width: number, height: number) {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  return canvas
}
