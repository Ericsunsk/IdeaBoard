import Compressor from 'compressorjs'
import { getFileExtension, isSupportedUploadImageFile } from '../utils/imageUploadSupport'
import { userFacingErrorCopy } from '../errors/errorCatalog'
import { i18n } from '../i18n'

export type ImageAssetCompressionPolicy = 'default' | 'none'

/**
 * Caller-supplied pipeline copy (issue #835). The pipeline stays
 * feature-free: resolution warnings embed these strings instead of
 * resolving feature locale keys itself.
 */
export type ImageAssetPipelineMessages = {
  unknownError: string
  readImageSizeSkipCompression: string
  compressFallbackOriginal: string
}

export interface PrepareImageAssetOptions {
  compressionPolicy?: ImageAssetCompressionPolicy
  messages: ImageAssetPipelineMessages
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

/**
 * Single source of truth for upload image mime/extension mapping.
 * Mirrors `SUPPORTED_IMAGE_UPLOAD_*` in `shared/constants/media`
 * (`image/x-ms-bmp` is a Windows alias of BMP with no canonical extension).
 */
const IMAGE_FORMAT_TABLE = [
  { mime: 'image/jpeg', extensions: ['.jpg', '.jpeg'] },
  { mime: 'image/png', extensions: ['.png'] },
  { mime: 'image/gif', extensions: ['.gif'] },
  { mime: 'image/webp', extensions: ['.webp'] },
  { mime: 'image/bmp', extensions: ['.bmp'] },
  { mime: 'image/x-ms-bmp', extensions: [] as string[] },
  { mime: 'image/avif', extensions: ['.avif'] },
  { mime: 'image/heic', extensions: ['.heic'] },
  { mime: 'image/heif', extensions: ['.heif'] },
] as const

const IMAGE_MIME_BY_EXTENSION: Record<string, string> = Object.fromEntries(
  IMAGE_FORMAT_TABLE.flatMap((format) =>
    format.extensions.map((extension) => [extension, format.mime]),
  ),
)

const IMAGE_EXTENSION_BY_MIME: Record<string, string> = Object.fromEntries(
  IMAGE_FORMAT_TABLE.filter((format) => format.extensions.length > 0).map((format) => [
    format.mime,
    format.extensions[0],
  ]),
)
// `image/x-ms-bmp` uploads normalize to the canonical BMP extension.
IMAGE_EXTENSION_BY_MIME['image/x-ms-bmp'] = '.bmp'

type DetectedImageFormat = {
  mime: string
  extension: string
  compatibleExtensions: readonly string[]
  requiresJPEGConversion?: boolean
}

const HEIC_UPLOAD_MIME_TYPES = new Set(['image/heic', 'image/heif'])
const HEIC_UPLOAD_EXTENSIONS = new Set(['.heic', '.heif'])

function hasBytes(data: Uint8Array, offset: number, expected: readonly number[]): boolean {
  if (data.length < offset + expected.length) return false
  return expected.every((value, index) => data[offset + index] === value)
}

function detectImageFormat(data: Uint8Array): DetectedImageFormat | null {
  if (hasBytes(data, 0, [0xff, 0xd8, 0xff])) {
    return { mime: 'image/jpeg', extension: '.jpg', compatibleExtensions: ['.jpg', '.jpeg'] }
  }
  if (hasBytes(data, 0, [0x89, 0x50, 0x4e, 0x47])) {
    return { mime: 'image/png', extension: '.png', compatibleExtensions: ['.png'] }
  }
  if (
    hasBytes(data, 0, [0x47, 0x49, 0x46, 0x38, 0x37, 0x61]) ||
    hasBytes(data, 0, [0x47, 0x49, 0x46, 0x38, 0x39, 0x61])
  ) {
    return { mime: 'image/gif', extension: '.gif', compatibleExtensions: ['.gif'] }
  }
  if (hasBytes(data, 0, [0x42, 0x4d])) {
    return { mime: 'image/bmp', extension: '.bmp', compatibleExtensions: ['.bmp'] }
  }
  if (hasBytes(data, 0, [0x52, 0x49, 0x46, 0x46]) && hasBytes(data, 8, [0x57, 0x45, 0x42, 0x50])) {
    return { mime: 'image/webp', extension: '.webp', compatibleExtensions: ['.webp'] }
  }
  if (hasBytes(data, 4, [0x66, 0x74, 0x79, 0x70])) {
    const brand = String.fromCharCode(...data.slice(8, 12))
    if (brand === 'avif') {
      return { mime: 'image/avif', extension: '.avif', compatibleExtensions: ['.avif'] }
    }
    if (['heic', 'heix', 'hevc', 'hevx'].includes(brand)) {
      return {
        mime: 'image/heic',
        extension: '.heic',
        compatibleExtensions: ['.heic'],
        requiresJPEGConversion: true,
      }
    }
    if (['mif1', 'msf1'].includes(brand)) {
      return {
        mime: 'image/heif',
        extension: '.heif',
        compatibleExtensions: ['.heif'],
        requiresJPEGConversion: true,
      }
    }
  }
  return null
}

function replaceFileExtension(fileName: string, extension: string): string {
  const trimmed = fileName.trim() || `image-${Date.now()}`
  return /\.[^.]+$/.test(trimmed)
    ? trimmed.replace(/\.[^.]+$/, extension)
    : `${trimmed}${extension}`
}

async function normalizeImageUploadFile(file: File): Promise<File> {
  // Dragged and fetched Blobs can carry stale name/type metadata, while the API
  // intentionally validates the filename extension against the actual magic bytes.
  let format: DetectedImageFormat | null
  try {
    const header = new Uint8Array(await file.slice(0, 12).arrayBuffer())
    format = detectImageFormat(header)
  } catch {
    return file
  }
  if (format?.requiresJPEGConversion || (!format && isHEICUpload(file))) {
    return convertHEICUploadToJPEG(file)
  }
  if (!format) return file

  const currentExtension = getFileExtension(file.name)
  const normalizedName = format.compatibleExtensions.includes(currentExtension)
    ? file.name
    : replaceFileExtension(file.name, format.extension)
  if (normalizedName === file.name && normalizeMime(file.type) === format.mime) return file

  return new File([file], normalizedName, {
    type: format.mime,
    lastModified: file.lastModified,
  })
}

function isHEICUpload(file: File): boolean {
  return (
    HEIC_UPLOAD_MIME_TYPES.has(normalizeMime(file.type)) ||
    HEIC_UPLOAD_EXTENSIONS.has(getFileExtension(file.name))
  )
}

async function convertHEICUploadToJPEG(file: File): Promise<File> {
  try {
    const { default: heic2any } = await import('heic2any')
    const converted = await heic2any({
      blob: file,
      toType: 'image/jpeg',
      quality: 0.9,
    })
    const jpegBlob = Array.isArray(converted) ? converted[0] : converted
    if (!(jpegBlob instanceof Blob)) {
      throw new Error(i18n.global.t('common.errors.heicConversionFailed'))
    }
    return new File([jpegBlob], replaceFileExtension(file.name, '.jpg'), {
      type: 'image/jpeg',
      lastModified: file.lastModified,
    })
  } catch (error) {
    if (
      error instanceof Error &&
      error.message === i18n.global.t('common.errors.heicConversionFailed')
    ) {
      throw error
    }
    throw new Error(i18n.global.t('common.errors.heicConversionFailed'), { cause: error })
  }
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
        reject(new Error(i18n.global.t('common.errors.readImageSizeFailed')))
        return
      }
      resolve({ width, height })
    }
    image.onerror = () => {
      URL.revokeObjectURL(objectUrl)
      reject(new Error(i18n.global.t('common.errors.parseImageFailed')))
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
          reject(new Error(i18n.global.t('common.errors.compressInvalidOutput')))
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
  originalFile: File,
  uploadFile: File,
  warnings: string[],
  dimensions?: { width: number; height: number },
): PreparedImageAsset {
  const ratio = dimensions ? dimensions.height / dimensions.width : 1
  return {
    originalFile,
    uploadFile,
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
  options: PrepareImageAssetOptions,
): Promise<PreparedImageAsset> {
  const uploadSourceFile = await normalizeImageUploadFile(file)
  if (!isSupportedUploadImageFile(uploadSourceFile)) {
    throw new Error(i18n.global.t('common.errors.unsupportedImageFormat'))
  }
  const warnings: string[] = []
  let dimensions: { width: number; height: number } | undefined
  try {
    dimensions = await readImageDimensions(uploadSourceFile)
  } catch (error) {
    const reason = userFacingErrorCopy(error, options.messages.unknownError)
    warnings.push(`${options.messages.readImageSizeSkipCompression}: ${reason}`)
    return buildUncompressedAsset(file, uploadSourceFile, warnings)
  }
  const compressionPolicy = options.compressionPolicy ?? 'default'

  if (
    compressionPolicy === 'none' ||
    !shouldCompress(uploadSourceFile, dimensions.width, dimensions.height)
  ) {
    return buildUncompressedAsset(file, uploadSourceFile, warnings, dimensions)
  }

  try {
    const compressedFile = await compressImageFile(uploadSourceFile)
    const compressedDimensions = await readImageDimensions(compressedFile).catch(() => dimensions)
    const ratio = compressedDimensions.height / compressedDimensions.width
    const reducedDimensions =
      Math.max(compressedDimensions.width, compressedDimensions.height) <
      Math.max(dimensions.width, dimensions.height)
    if (compressedFile.size >= uploadSourceFile.size && !reducedDimensions) {
      return buildUncompressedAsset(file, uploadSourceFile, warnings, dimensions)
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
    const reason = userFacingErrorCopy(error, options.messages.unknownError)
    warnings.push(`${options.messages.compressFallbackOriginal}: ${reason}`)
    return buildUncompressedAsset(file, uploadSourceFile, warnings, dimensions)
  }
}

export { IMAGE_ASSET_PIPELINE_CONFIG }
