import type { MaskNodeItem } from '../types/canvasNodes'
import { uploadApi } from '../../../shared/api/upload'
import { ensureFlowMaskImageUrl } from './flowMediaUpload'
import { isDataUrl, resolveFetchUrl } from './flowMediaIO'

interface ResolveFlowExecutionMaskUrlOptions {
  boardId?: string
}

export interface FlowExecutionMaskAsset {
  url: string
  temporary: boolean
}

export function clampMaskRefineAmount(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(-100, Math.min(100, Math.round(value)))
}

async function decodeMaskBitmap(rawUrl: string): Promise<ImageBitmap> {
  const resolvedUrl = resolveFetchUrl(rawUrl)
  const response = await fetch(resolvedUrl, { credentials: 'include' })
  if (!response.ok) {
    throw new Error(`读取 Mask 失败：HTTP ${response.status}`)
  }
  const blob = await response.blob()
  return await createImageBitmap(blob)
}

function extractMaskIntensity(
  data: Uint8ClampedArray,
  width: number,
  height: number,
): Float32Array {
  const total = width * height
  const values = new Float32Array(total)
  for (let i = 0; i < total; i += 1) {
    const base = i * 4
    const alpha = data[base + 3] / 255
    if (alpha > 0) {
      values[i] = alpha
      continue
    }
    const luminance = (0.299 * data[base] + 0.587 * data[base + 1] + 0.114 * data[base + 2]) / 255
    values[i] = luminance
  }
  return values
}

function runMaskMorphology(
  source: Float32Array,
  width: number,
  height: number,
  mode: 'dilate' | 'erode',
  rounds: number,
): Float32Array {
  if (rounds <= 0) return source
  let input = source
  for (let r = 0; r < rounds; r += 1) {
    const output = new Float32Array(input.length)
    for (let y = 0; y < height; y += 1) {
      for (let x = 0; x < width; x += 1) {
        let next = mode === 'dilate' ? 0 : 1
        for (let ky = -1; ky <= 1; ky += 1) {
          const sy = y + ky
          if (sy < 0 || sy >= height) continue
          for (let kx = -1; kx <= 1; kx += 1) {
            const sx = x + kx
            if (sx < 0 || sx >= width) continue
            const val = input[sy * width + sx]
            if (mode === 'dilate') next = Math.max(next, val)
            else next = Math.min(next, val)
          }
        }
        output[y * width + x] = next
      }
    }
    input = output
  }
  return input
}

function runMaskFeather(
  source: Float32Array,
  width: number,
  height: number,
  passes: number,
): Float32Array {
  if (passes <= 0) return source
  let input = source
  for (let p = 0; p < passes; p += 1) {
    const output = new Float32Array(input.length)
    for (let y = 0; y < height; y += 1) {
      for (let x = 0; x < width; x += 1) {
        let sum = 0
        let count = 0
        for (let ky = -1; ky <= 1; ky += 1) {
          const sy = y + ky
          if (sy < 0 || sy >= height) continue
          for (let kx = -1; kx <= 1; kx += 1) {
            const sx = x + kx
            if (sx < 0 || sx >= width) continue
            sum += input[sy * width + sx]
            count += 1
          }
        }
        output[y * width + x] = count > 0 ? sum / count : 0
      }
    }
    input = output
  }
  return input
}

async function buildRefinedExecutionMaskDataUrl(
  maskUrl: string,
  refineAmount: number,
  invertMask: boolean,
): Promise<string> {
  let maskBitmap: ImageBitmap | null = null
  try {
    maskBitmap = await decodeMaskBitmap(maskUrl)
    const width = Math.max(1, Math.round(maskBitmap.width))
    const height = Math.max(1, Math.round(maskBitmap.height))
    const sourceCanvas = document.createElement('canvas')
    sourceCanvas.width = width
    sourceCanvas.height = height
    const sourceCtx = sourceCanvas.getContext('2d')
    if (!sourceCtx) throw new Error('初始化 Mask 画布失败')
    sourceCtx.drawImage(maskBitmap, 0, 0, width, height)
    const source = sourceCtx.getImageData(0, 0, width, height)

    let maskValues = extractMaskIntensity(source.data, width, height)
    if (invertMask) {
      for (let i = 0; i < maskValues.length; i += 1) {
        maskValues[i] = 1 - maskValues[i]
      }
    }

    const amount = clampMaskRefineAmount(refineAmount)
    const rounds = Math.round(Math.abs(amount) / 12)
    if (rounds > 0) {
      maskValues = runMaskMorphology(
        maskValues,
        width,
        height,
        amount > 0 ? 'dilate' : 'erode',
        rounds,
      )
    }
    if (amount !== 0) {
      maskValues = runMaskFeather(maskValues, width, height, 1)
    }

    const outputCanvas = document.createElement('canvas')
    outputCanvas.width = width
    outputCanvas.height = height
    const outputCtx = outputCanvas.getContext('2d')
    if (!outputCtx) throw new Error('初始化输出 Mask 失败')
    const output = outputCtx.createImageData(width, height)
    for (let i = 0; i < maskValues.length; i += 1) {
      const base = i * 4
      const value = Math.max(0, Math.min(1, maskValues[i]))
      output.data[base] = 255
      output.data[base + 1] = 255
      output.data[base + 2] = 255
      output.data[base + 3] = Math.round(value * 255)
    }
    outputCtx.putImageData(output, 0, 0)
    return outputCanvas.toDataURL('image/png')
  } finally {
    maskBitmap?.close()
  }
}

export async function resolveFlowExecutionMaskAsset(
  maskNode: MaskNodeItem,
  options: ResolveFlowExecutionMaskUrlOptions = {},
): Promise<FlowExecutionMaskAsset> {
  const rawMaskUrl = (maskNode.executionMaskDataUrl || maskNode.maskDataUrl || '').trim()
  if (!rawMaskUrl) return { url: '', temporary: false }

  const refineAmount = clampMaskRefineAmount(maskNode.refineAmount ?? 0)
  const invertMask = !!maskNode.invertMask
  try {
    if (refineAmount === 0 && !invertMask) {
      if (isDataUrl(rawMaskUrl)) {
        return {
          url: await ensureFlowMaskImageUrl(rawMaskUrl, { boardId: options.boardId }),
          temporary: true,
        }
      }
      return { url: rawMaskUrl, temporary: false }
    }
    const refinedDataUrl = await buildRefinedExecutionMaskDataUrl(
      rawMaskUrl,
      refineAmount,
      invertMask,
    )
    return {
      url: await ensureFlowMaskImageUrl(refinedDataUrl, { boardId: options.boardId }),
      temporary: true,
    }
  } catch (error) {
    const msg = error instanceof Error ? error.message : '未知错误'
    throw new Error(`处理 Mask 失败：${msg}`, { cause: error })
  }
}

export async function resolveFlowExecutionMaskUrl(
  maskNode: MaskNodeItem,
  options: ResolveFlowExecutionMaskUrlOptions = {},
): Promise<string> {
  const asset = await resolveFlowExecutionMaskAsset(maskNode, options)
  return asset.url
}

export async function cleanupFlowExecutionMaskAsset(
  asset: FlowExecutionMaskAsset | null | undefined,
  boardId?: string,
): Promise<void> {
  const normalizedBoardId = `${boardId ?? ''}`.trim()
  if (!asset?.temporary || !asset.url || !normalizedBoardId) return
  await Promise.allSettled([uploadApi.recycle(asset.url, normalizedBoardId)])
}
