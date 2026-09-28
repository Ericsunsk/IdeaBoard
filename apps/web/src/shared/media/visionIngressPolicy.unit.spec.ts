import { describe, expect, it } from 'vitest'
import {
  SUPPORTED_IMAGE_UPLOAD_EXTENSIONS,
  SUPPORTED_IMAGE_UPLOAD_MIME_TYPES,
} from '../constants/media'
import {
  VISION_HEIC_EXTENSIONS,
  VISION_HEIC_MIME_TYPES,
  VISION_REFERENCE_ACCEPT,
  VISION_REFERENCE_EXTENSIONS,
  VISION_REFERENCE_MAX_BYTES,
  VISION_REFERENCE_MAX_COUNT,
  VISION_REFERENCE_MIME_TYPES,
  isVisionReferenceFile,
} from './visionIngressPolicy'

function visionFile(name: string, type: string): File {
  return new File(['bytes'], name, { type })
}

describe('visionIngressPolicy', () => {
  it('keeps the behavior-identical budgets (4 refs, 10MB)', () => {
    expect(VISION_REFERENCE_MAX_COUNT).toBe(4)
    expect(VISION_REFERENCE_MAX_BYTES).toBe(10 * 1024 * 1024)
  })

  it('sources the MIME allowlist from the shared upload constants by reference', () => {
    expect(VISION_REFERENCE_MIME_TYPES).toBe(SUPPORTED_IMAGE_UPLOAD_MIME_TYPES)
    expect(VISION_REFERENCE_EXTENSIONS).toBe(SUPPORTED_IMAGE_UPLOAD_EXTENSIONS)
    expect([...VISION_REFERENCE_MIME_TYPES]).toEqual([
      'image/jpeg',
      'image/png',
      'image/gif',
      'image/bmp',
      'image/x-ms-bmp',
      'image/webp',
      'image/avif',
      'image/heic',
      'image/heif',
    ])
  })

  it('builds a picker accept string covering every allowlisted mime and extension', () => {
    for (const mime of VISION_REFERENCE_MIME_TYPES) {
      expect(VISION_REFERENCE_ACCEPT).toContain(mime)
    }
    for (const extension of VISION_REFERENCE_EXTENSIONS) {
      expect(VISION_REFERENCE_ACCEPT).toContain(extension)
    }
  })

  it('exports the HEIC subset the pipeline converts to JPEG', () => {
    expect([...VISION_HEIC_MIME_TYPES]).toEqual(['image/heic', 'image/heif'])
    expect([...VISION_HEIC_EXTENSIONS]).toEqual(['.heic', '.heif'])
  })

  it('accepts every allowlisted mime type at the ingress gate', () => {
    for (const mime of VISION_REFERENCE_MIME_TYPES) {
      expect(isVisionReferenceFile(visionFile('photo.jpg', mime))).toBe(true)
    }
  })

  it('accepts allowlisted extensions when the mime metadata is missing', () => {
    expect(isVisionReferenceFile(visionFile('photo.jpg', ''))).toBe(true)
    expect(isVisionReferenceFile(visionFile('photo.HEIC', ''))).toBe(true)
  })

  it('rejects non-image and unsupported image shapes before upload', () => {
    expect(isVisionReferenceFile(visionFile('notes.txt', 'text/plain'))).toBe(false)
    expect(isVisionReferenceFile(visionFile('vector.svg', 'image/svg+xml'))).toBe(false)
    expect(isVisionReferenceFile(visionFile('scan.tiff', 'image/tiff'))).toBe(false)
    expect(isVisionReferenceFile(visionFile('photo', ''))).toBe(false)
  })
})
