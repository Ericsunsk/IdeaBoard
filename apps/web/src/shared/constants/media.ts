import { resolveApiUrl } from '../config/runtimeUrls'

export const ADMIN_UPLOAD_ENDPOINT = resolveApiUrl('/admin/upload')
export const USER_DOWNLOAD_ENDPOINT = resolveApiUrl('/user/download')
export const MEDIA_ASSET_ENDPOINT = resolveApiUrl('/user/media-assets')

export const SUPPORTED_IMAGE_UPLOAD_MIME_TYPES = [
  'image/jpeg',
  'image/png',
  'image/gif',
  'image/bmp',
  'image/x-ms-bmp',
  'image/webp',
  'image/avif',
  'image/heic',
  'image/heif',
] as const
export const SUPPORTED_IMAGE_UPLOAD_EXTENSIONS = [
  '.jpg',
  '.jpeg',
  '.png',
  '.gif',
  '.bmp',
  '.webp',
  '.avif',
  '.heic',
  '.heif',
] as const
export const IMAGE_UPLOAD_ACCEPT = [
  ...SUPPORTED_IMAGE_UPLOAD_MIME_TYPES,
  ...SUPPORTED_IMAGE_UPLOAD_EXTENSIONS,
].join(',')
export const IMAGE_UPLOAD_FORMAT_LABEL = 'JPG / PNG / GIF / BMP / WEBP / AVIF / HEIC / HEIF'

export const DEFAULT_IMAGE_DOWNLOAD_NAME = 'image.png'
