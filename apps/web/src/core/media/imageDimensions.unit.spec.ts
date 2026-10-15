import { describe, expect, it, vi, afterEach } from 'vitest'
import {
  clearImageDimensionCacheForTesting,
  normalizeNaturalSize,
  readImageNaturalSize,
} from './imageDimensions'

class FailingImage {
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  naturalWidth = 0
  naturalHeight = 0
  complete = false

  set src(_value: string) {
    queueMicrotask(() => this.onerror?.())
  }
}

class LoadedImage {
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  naturalWidth = 800
  naturalHeight = 600
  complete = false

  set src(_value: string) {
    queueMicrotask(() => this.onload?.())
  }
}

describe('normalizeNaturalSize', () => {
  it('rounds valid dimensions and rejects non-positive input', () => {
    expect(normalizeNaturalSize(800.4, 600.5)).toEqual({ width: 800, height: 601 })
    expect(normalizeNaturalSize(0, 100)).toBeNull()
    expect(normalizeNaturalSize(NaN, 100)).toBeNull()
  })
})

describe('readImageNaturalSize', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    clearImageDimensionCacheForTesting()
  })

  it('resolves null when the image fails to load', async () => {
    vi.stubGlobal('Image', FailingImage)
    await expect(readImageNaturalSize('https://example.com/missing.png')).resolves.toBeNull()
  })

  it('cancels one joined consumer without cancelling another image load', async () => {
    class PendingImage extends LoadedImage {
      override set src(_value: string) {}
    }
    const image = new PendingImage()
    vi.stubGlobal(
      'Image',
      vi.fn(function () {
        return image
      }),
    )
    const firstController = new AbortController()
    const first = readImageNaturalSize(
      'https://example.com/shared.png',
      1_800,
      firstController.signal,
    )
    const second = readImageNaturalSize('https://example.com/shared.png')
    firstController.abort()
    await expect(first).resolves.toBeNull()
    image.onload?.()
    await expect(second).resolves.toEqual({ width: 800, height: 600 })
  })

  it('caches successful probes per URL', async () => {
    vi.stubGlobal('Image', LoadedImage)
    const first = await readImageNaturalSize('https://example.com/photo.png')
    expect(first).toEqual({ width: 800, height: 600 })

    // Second call hits the LRU cache without touching the Image constructor.
    const imageSpy = vi.fn()
    vi.stubGlobal('Image', imageSpy)
    await expect(readImageNaturalSize('https://example.com/photo.png')).resolves.toEqual({
      width: 800,
      height: 600,
    })
    expect(imageSpy).not.toHaveBeenCalled()
  })
})
