import Compressor from 'compressorjs'
import { getFileExtension, isSupportedUploadImageFile } from '../utils/imageUploadSupport'

export type ImageAssetCompressionPolicy = 'default' | 'none'

export interface PrepareImageAssetOptions {
  compressionPolicy?: ImageAssetCompressionPolicy
}

export interface PreparedImageAsset {
  originalFile: File
  uploadFile: File
  width: number
  height: number
  ratio: number
  dimensionsKnown: boolean
  warnings: string[]
  compressed: boolean
}

const IMAGE_ASSET_PIPELINE_CONFIG = {
  maxWidth: 2048,
  maxHeight: 2048,
  quality: 0.82,
  minCompressBytes: 512 * 1024,
  minCompressLongSide: 1600,
  convertSize: Number.POSITIVE_INFINITY,
} as const

const COMPRESSIBLE_IMAGE_MIME_TYPES = new Set([
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/bmp',
  'image/x-ms-bmp',
])

const IMAGE_MIME_BY_EXTENSION: Record<string, string> = {
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.png': 'image/png',
  '.webp': 'image/webp',
  '.bmp': 'image/bmp',
}

const IMAGE_EXTENSION_BY_MIME: Record<string, string> = {
  'image/jpeg': '.jpg',
  'image/png': '.png',
  'image/webp': '.webp',
  'image/bmp': '.bmp',
  'image/x-ms-bmp': '.bmp',
}

function normalizeMime(mime: string): string {
  return mime.trim().toLowerCase()
}

function resolveInputMimeType(file: File): string {
  const mime = normalizeMime(file.type)
  if (mime) return mime
  return IMAGE_MIME_BY_EXTENSION[getFileExtension(file.name)] ?? ''
}

function resolveOutputMimeType(file: File): string {
  const normalized = resolveInputMimeType(file)
  if (normalized === 'image/jpeg') return 'image/jpeg'
  if (normalized === 'image/webp') return 'image/webp'
  if (normalized === 'image/png' || normalized === 'image/bmp' || normalized === 'image/x-ms-bmp') {
    return 'image/png'
  }
  return normalized || 'image/jpeg'
}

function resolveUploadFileName(fileName: string, mimeType: string): string {
  const ext = IMAGE_EXTENSION_BY_MIME[normalizeMime(mimeType)]
  if (!ext) return fileName
  const trimmed = fileName.trim() || `image-${Date.now()}${ext}`
  return /\.[^.]+$/.test(trimmed) ? trimmed.replace(/\.[^.]+$/, ext) : `${trimmed}${ext}`
}

function readImageDimensions(file: File): Promise<{ width: number; height: number }> {
  const objectUrl = URL.createObjectURL(file)
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => {
      const width = Math.max(1, Math.round(image.naturalWidth || image.width || 0))
      const height = Math.max(1, Math.round(image.naturalHeight || image.height || 0))
      URL.revokeObjectURL(objectUrl)
      if (!width || !height) {
        reject(new Error('无法读取图片尺寸'))
        return
      }
      resolve({ width, height })
    }
    image.onerror = () => {
      URL.revokeObjectURL(objectUrl)
      reject(new Error('无法解析图片内容'))
    }
    image.src = objectUrl
  })
}

function compressImageFile(file: File): Promise<File> {
  return new Promise((resolve, reject) => {
    const mimeType = resolveOutputMimeType(file)
    const compressor = new Compressor(file, {
      maxWidth: IMAGE_ASSET_PIPELINE_CONFIG.maxWidth,
      maxHeight: IMAGE_ASSET_PIPELINE_CONFIG.maxHeight,
      quality: IMAGE_ASSET_PIPELINE_CONFIG.quality,
      convertSize: IMAGE_ASSET_PIPELINE_CONFIG.convertSize,
      mimeType,
      success: (result) => {
        if (!(result instanceof Blob)) {
          reject(new Error('压缩失败：无效输出'))
          return
        }
        const outputMimeType = normalizeMime(result.type || mimeType) || mimeType
        resolve(
          new File([result], resolveUploadFileName(file.name, outputMimeType), {
            type: outputMimeType,
            lastModified: file.lastModified,
          }),
        )
      },
      error: (error) => reject(error),
    })
    void compressor
  })
}

function shouldCompress(file: File, width: number, height: number): boolean {
  const mime = resolveInputMimeType(file)
  if (!COMPRESSIBLE_IMAGE_MIME_TYPES.has(mime)) return false
  const longSide = Math.max(width, height)
  return (
    file.size >= IMAGE_ASSET_PIPELINE_CONFIG.minCompressBytes ||
    longSide >= IMAGE_ASSET_PIPELINE_CONFIG.minCompressLongSide
  )
}

function buildUncompressedAsset(
  file: File,
  warnings: string[],
  dimensions?: { width: number; height: number },
): PreparedImageAsset {
  const ratio = dimensions ? dimensions.height / dimensions.width : 1
  return {
    originalFile: file,
    uploadFile: file,
    width: dimensions?.width ?? 0,
    height: dimensions?.height ?? 0,
    ratio: ratio > 0 ? ratio : 1,
    dimensionsKnown: !!dimensions,
    warnings,
    compressed: false,
  }
}

export async function prepareImageAssetForUpload(
  file: File,
  options: PrepareImageAssetOptions = {},
): Promise<PreparedImageAsset> {
  if (!isSupportedUploadImageFile(file)) {
    throw new Error('不支持的图片格式')
  }
  const warnings: string[] = []
  let dimensions: { width: number; height: number } | undefined
  try {
    dimensions = await readImageDimensions(file)
  } catch (error) {
    const reason = error instanceof Error ? error.message : '未知错误'
    warnings.push(`无法读取图片尺寸，已跳过上传前压缩：${reason}`)
    return buildUncompressedAsset(file, warnings)
  }
  const compressionPolicy = options.compressionPolicy ?? 'default'

  if (compressionPolicy === 'none' || !shouldCompress(file, dimensions.width, dimensions.height)) {
    return buildUncompressedAsset(file, warnings, dimensions)
  }

  try {
    const compressedFile = await compressImageFile(file)
    const compressedDimensions = await readImageDimensions(compressedFile).catch(() => dimensions)
    const ratio = compressedDimensions.height / compressedDimensions.width
    const reducedDimensions =
      Math.max(compressedDimensions.width, compressedDimensions.height) <
      Math.max(dimensions.width, dimensions.height)
    if (compressedFile.size >= file.size && !reducedDimensions) {
      return buildUncompressedAsset(file, warnings, dimensions)
    }
    return {
      originalFile: file,
      uploadFile: compressedFile,
      width: compressedDimensions.width,
      height: compressedDimensions.height,
      ratio: ratio > 0 ? ratio : 1,
      dimensionsKnown: true,
      warnings,
      compressed: true,
    }
  } catch (error) {
    const reason = error instanceof Error ? error.message : '未知错误'
    warnings.push(`图片压缩失败，已降级原图上传：${reason}`)
    return buildUncompressedAsset(file, warnings, dimensions)
  }
}

export { IMAGE_ASSET_PIPELINE_CONFIG }
