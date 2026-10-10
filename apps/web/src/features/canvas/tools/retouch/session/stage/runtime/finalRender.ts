import { canvasToBlob } from '../../../../../infrastructure/media/canvasMediaIo.ts'
import type { ImageRetouchRecipe } from '../../../../../../board/ports/canvas'
import type { RetouchEffectLayer, RetouchLayer, RetouchRasterContent } from '../../../core/model.ts'
import { createImageProcessingOutputFileName } from './imageOutputPolicy'
import { getImageRetouchProcessingRuntime } from './processingRuntime.ts'
import type { ImageProcessingProgress, RenderFinalInput } from './types'
import { ImageProcessingErrorRecord } from './types'
import { createImageSourceCache, readImageSourceImageData } from './imageSourceCache'
import { compositeRetouchLayerStack } from '../../../core/rasterComposite.ts'
import { buildImageRetouchPlan } from '../../../core/planPipeline.ts'
import { deserializeImageRetouchPlan } from '../../../core/recipe.ts'
import {
  flattenVisibleRetouchEffectLayers,
  hasEffectiveRetouchEffectLayers,
} from '../../../core/layers/layerEffects.ts'
import { flattenRetouchRasterContents, hasRetouchRasterEdits } from '../../../core/rasterTools.ts'

export type ImageRetouchRenderSource = {
  url: string
  title?: string
}

const IMAGE_RETOUCH_FINAL_TASK_ID = 'image-retouch-final'

type ImageRetouchRenderOptions = {
  scopeId?: string
  onProgress?: (progress: ImageProcessingProgress) => void
  maxPixels?: number
  maxSide?: number
  fitWithinMaxPixels?: boolean
  preserveSourceMimeType?: boolean
}

export async function renderImageRetouchFile(
  source: ImageRetouchRenderSource,
  layers: readonly RetouchEffectLayer[],
  options: ImageRetouchRenderOptions = {},
): Promise<File> {
  const plan = buildImageRetouchPlan(layers)
  if (plan.length === 0) {
    throw new ImageProcessingErrorRecord('empty-plan')
  }
  return renderImageRetouchPlanFile(source, plan, options)
}

export async function renderImageRetouchRecipeFile(
  source: ImageRetouchRenderSource,
  recipe: ImageRetouchRecipe,
  options: ImageRetouchRenderOptions = {},
): Promise<File> {
  return renderImageRetouchPlanFile(source, deserializeImageRetouchPlan(recipe), {
    ...options,
    preserveSourceMimeType: true,
    maxPixels: options.maxPixels ?? 64_000_000,
    fitWithinMaxPixels: options.fitWithinMaxPixels ?? false,
  })
}

async function renderImageRetouchPlanFile(
  source: ImageRetouchRenderSource,
  plan: ReturnType<typeof deserializeImageRetouchPlan>,
  options: ImageRetouchRenderOptions,
): Promise<File> {
  if (plan.length === 0) throw new ImageProcessingErrorRecord('empty-plan')
  const renderInput: RenderFinalInput = {
    source,
    plan,
    taskId: getImageRetouchFinalTaskId(options.scopeId),
    latestOnly: true,
    maxPixels: options.maxPixels,
    maxSide: options.maxSide,
    fitWithinMaxPixels: options.fitWithinMaxPixels,
    preserveSourceMimeType: options.preserveSourceMimeType,
  }
  if (options.onProgress !== undefined) renderInput.onProgress = options.onProgress
  const result = await getImageRetouchProcessingRuntime().renderFinal(renderInput)
  if (result.status === 'completed') return result.output.file
  throw result.error
}

export async function renderRetouchLayerStackFile(
  source: ImageRetouchRenderSource,
  layers: readonly RetouchLayer[],
  options: ImageRetouchRenderOptions = {},
): Promise<File> {
  if (!hasRetouchRasterEdits(layers)) {
    return renderImageRetouchFile(source, flattenVisibleRetouchEffectLayers(layers), options)
  }

  options.onProgress?.({ phase: 'decode' })
  const sourceCache = createImageSourceCache({ maxEntries: 1 })
  const sourceEntry = await sourceCache.getFullSizeSource(source.url, {
    maxPixels: options.maxPixels,
    maxSide: options.maxSide,
    fitWithinMaxPixels: options.fitWithinMaxPixels ?? true,
  })
  if (!sourceEntry) throw new ImageProcessingErrorRecord('invalid-source')

  try {
    const rasterImages = await loadRasterContentImages(flattenRetouchRasterContents(layers))
    options.onProgress?.({ phase: 'render' })
    const output = compositeRetouchLayerStack(readImageSourceImageData(sourceEntry), layers, {
      getRasterImageData: (content) => rasterImages.get(content.dataUrl) ?? null,
    })

    options.onProgress?.({ phase: 'encode' })
    const canvas = document.createElement('canvas')
    canvas.width = output.width
    canvas.height = output.height
    const context = canvas.getContext('2d')
    if (!context) throw new ImageProcessingErrorRecord('unsupported-canvas')
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

export function hasRenderableRetouchLayerStack(layers: readonly RetouchLayer[]) {
  return hasRetouchRasterEdits(layers) || hasEffectiveRetouchEffectLayers(layers)
}

export function cancelImageRetouchFinalRender(scopeId?: string) {
  getImageRetouchProcessingRuntime().cancel(getImageRetouchFinalTaskId(scopeId))
}

function getImageRetouchFinalTaskId(scopeId?: string) {
  return scopeId ? `${IMAGE_RETOUCH_FINAL_TASK_ID}:${scopeId}` : IMAGE_RETOUCH_FINAL_TASK_ID
}

async function loadRasterContentImages(contents: readonly RetouchRasterContent[]) {
  const images = new Map<string, ImageData>()
  await Promise.all(
    contents.map(async (content) => {
      if (images.has(content.dataUrl)) return
      images.set(content.dataUrl, await decodeRasterContentImageData(content))
    }),
  )
  return images
}

async function decodeRasterContentImageData(content: RetouchRasterContent): Promise<ImageData> {
  const response = await fetch(content.dataUrl)
  const blob = await response.blob()
  const bitmap = await createImageBitmap(blob)
  try {
    const canvas = document.createElement('canvas')
    canvas.width = content.width || bitmap.width
    canvas.height = content.height || bitmap.height
    const context = canvas.getContext('2d', { willReadFrequently: true })
    if (!context) throw new ImageProcessingErrorRecord('unsupported-canvas')
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    return context.getImageData(0, 0, canvas.width, canvas.height)
  } finally {
    bitmap.close()
  }
}
