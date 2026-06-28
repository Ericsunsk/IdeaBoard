import type { ImageEditorFilterPreview } from '../types'
import { executeImageAdjustmentPlan, type ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import { cloneImageData } from '../utils/imageDataUtils'
import { releaseCanvas } from '../utils/imageAdjustmentMedia'
import { resolveFetchUrl } from '../../services/flowMediaIO'

const PREVIEW_MAX_SIDE = 960

type PreviewSource = {
  sourceUrl: string
  imageData: ImageData
  width: number
  height: number
}

type ImageAdjustmentInlinePreviewRendererOptions = {
  onPreview: (preview: ImageEditorFilterPreview | null) => void
  onError?: (error: unknown) => void
}

export type ImageAdjustmentInlinePreviewRenderer = {
  updateSource: (sourceUrl: string | null) => void
  updatePlan: (plan: ImageAdjustmentPlan) => void
  dispose: () => void
}

export function createImageAdjustmentInlinePreviewRenderer(
  options: ImageAdjustmentInlinePreviewRendererOptions,
): ImageAdjustmentInlinePreviewRenderer {
  let previewSource: PreviewSource | null = null
  let currentPlan: ImageAdjustmentPlan = []
  let currentSourceUrl: string | null = null
  let sourceRequestToken = 0
  let previewFrameId: number | null = null

  return {
    updateSource(sourceUrl) {
      if (sourceUrl === currentSourceUrl) return
      currentSourceUrl = sourceUrl
      sourceRequestToken += 1
      previewSource = null
      options.onPreview(null)
      cancelPreviewFrame()
      if (!sourceUrl) return

      const token = sourceRequestToken
      void loadPreviewSource(sourceUrl)
        .then((source) => {
          if (token !== sourceRequestToken) return
          previewSource = source
          scheduleRenderedPreview()
        })
        .catch((error) => {
          if (token !== sourceRequestToken) return
          options.onError?.(error)
        })
    },
    updatePlan(plan) {
      currentPlan = plan
      scheduleRenderedPreview()
    },
    dispose() {
      sourceRequestToken += 1
      currentSourceUrl = null
      previewSource = null
      currentPlan = []
      options.onPreview(null)
      cancelPreviewFrame()
    },
  }

  function scheduleRenderedPreview() {
    cancelPreviewFrame()
    if (currentPlan.length === 0) {
      options.onPreview(null)
      return
    }
    if (!previewSource) return

    let didRunSynchronously = false
    const frameId = window.requestAnimationFrame(() => {
      didRunSynchronously = true
      previewFrameId = null
      renderLatestPreview(previewSource, currentPlan)
    })
    previewFrameId = didRunSynchronously ? null : frameId
  }

  function cancelPreviewFrame() {
    if (previewFrameId == null) return
    window.cancelAnimationFrame(previewFrameId)
    previewFrameId = null
  }

  function renderLatestPreview(source: PreviewSource | null, plan: ImageAdjustmentPlan) {
    if (!source || plan.length === 0) {
      options.onPreview(null)
      return
    }
    const imageData = cloneImageData(source.imageData)
    executeImageAdjustmentPlan(imageData, plan)
    options.onPreview({
      type: 'rendered-image',
      imageData,
      width: source.width,
      height: source.height,
    })
  }
}

async function loadPreviewSource(sourceUrl: string): Promise<PreviewSource> {
  const fetchUrl = resolveFetchUrl(sourceUrl)
  if (!fetchUrl) throw new Error('图片地址无效')

  const response = await fetch(fetchUrl, { credentials: 'include' })
  if (!response.ok) throw new Error('读取图片失败')

  const blob = await response.blob()
  if (!blob.type.startsWith('image/')) throw new Error('读取到的文件不是图片')

  const bitmap = await createImageBitmap(blob)
  let canvas: HTMLCanvasElement | null = null
  try {
    const { width, height } = resolvePreviewSize(bitmap.width, bitmap.height)
    canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (!context) throw new Error('当前浏览器不支持图像处理')
    context.drawImage(bitmap, 0, 0, width, height)
    return {
      sourceUrl,
      imageData: context.getImageData(0, 0, width, height),
      width,
      height,
    }
  } finally {
    bitmap.close()
    releaseCanvas(canvas)
  }
}

function resolvePreviewSize(width: number, height: number) {
  const longestSide = Math.max(width, height)
  if (longestSide <= 0) throw new Error('图片解码失败')
  const scale = Math.min(1, PREVIEW_MAX_SIDE / longestSide)
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  }
}
