// Shared image-upload vocabulary (no endpoint constants: the legacy
// `/admin/upload`, `/user/download`, and `/user/media-assets` routes are
// gone with the #809 hard cut; delivery URLs come from the v1 workspace
// media boundary in `api/mediaAssetDelivery`).

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
