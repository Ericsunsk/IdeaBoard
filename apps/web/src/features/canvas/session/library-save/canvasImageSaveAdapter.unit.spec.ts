import { describe, expect, it, vi } from 'vitest'
import { prepareImageAssetForUpload } from '../../../../core/media/imageAssetPipeline'
import { createCanvasImageSaveInputs } from './canvasImageSaveAdapter'

vi.mock('../../../../core/media/imageAssetPipeline', () => ({
  prepareImageAssetForUpload: vi.fn(),
}))

const prepareMock = vi.mocked(prepareImageAssetForUpload)

describe('createCanvasImageSaveInputs', () => {
  it('fetches display bytes into prepared upload files', async () => {
    const fetchBytes = vi.fn(async () => new Blob(['pixels'], { type: 'image/png' }))
    const uploadFile = new File(['pixels'], 'a.png', { type: 'image/png' })
    prepareMock.mockResolvedValue({
      originalFile: uploadFile,
      uploadFile,
      width: 800,
      height: 600,
      ratio: 0.75,
      isDimensionsKnown: true,
      warnings: [],
      isCompressed: false,
    })

    const inputs = createCanvasImageSaveInputs(
      [{ displayUrl: 'https://example.com/a.png', title: 'Hero' }],
      { fetchBytes },
    )

    expect(fetchBytes).not.toHaveBeenCalled()
    const input = inputs[0]
    if (input?.source !== 'media' || !('file' in input) || typeof input.file !== 'function') {
      throw new Error('Expected deferred image preparation')
    }
    expect(await input.file()).toBe(uploadFile)
    expect(fetchBytes).toHaveBeenCalledWith('https://example.com/a.png')
    expect(prepareMock).toHaveBeenCalledOnce()
    expect(input.title).toBe('Hero')
    expect(input).not.toHaveProperty('width')
  })

  it('normalizes uploads without tracking dimensions', async () => {
    const fetchBytes = vi.fn(async () => new Blob(['pixels'], { type: 'image/png' }))
    const uploadFile = new File(['pixels'], 'a.png', { type: 'image/png' })
    prepareMock.mockResolvedValue({
      originalFile: uploadFile,
      uploadFile,
      width: 0,
      height: 0,
      ratio: 1,
      isDimensionsKnown: false,
      warnings: ['skipped'],
      isCompressed: false,
    })

    const inputs = createCanvasImageSaveInputs([{ displayUrl: 'https://example.com/a.png' }], {
      fetchBytes,
    })

    const input = inputs[0]
    if (input?.source !== 'media' || !('file' in input) || typeof input.file !== 'function') {
      throw new Error('Expected deferred image preparation')
    }
    expect(await input.file()).toBe(uploadFile)
    expect(input).not.toHaveProperty('width')
  })

  it('keeps failed preparations as retryable inputs instead of filtering the batch', async () => {
    const fetchBytes = vi.fn(async (url: string) => {
      if (url.includes('missing')) throw new Error('gone')
      return new Blob(['pixels'], { type: 'image/png' })
    })
    const uploadFile = new File(['pixels'], 'a.png', { type: 'image/png' })
    prepareMock.mockResolvedValue({
      originalFile: uploadFile,
      uploadFile,
      width: 100,
      height: 100,
      ratio: 1,
      isDimensionsKnown: true,
      warnings: [],
      isCompressed: false,
    })

    const inputs = createCanvasImageSaveInputs(
      [
        { displayUrl: 'https://example.com/a.png' },
        { displayUrl: 'https://example.com/missing.png' },
      ],
      { fetchBytes },
    )

    expect(inputs).toHaveLength(2)
    const results = await Promise.allSettled(
      inputs.map((input) => {
        if (input.source !== 'media' || !('file' in input) || typeof input.file !== 'function') {
          throw new Error('Expected deferred image preparation')
        }
        return input.file()
      }),
    )
    expect(results.map((result) => result.status)).toEqual(['fulfilled', 'rejected'])
  })

  it('preserves managed identity without fetching display renditions', () => {
    const fetchBytes = vi.fn()
    const inputs = createCanvasImageSaveInputs(
      [
        {
          displayUrl: '/derivatives/preview',
          mediaAssetId: 'asset_managed001',
          title: 'Original',
        },
      ],
      { fetchBytes },
    )
    expect(inputs).toEqual([
      { source: 'media', mediaAssetId: 'asset_managed001', title: 'Original' },
    ])
    expect(fetchBytes).not.toHaveBeenCalled()
  })
})
