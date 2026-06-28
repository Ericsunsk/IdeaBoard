import {
  resolvePreviewMimeType,
  WEBP_PREVIEW_MIME_TYPE,
  WEBP_PREVIEW_QUALITY,
} from '../../utils/displayImagePreviewEncoding'
import type {
  ImageProcessingOutputEncoding,
  ImageProcessingOutputFileNameInput,
  ImageProcessingOutputPolicy,
  ImageProcessingOutputPolicyInput,
} from './types'

export const DEFAULT_IMAGE_PROCESSING_OUTPUT_SUFFIX = 'adjusted'
export const DEFAULT_IMAGE_PROCESSING_OUTPUT_BASENAME = 'image-adjusted'

export function resolveImageProcessingOutputEncoding(
  input: ImageProcessingOutputPolicyInput,
): ImageProcessingOutputEncoding {
  const mimeType = resolvePreviewMimeType(input.sourceMimeType, input.hasTransparentPixels)
  return {
    mimeType,
    ...(mimeType === WEBP_PREVIEW_MIME_TYPE ? { quality: WEBP_PREVIEW_QUALITY } : {}),
  }
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

export const imageProcessingOutputPolicy: ImageProcessingOutputPolicy = {
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
