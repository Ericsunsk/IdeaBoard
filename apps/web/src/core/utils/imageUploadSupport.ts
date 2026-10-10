import {
  IMAGE_UPLOAD_FORMAT_LABEL,
  SUPPORTED_IMAGE_UPLOAD_EXTENSIONS,
  SUPPORTED_IMAGE_UPLOAD_MIME_TYPES,
} from '../constants/media'
import { i18n } from '../i18n'

const supportedUploadImageMimeTypes = new Set<string>(SUPPORTED_IMAGE_UPLOAD_MIME_TYPES)
const supportedUploadImageExtensions = new Set<string>(SUPPORTED_IMAGE_UPLOAD_EXTENSIONS)

export function getFileExtension(fileName: string): string {
  const match = fileName
    .trim()
    .toLowerCase()
    .match(/\.[^.]+$/)
  return match?.[0] ?? ''
}

export function isSupportedUploadImageFile(file: File): boolean {
  const mimeType = file.type.trim().toLowerCase()
  if (mimeType && supportedUploadImageMimeTypes.has(mimeType)) return true
  return supportedUploadImageExtensions.has(getFileExtension(file.name))
}

export function unsupportedUploadImageMessage(): string {
  return i18n.global.t('common.errors.unsupportedImageFormat', {
    formats: IMAGE_UPLOAD_FORMAT_LABEL,
  })
}
