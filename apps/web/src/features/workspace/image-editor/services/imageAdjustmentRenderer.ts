import type { ImageEditorAdjustmentLayer } from '../types'
import { canvasToBlob, resolveFetchUrl } from '../../services/flowMediaIO'
import { applyImageAdjustments } from '../utils/imageAdjustmentEngine'
import {
  detectCanvasTransparency,
  resolvePreviewMimeType,
  WEBP_PREVIEW_MIME_TYPE,
  WEBP_PREVIEW_QUALITY,
} from '../../utils/displayImagePreviewEncoding'

type ImageAdjustmentRenderSource = {
  url: string
  title?: string
}

export async function renderImageAdjustmentFile(
  source: ImageAdjustmentRenderSource,
  layers: readonly ImageEditorAdjustmentLayer[],
): Promise<File> {
  const visibleLayers = layers.filter((layer) => layer.visible)
  if (visibleLayers.length === 0) {
    throw new Error('没有可应用的调整图层')
  }

  const loaded = await loadImageAdjustmentSource(source.url)
  const canvas = document.createElement('canvas')
  canvas.width = loaded.width
  canvas.height = loaded.height
  const context = canvas.getContext('2d')
  if (!context) throw new Error('当前浏览器不支持图像处理')

  context.drawImage(loaded.canvas, 0, 0)
  const imageData = context.getImageData(0, 0, canvas.width, canvas.height)
  const adjustedImageData = applyImageAdjustments(imageData, visibleLayers)
  context.putImageData(adjustedImageData, 0, 0)

  const hasTransparentPixels = detectCanvasTransparency(context, canvas.width, canvas.height)
  const mime = resolvePreviewMimeType(loaded.mimeType, hasTransparentPixels)
  const blob = await canvasToBlob(
    canvas,
    mime,
    mime === WEBP_PREVIEW_MIME_TYPE ? WEBP_PREVIEW_QUALITY : undefined,
  )
  return new File([blob], createAdjustedFileName(source.title, mime), { type: blob.type || mime })
}

async function loadImageAdjustmentSource(sourceUrl: string) {
  const fetchUrl = resolveFetchUrl(sourceUrl)
  if (!fetchUrl) throw new Error('图片地址无效')

  const response = await fetch(fetchUrl, { credentials: 'include' })
  if (!response.ok) throw new Error('读取图片失败')

  const blob = await response.blob()
  if (!blob.type.startsWith('image/')) throw new Error('读取到的文件不是图片')

  const bitmap = await createImageBitmap(blob)
  try {
    const canvas = document.createElement('canvas')
    canvas.width = bitmap.width
    canvas.height = bitmap.height
    const context = canvas.getContext('2d')
    if (!context) throw new Error('当前浏览器不支持图像处理')
    context.drawImage(bitmap, 0, 0)
    return {
      canvas,
      width: bitmap.width,
      height: bitmap.height,
      mimeType: blob.type,
    }
  } finally {
    bitmap.close()
  }
}

function createAdjustedFileName(title: string | undefined, mime: string) {
  const ext = mime === 'image/png' ? 'png' : 'webp'
  const base = `${title ?? 'image-adjusted'}`
    .trim()
    .replace(/\.[A-Za-z0-9]{1,10}$/, '')
    .replace(/[^\w.-]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return `${base || 'image-adjusted'}-adjusted.${ext}`
}
