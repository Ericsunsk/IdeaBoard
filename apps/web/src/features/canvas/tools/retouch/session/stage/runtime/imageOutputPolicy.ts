import {
  resolvePreviewMimeType,
  WEBP_PREVIEW_MIME_TYPE,
  WEBP_PREVIEW_QUALITY,
} from '../../../../../infrastructure/media/displayImagePreviewEncoding.ts'
import type {
  ImageProcessingOutputEncoding,
  ImageProcessingOutputFileNameInput,
  ImageProcessingOutputPolicy,
} from './types'

const DEFAULT_IMAGE_PROCESSING_OUTPUT_SUFFIX = 'adjusted'
const DEFAULT_IMAGE_PROCESSING_OUTPUT_BASENAME = 'image-adjusted'

export function resolveImageProcessingOutputEncoding(
  input: Parameters<ImageProcessingOutputPolicy['resolveEncoding']>[0],
): ImageProcessingOutputEncoding {
  const mimeType = resolvePreviewMimeType(input.sourceMimeType, input.hasTransparentPixels)
  return {
    mimeType,
    ...(mimeType === WEBP_PREVIEW_MIME_TYPE ? { quality: WEBP_PREVIEW_QUALITY } : {}),
  }
}

export function resolveImageExportEncoding(
  sourceMimeType: string,
  hasTransparentPixels: boolean | null,
): ImageProcessingOutputEncoding {
  const normalized = sourceMimeType.trim().toLowerCase().split(';', 1)[0]
  if (hasTransparentPixels && normalized === 'image/png') return { mimeType: 'image/png' }
  if (normalized === 'image/jpeg' || normalized === 'image/jpg') {
    return { mimeType: 'image/jpeg', quality: 0.92 }
  }
  if (normalized === 'image/png') return { mimeType: 'image/png' }
  return { mimeType: 'image/webp', quality: 0.92 }
}

export function createImageProcessingOutputFileName(input: ImageProcessingOutputFileNameInput) {
  const suffix = normalizeFileNamePart(input.suffix ?? DEFAULT_IMAGE_PROCESSING_OUTPUT_SUFFIX)
  const fallbackBaseName = normalizeFileNamePart(
    input.fallbackBaseName ?? DEFAULT_IMAGE_PROCESSING_OUTPUT_BASENAME,
  )
  const baseName = normalizeFileNamePart(
    `${input.title ?? fallbackBaseName}`.trim().replace(/\.[A-Za-z0-9]{1,10}$/, ''),
  )
  const extension = resolveImageProcessingOutputExtension(input.mimeType)
  const resolvedBaseName = baseName || fallbackBaseName || DEFAULT_IMAGE_PROCESSING_OUTPUT_BASENAME
  return suffix ? `${resolvedBaseName}-${suffix}.${extension}` : `${resolvedBaseName}.${extension}`
}

const imageProcessingOutputPolicy: ImageProcessingOutputPolicy = {
  resolveEncoding: resolveImageProcessingOutputEncoding,
  resolveFileName: createImageProcessingOutputFileName,
}

export function createImageProcessingOutputPolicy(): ImageProcessingOutputPolicy {
  return imageProcessingOutputPolicy
}

function resolveImageProcessingOutputExtension(mimeType: string) {
  switch (mimeType.trim().toLowerCase()) {
    case 'image/png':
      return 'png'
    case 'image/jpeg':
    case 'image/jpg':
      return 'jpg'
    case 'image/webp':
    default:
      return 'webp'
  }
}

function normalizeFileNamePart(value: string) {
  return value.replace(/[^\p{L}\p{N}._-]+/gu, '-').replace(/^-+|-+$/g, '')
}
