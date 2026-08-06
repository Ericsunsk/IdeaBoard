import { canvasToBlob } from '../../services/flowMediaIO'
import type {
  ImageEditorAdjustmentLayer,
  ImageEditorLayer,
  ImageEditorRasterContent,
} from '../types'
import { createImageProcessingOutputFileName } from '../runtime/imageOutputPolicy'
import { getImageAdjustmentProcessingRuntime } from '../runtime/imageAdjustmentProcessingRuntime'
import type { ImageProcessingProgress, RenderFinalInput } from '../runtime/types'
import { createImageSourceCache, readImageSourceImageData } from '../runtime/imageSourceCache'
import { compositeImageEditorLayerStack } from '../utils/imageEditorRasterComposite'
import { buildImageAdjustmentPlan } from '../utils/imageAdjustmentPlanPipeline'
import {
  flattenVisibleImageEditorAdjustmentLayers,
  hasEffectiveImageEditorAdjustmentLayers,
} from '../utils/imageEditorLayerAdjustments'
import {
  flattenImageEditorRasterContents,
  hasImageEditorRasterEdits,
} from '../utils/imageEditorRasterTools'

type ImageAdjustmentRenderSource = {
  url: string
  title?: string
}

const IMAGE_ADJUSTMENT_FINAL_TASK_ID = 'image-adjustment-final'

type ImageAdjustmentRenderOptions = {
  scopeId?: string
  onProgress?: (progress: ImageProcessingProgress) => void
}

export async function renderImageAdjustmentFile(
  source: ImageAdjustmentRenderSource,
  layers: readonly ImageEditorAdjustmentLayer[],
  options: ImageAdjustmentRenderOptions = {},
): Promise<File> {
  const plan = buildImageAdjustmentPlan(layers)
  if (plan.length === 0) {
    throw new Error('没有可应用的编辑图层')
  }

  const renderInput: RenderFinalInput = {
    source,
    plan,
    taskId: getImageAdjustmentFinalTaskId(options.scopeId),
    latestOnly: true,
  }
  if (options.onProgress !== undefined) renderInput.onProgress = options.onProgress

  const result = await getImageAdjustmentProcessingRuntime().renderFinal(renderInput)

  if (result.status === 'completed') return result.output.file
  throw new Error(result.error.message)
}

export async function renderImageEditorLayerStackFile(
  source: ImageAdjustmentRenderSource,
  layers: readonly ImageEditorLayer[],
  options: ImageAdjustmentRenderOptions = {},
): Promise<File> {
  if (!hasImageEditorRasterEdits(layers)) {
    return renderImageAdjustmentFile(
      source,
      flattenVisibleImageEditorAdjustmentLayers(layers),
      options,
    )
  }

  options.onProgress?.({ phase: 'decode' })
  const sourceCache = createImageSourceCache({ maxEntries: 1 })
  const sourceEntry = await sourceCache.getFullSizeSource(source.url)
  if (!sourceEntry) throw new Error('读取图片失败')

  try {
    const rasterImages = await loadRasterContentImages(flattenImageEditorRasterContents(layers))
    options.onProgress?.({ phase: 'render' })
    const output = compositeImageEditorLayerStack(readImageSourceImageData(sourceEntry), layers, {
      getRasterImageData: (content) => rasterImages.get(content.dataUrl) ?? null,
    })

    options.onProgress?.({ phase: 'encode' })
    const canvas = document.createElement('canvas')
    canvas.width = output.width
    canvas.height = output.height
    const context = canvas.getContext('2d')
    if (!context) throw new Error('当前浏览器不支持图像处理')
    context.putImageData(output, 0, 0)
    const blob = await canvasToBlob(canvas, 'image/png')
    const fileName = createImageProcessingOutputFileName({
      title: source.title,
      mimeType: 'image/png',
      suffix: 'edited',
      fallbackBaseName: 'image-edited',
    })
    options.onProgress?.({ phase: 'complete' })
    return new File([blob], fileName, { type: blob.type || 'image/png' })
  } finally {
    sourceCache.releaseSource(sourceEntry)
    sourceCache.dispose()
  }
}

export function hasRenderableImageEditorLayerStack(layers: readonly ImageEditorLayer[]) {
  return hasImageEditorRasterEdits(layers) || hasEffectiveImageEditorAdjustmentLayers(layers)
}

export function cancelImageAdjustmentFinalRender(scopeId?: string) {
  getImageAdjustmentProcessingRuntime().cancel(getImageAdjustmentFinalTaskId(scopeId))
}

function getImageAdjustmentFinalTaskId(scopeId?: string) {
  return scopeId ? `${IMAGE_ADJUSTMENT_FINAL_TASK_ID}:${scopeId}` : IMAGE_ADJUSTMENT_FINAL_TASK_ID
}

async function loadRasterContentImages(contents: readonly ImageEditorRasterContent[]) {
  const images = new Map<string, ImageData>()
  await Promise.all(
    contents.map(async (content) => {
      if (images.has(content.dataUrl)) return
      images.set(content.dataUrl, await decodeRasterContentImageData(content))
    }),
  )
  return images
}

async function decodeRasterContentImageData(content: ImageEditorRasterContent): Promise<ImageData> {
  const response = await fetch(content.dataUrl)
  const blob = await response.blob()
  const bitmap = await createImageBitmap(blob)
  try {
    const canvas = document.createElement('canvas')
    canvas.width = content.width || bitmap.width
    canvas.height = content.height || bitmap.height
    const context = canvas.getContext('2d', { willReadFrequently: true })
    if (!context) throw new Error('当前浏览器不支持图像处理')
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    return context.getImageData(0, 0, canvas.width, canvas.height)
  } finally {
    bitmap.close()
  }
}
