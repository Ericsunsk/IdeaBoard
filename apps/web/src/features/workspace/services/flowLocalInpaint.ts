import { uploadApi } from '../../../shared/api'
import { USER_DOWNLOAD_ENDPOINT } from '../../../shared/constants/media'

export const FLOW_LOCAL_INPAINT_PADDING = 128

export interface FlowLocalInpaintBBox {
  x: number
  y: number
  width: number
  height: number
}

export interface FlowLocalInpaintCropResult {
  croppedImageUrl: string
  croppedMaskUrl: string
  bbox: FlowLocalInpaintBBox
  originalSize: { width: number; height: number }
  cropSize: { width: number; height: number }
}

interface PrepareFlowLocalInpaintOptions {
  originalImageUrl: string
  maskUrl: string
  boardId?: string
  padding?: number
}

interface ComposeFlowLocalInpaintOptions {
  originalImageUrl: string
  croppedResultUrl: string
  croppedMaskUrl: string
  bbox: FlowLocalInpaintBBox
  boardId?: string
}

function isDataUrl(value: string): boolean {
  return /^data:/i.test(value.trim())
}

function toAbsoluteUrl(raw: string): URL | null {
  try {
    return new URL(raw, window.location.origin)
  } catch {
    return null
  }
}

function resolveFetchUrl(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  if (isDataUrl(trimmed)) return trimmed
  const parsed = toAbsoluteUrl(trimmed)
  if (!parsed) return trimmed
  if (parsed.origin === window.location.origin && parsed.pathname.startsWith(USER_DOWNLOAD_ENDPOINT)) {
    const target = parsed.searchParams.get('url')?.trim()
    if (target) return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(target)}`
    return parsed.href
  }
  if (parsed.origin === window.location.origin && parsed.pathname.startsWith('/uploads')) {
    return parsed.href
  }
  if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
    return `${USER_DOWNLOAD_ENDPOINT}?url=${encodeURIComponent(parsed.href)}`
  }
  return parsed.href
}

async function fetchBlob(rawUrl: string): Promise<Blob> {
  const url = resolveFetchUrl(rawUrl)
  if (!url) throw new Error('图片链接为空')
  const response = await fetch(url, { credentials: 'include' })
  if (!response.ok) {
    throw new Error(`读取图片失败：HTTP ${response.status}`)
  }
  return await response.blob()
}

async function decodeImageBitmap(rawUrl: string): Promise<ImageBitmap> {
  const blob = await fetchBlob(rawUrl)
  return await createImageBitmap(blob)
}

function uploadBlobFile(blob: Blob, name: string, boardId?: string): Promise<string> {
  const file = new File([blob], name, { type: blob.type || 'image/png' })
  return uploadApi.upload(file, boardId).then((res) => {
    if (res.code !== 0 || !res.data?.url) {
      throw new Error(res.msg || '上传图片失败')
    }
    return res.data.url
  })
}

function canvasToBlob(canvas: HTMLCanvasElement, type = 'image/png', quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error('图片编码失败'))
        return
      }
      resolve(blob)
    }, type, quality)
  })
}

function computeMaskBbox(maskData: ImageData): FlowLocalInpaintBBox | null {
  const { width, height, data } = maskData
  let minX = width
  let minY = height
  let maxX = -1
  let maxY = -1
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const alpha = data[(y * width + x) * 4 + 3]
      if (alpha <= 0) continue
      if (x < minX) minX = x
      if (y < minY) minY = y
      if (x > maxX) maxX = x
      if (y > maxY) maxY = y
    }
  }
  if (maxX < minX || maxY < minY) return null
  return {
    x: minX,
    y: minY,
    width: maxX - minX + 1,
    height: maxY - minY + 1,
  }
}

function expandBbox(
  bbox: FlowLocalInpaintBBox,
  imageSize: { width: number; height: number },
  padding: number,
): FlowLocalInpaintBBox {
  const left = Math.max(0, bbox.x - padding)
  const top = Math.max(0, bbox.y - padding)
  const right = Math.min(imageSize.width, bbox.x + bbox.width + padding)
  const bottom = Math.min(imageSize.height, bbox.y + bbox.height + padding)
  return {
    x: left,
    y: top,
    width: Math.max(1, right - left),
    height: Math.max(1, bottom - top),
  }
}

export async function prepareFlowLocalInpaint(options: PrepareFlowLocalInpaintOptions): Promise<FlowLocalInpaintCropResult> {
  const padding = Number.isFinite(options.padding) ? Math.max(0, Math.round(options.padding ?? FLOW_LOCAL_INPAINT_PADDING)) : FLOW_LOCAL_INPAINT_PADDING
  let originalBitmap: ImageBitmap | null = null
  let maskBitmap: ImageBitmap | null = null
  try {
    [originalBitmap, maskBitmap] = await Promise.all([
      decodeImageBitmap(options.originalImageUrl),
      decodeImageBitmap(options.maskUrl),
    ])
    const width = Math.max(1, Math.round(originalBitmap.width))
    const height = Math.max(1, Math.round(originalBitmap.height))

    const maskCanvas = document.createElement('canvas')
    maskCanvas.width = width
    maskCanvas.height = height
    const maskCtx = maskCanvas.getContext('2d')
    if (!maskCtx) throw new Error('初始化蒙版失败')
    maskCtx.drawImage(maskBitmap, 0, 0, width, height)

    const maskData = maskCtx.getImageData(0, 0, width, height)
    const rawBbox = computeMaskBbox(maskData)
    if (!rawBbox) {
      throw new Error('Mask 为空，请先涂抹需要处理的区域')
    }
    const bbox = expandBbox(rawBbox, { width, height }, padding)

    const imageCropCanvas = document.createElement('canvas')
    imageCropCanvas.width = bbox.width
    imageCropCanvas.height = bbox.height
    const imageCropCtx = imageCropCanvas.getContext('2d')
    if (!imageCropCtx) throw new Error('初始化图片裁切失败')
    imageCropCtx.drawImage(
      originalBitmap,
      bbox.x,
      bbox.y,
      bbox.width,
      bbox.height,
      0,
      0,
      bbox.width,
      bbox.height,
    )

    const maskCropCanvas = document.createElement('canvas')
    maskCropCanvas.width = bbox.width
    maskCropCanvas.height = bbox.height
    const maskCropCtx = maskCropCanvas.getContext('2d')
    if (!maskCropCtx) throw new Error('初始化蒙版裁切失败')
    maskCropCtx.drawImage(
      maskCanvas,
      bbox.x,
      bbox.y,
      bbox.width,
      bbox.height,
      0,
      0,
      bbox.width,
      bbox.height,
    )
    const maskCropData = maskCropCtx.getImageData(0, 0, bbox.width, bbox.height)
    for (let i = 0; i < maskCropData.data.length; i += 4) {
      const alpha = maskCropData.data[i + 3]
      maskCropData.data[i] = 255
      maskCropData.data[i + 1] = 255
      maskCropData.data[i + 2] = 255
      maskCropData.data[i + 3] = alpha > 0 ? 255 : 0
    }
    maskCropCtx.putImageData(maskCropData, 0, 0)

    const [imageBlob, maskBlob] = await Promise.all([
      canvasToBlob(imageCropCanvas, 'image/png'),
      canvasToBlob(maskCropCanvas, 'image/png'),
    ])
    const [croppedImageUrl, croppedMaskUrl] = await Promise.all([
      uploadBlobFile(imageBlob, `flow-local-inpaint-image-${Date.now()}.png`, options.boardId),
      uploadBlobFile(maskBlob, `flow-local-inpaint-mask-${Date.now()}.png`, options.boardId),
    ])

    return {
      croppedImageUrl,
      croppedMaskUrl,
      bbox,
      originalSize: { width, height },
      cropSize: { width: bbox.width, height: bbox.height },
    }
  } finally {
    originalBitmap?.close()
    maskBitmap?.close()
  }
}

export async function composeFlowLocalInpaint(options: ComposeFlowLocalInpaintOptions): Promise<string> {
  let originalBitmap: ImageBitmap | null = null
  let resultBitmap: ImageBitmap | null = null
  let maskBitmap: ImageBitmap | null = null
  try {
    [originalBitmap, resultBitmap, maskBitmap] = await Promise.all([
      decodeImageBitmap(options.originalImageUrl),
      decodeImageBitmap(options.croppedResultUrl),
      decodeImageBitmap(options.croppedMaskUrl),
    ])

    const outputCanvas = document.createElement('canvas')
    outputCanvas.width = Math.max(1, Math.round(originalBitmap.width))
    outputCanvas.height = Math.max(1, Math.round(originalBitmap.height))
    const outputCtx = outputCanvas.getContext('2d')
    if (!outputCtx) throw new Error('初始化结果画布失败')
    outputCtx.drawImage(originalBitmap, 0, 0, outputCanvas.width, outputCanvas.height)

    const patchCanvas = document.createElement('canvas')
    patchCanvas.width = Math.max(1, Math.round(options.bbox.width))
    patchCanvas.height = Math.max(1, Math.round(options.bbox.height))
    const patchCtx = patchCanvas.getContext('2d')
    if (!patchCtx) throw new Error('初始化局部画布失败')
    patchCtx.drawImage(resultBitmap, 0, 0, patchCanvas.width, patchCanvas.height)
    patchCtx.globalCompositeOperation = 'destination-in'
    patchCtx.drawImage(maskBitmap, 0, 0, patchCanvas.width, patchCanvas.height)
    patchCtx.globalCompositeOperation = 'source-over'

    outputCtx.drawImage(
      patchCanvas,
      0,
      0,
      patchCanvas.width,
      patchCanvas.height,
      options.bbox.x,
      options.bbox.y,
      options.bbox.width,
      options.bbox.height,
    )

    const finalBlob = await canvasToBlob(outputCanvas, 'image/png')
    return await uploadBlobFile(finalBlob, `flow-local-inpaint-final-${Date.now()}.png`, options.boardId)
  } finally {
    originalBitmap?.close()
    resultBitmap?.close()
    maskBitmap?.close()
  }
}

export async function cleanupFlowLocalInpaintAssets(
  localInpaint: FlowLocalInpaintCropResult | null | undefined,
  boardId?: string,
): Promise<void> {
  const normalizedBoardId = `${boardId ?? ''}`.trim()
  if (!localInpaint || !normalizedBoardId) return
  const recycleTargets = [localInpaint.croppedImageUrl, localInpaint.croppedMaskUrl]
    .map(url => url.trim())
    .filter(Boolean)
  if (recycleTargets.length === 0) return
  await Promise.allSettled(recycleTargets.map(url => uploadApi.recycle(url, normalizedBoardId)))
}
