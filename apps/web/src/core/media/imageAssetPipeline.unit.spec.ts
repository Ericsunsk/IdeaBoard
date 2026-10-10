import { afterEach, describe, expect, it, vi } from 'vitest'
import { prepareImageAssetForUpload } from './imageAssetPipeline'

const mocks = vi.hoisted(() => ({
  heic2any: vi.fn(),
}))

vi.mock('heic2any', () => ({ default: mocks.heic2any }))

class FailingImage {
  naturalWidth = 0
  naturalHeight = 0
  width = 0
  height = 0
  onload: (() => void) | null = null
  onerror: (() => void) | null = null

  set src(_value: string) {
    queueMicrotask(() => this.onerror?.())
  }
}

const PIPELINE_MESSAGES = {
  unknownError: 'Unknown test error',
  readImageSizeSkipCompression: 'Test skipped compression',
  compressFallbackOriginal: 'Test fell back to original',
}

describe('prepareImageAssetForUpload', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    mocks.heic2any.mockReset()
  })

  it.each([
    {
      format: 'JPEG',
      bytes: [0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46],
      extension: '.jpg',
      mime: 'image/jpeg',
    },
    {
      format: 'PNG',
      bytes: [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a],
      extension: '.png',
      mime: 'image/png',
    },
    {
      format: 'GIF',
      bytes: [0x47, 0x49, 0x46, 0x38, 0x39, 0x61],
      extension: '.gif',
      mime: 'image/gif',
    },
    {
      format: 'BMP',
      bytes: [0x42, 0x4d, 0x00, 0x00],
      extension: '.bmp',
      mime: 'image/bmp',
    },
    {
      format: 'WEBP',
      bytes: [0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50],
      extension: '.webp',
      mime: 'image/webp',
    },
  ])(
    'normalizes mismatched dragged $format metadata from magic bytes before upload',
    async ({ bytes, extension, mime }) => {
      vi.stubGlobal('Image', FailingImage)
      vi.stubGlobal('URL', {
        createObjectURL: vi.fn(() => 'blob:test-image'),
        revokeObjectURL: vi.fn(),
      })

      const imageBytes = new Uint8Array(bytes)
      const draggedFile = new File([imageBytes], 'dropped-image.invalid', {
        type: 'image/png',
        lastModified: 123,
      })

      const prepared = await prepareImageAssetForUpload(draggedFile, {
        compressionPolicy: 'none',
        messages: PIPELINE_MESSAGES,
      })

      expect(prepared.originalFile).toBe(draggedFile)
      expect(prepared.uploadFile.name).toBe(`dropped-image${extension}`)
      expect(prepared.uploadFile.type).toBe(mime)
      expect(prepared.uploadFile.lastModified).toBe(123)
      expect(new Uint8Array(await prepared.uploadFile.arrayBuffer())).toEqual(imageBytes)
      // Warning copy is caller-supplied: the pipeline embeds the injected
      // messages instead of resolving feature locale keys itself.
      expect(prepared.warnings).toEqual([
        `${PIPELINE_MESSAGES.readImageSizeSkipCompression}: ${PIPELINE_MESSAGES.unknownError}`,
      ])
    },
  )

  it('converts a HEIC upload to JPEG before the normal image pipeline', async () => {
    vi.stubGlobal('Image', FailingImage)
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => 'blob:test-image'),
      revokeObjectURL: vi.fn(),
    })
    const jpegBytes = new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10])
    mocks.heic2any.mockResolvedValue(new Blob([jpegBytes], { type: 'image/jpeg' }))
    const heicBytes = new Uint8Array([
      0, 0, 0, 24, 0x66, 0x74, 0x79, 0x70, 0x68, 0x65, 0x69, 0x63, 0, 0, 0, 0,
    ])
    const heicFile = new File([heicBytes], 'phone-photo.heic', {
      type: 'image/heic',
      lastModified: 456,
    })

    const prepared = await prepareImageAssetForUpload(heicFile, {
      compressionPolicy: 'none',
      messages: PIPELINE_MESSAGES,
    })

    expect(mocks.heic2any).toHaveBeenCalledWith({
      blob: heicFile,
      toType: 'image/jpeg',
      quality: 0.9,
    })
    expect(prepared.originalFile).toBe(heicFile)
    expect(prepared.uploadFile.name).toBe('phone-photo.jpg')
    expect(prepared.uploadFile.type).toBe('image/jpeg')
    expect(prepared.uploadFile.lastModified).toBe(456)
    expect(new Uint8Array(await prepared.uploadFile.arrayBuffer())).toEqual(jpegBytes)
  })

  it('keeps AVIF bytes and normalizes stale upload metadata', async () => {
    vi.stubGlobal('Image', FailingImage)
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => 'blob:test-image'),
      revokeObjectURL: vi.fn(),
    })
    const avifBytes = new Uint8Array([0, 0, 0, 24, 0x66, 0x74, 0x79, 0x70, 0x61, 0x76, 0x69, 0x66])
    const avifFile = new File([avifBytes], 'generated-image.bin', {
      type: 'application/octet-stream',
    })

    const prepared = await prepareImageAssetForUpload(avifFile, {
      compressionPolicy: 'none',
      messages: PIPELINE_MESSAGES,
    })

    expect(prepared.uploadFile.name).toBe('generated-image.avif')
    expect(prepared.uploadFile.type).toBe('image/avif')
    expect(new Uint8Array(await prepared.uploadFile.arrayBuffer())).toEqual(avifBytes)
    expect(mocks.heic2any).not.toHaveBeenCalled()
  })

  it('uses the first converted image when a HEIF container contains multiple images', async () => {
    vi.stubGlobal('Image', FailingImage)
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => 'blob:test-image'),
      revokeObjectURL: vi.fn(),
    })
    const firstJPEG = new Blob([new Uint8Array([0xff, 0xd8, 0xff, 1])], { type: 'image/jpeg' })
    const secondJPEG = new Blob([new Uint8Array([0xff, 0xd8, 0xff, 2])], { type: 'image/jpeg' })
    mocks.heic2any.mockResolvedValue([firstJPEG, secondJPEG])
    const heifFile = new File(
      [new Uint8Array([0, 0, 0, 24, 0x66, 0x74, 0x79, 0x70, 0x6d, 0x69, 0x66, 0x31])],
      'burst.heif',
      { type: 'image/heif' },
    )

    const prepared = await prepareImageAssetForUpload(heifFile, {
      compressionPolicy: 'none',
      messages: PIPELINE_MESSAGES,
    })

    expect(prepared.uploadFile.name).toBe('burst.jpg')
    expect(new Uint8Array(await prepared.uploadFile.arrayBuffer())).toEqual(
      new Uint8Array(await firstJPEG.arrayBuffer()),
    )
  })

  it('reports a clear error when HEIC conversion fails', async () => {
    mocks.heic2any.mockRejectedValue(new Error('decoder failed'))
    const heicFile = new File([], 'broken.heic', { type: 'image/heic' })

    await expect(
      prepareImageAssetForUpload(heicFile, { messages: PIPELINE_MESSAGES }),
    ).rejects.toThrow('HEIC/HEIF image could not be converted')
  })
})
