import {
  SUPPORTED_IMAGE_UPLOAD_EXTENSIONS,
  SUPPORTED_IMAGE_UPLOAD_MIME_TYPES,
} from '../constants/media'
import { isSupportedUploadImageFile } from '../utils/imageUploadSupport'

/**
 * Vision reference ingress policy: the single client-side gate for vision
 * reference intake (search `SearchPage` picker, `useVisionQueryLifecycle`
 * checks, `visionReferencePolicy` budgets, and the shared image pipeline's
 * HEIC/normalize/compress path).
 *
 * The MIME allowlist is the shared upload allowlist by reference (not a
 * copy), so the picker, the lifecycle gate, and the pipeline can never
 * drift. Server remains authoritative: these gates only fail fast with
 * local copy; the API re-validates every byte.
 */

export const VISION_REFERENCE_MAX_COUNT = 4
export const VISION_REFERENCE_MAX_BYTES = 10 * 1024 * 1024

export const VISION_REFERENCE_MIME_TYPES = SUPPORTED_IMAGE_UPLOAD_MIME_TYPES
export const VISION_REFERENCE_EXTENSIONS = SUPPORTED_IMAGE_UPLOAD_EXTENSIONS

export const VISION_REFERENCE_ACCEPT = [
  ...VISION_REFERENCE_MIME_TYPES,
  ...VISION_REFERENCE_EXTENSIONS,
].join(',')

/** HEIC subset of the allowlist: sources the pipeline converts to JPEG. */
export const VISION_HEIC_MIME_TYPES: readonly string[] = ['image/heic', 'image/heif']
export const VISION_HEIC_EXTENSIONS: readonly string[] = ['.heic', '.heif']

/** Client-side ingress predicate: mime allowlist hit or allowlisted extension. */
export function isVisionReferenceFile(file: File): boolean {
  return isSupportedUploadImageFile(file)
}
