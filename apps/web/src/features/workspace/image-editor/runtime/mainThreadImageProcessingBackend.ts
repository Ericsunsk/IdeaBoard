import { canvasToBlob } from '../../services/flowMediaIO'
import { detectCanvasTransparency } from '../../utils/displayImagePreviewEncoding'
import { createCpuImageAdjustmentPlanExecutor } from './cpuPlanExecutor'
import {
  createImageSourceCache,
  readImageSourceImageData,
  type ImageSourceCache,
} from './imageSourceCache'
import type { ImageAdjustmentPlanExecutor } from './planExecutorTypes'
import type {
  ImageProcessingBackend,
  ImageProcessingBackendFinalInput,
  RenderFinalOutput,
} from './types'

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

  async renderFinal(input: ImageProcessingBackendFinalInput): Promise<RenderFinalOutput> {
    if (input.plan.length === 0) throw new Error('没有可应用的编辑图层')
    if (input.context.signal.aborted) throw new Error('图像处理已取消')

    input.context.reportProgress({ phase: 'decode' })
    const source = await this.sourceCache.getFullSizeSource(input.source.url)
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
