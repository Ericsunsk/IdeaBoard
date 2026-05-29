import { resolveApiUrl } from '../config/runtimeUrls'

export const ADMIN_UPLOAD_ENDPOINT = resolveApiUrl('/admin/upload')
export const USER_DOWNLOAD_ENDPOINT = resolveApiUrl('/user/download')

export const SUPPORTED_IMAGE_UPLOAD_MIME_TYPES = [
  'image/jpeg',
  'image/png',
  'image/gif',
  'image/bmp',
  'image/x-ms-bmp',
  'image/webp',
] as const
export const SUPPORTED_IMAGE_UPLOAD_EXTENSIONS = [
  '.jpg',
  '.jpeg',
  '.png',
  '.gif',
  '.bmp',
  '.webp',
] as const
export const IMAGE_UPLOAD_ACCEPT = [
  ...SUPPORTED_IMAGE_UPLOAD_MIME_TYPES,
  ...SUPPORTED_IMAGE_UPLOAD_EXTENSIONS,
].join(',')
export const IMAGE_UPLOAD_FORMAT_LABEL = 'JPG / PNG / GIF / BMP / WEBP'

export const DEFAULT_IMAGE_DOWNLOAD_NAME = 'image.png'
export const UPLOAD_SUCCESS_MESSAGE = '上传成功'
export const UPLOAD_FAILED_MESSAGE = '上传失败'
