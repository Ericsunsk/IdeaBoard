import { afterEach, describe, expect, it, vi } from 'vitest'
import { ImageSourceCache } from './imageSourceCache'

const fetchMock = vi.fn()
const createImageBitmapMock = vi.fn()

afterEach(() => {
  vi.restoreAllMocks()
  fetchMock.mockReset()
  createImageBitmapMock.mockReset()
})

function installCanvasMocks() {
  const contexts: Array<{
    drawImage: ReturnType<typeof vi.fn>
    getImageData: ReturnType<typeof vi.fn>
  }> = []

  vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') {
      return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
    }

    const canvas = document.createElementNS(
      'http://www.w3.org/1999/xhtml',
      'canvas',
    ) as HTMLCanvasElement
    const context = {
      drawImage: vi.fn(),
      getImageData: vi.fn(() => ({ data: new Uint8ClampedArray([1, 2, 3, 255]) })),
    }
    Object.defineProperty(canvas, 'getContext', {
      value: () => context,
    })
    contexts.push(context)
    return canvas
  })

  return { contexts }
}

function installImageMocks(width = 4, height = 2) {
  fetchMock.mockResolvedValue({
    ok: true,
    blob: async () => new Blob(['source'], { type: 'image/png' }),
  })
  createImageBitmapMock.mockResolvedValue({
    width,
    height,
    close: vi.fn(),
  })
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('createImageBitmap', createImageBitmapMock)
}

describe('ImageSourceCache', () => {
  it('reuses cached full-size sources without fetching or decoding twice', async () => {
    const { contexts } = installCanvasMocks()
    installImageMocks()
    const cache = new ImageSourceCache({ maxEntries: 2 })

    const first = await cache.getFullSizeSource('https://example.com/source.png')
    const second = await cache.getFullSizeSource('https://example.com/source.png')

    expect(second).toBe(first)
    expect(first?.width).toBe(4)
    expect(first?.height).toBe(2)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(createImageBitmapMock).toHaveBeenCalledTimes(1)
    expect(contexts[0]?.drawImage).toHaveBeenCalledWith(
      expect.objectContaining({ width: 4, height: 2 }),
      0,
      0,
      4,
      2,
    )
  })

  it('trims least-recently-used canvases when the cache reaches its limit', async () => {
    installCanvasMocks()
    installImageMocks()
    const cache = new ImageSourceCache({ maxEntries: 2 })

    const first = await cache.getFullSizeSource('https://example.com/a.png')
    const second = await cache.getFullSizeSource('https://example.com/b.png')
    await cache.getFullSizeSource('https://example.com/a.png')
    const third = await cache.getFullSizeSource('https://example.com/c.png')

    expect(cache.size).toBe(2)
    expect(second?.canvas.width).toBe(0)
    expect(second?.canvas.height).toBe(0)
    expect(first?.canvas.width).toBeGreaterThan(0)
    expect(third?.canvas.width).toBeGreaterThan(0)
  })

  it('rejects animated images before decoding', async () => {
    installCanvasMocks()
    fetchMock.mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['source'], { type: 'image/gif' }),
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('createImageBitmap', createImageBitmapMock)

    const cache = new ImageSourceCache()

    await expect(cache.getFullSizeSource('https://example.com/animated.gif')).rejects.toMatchObject(
      {
        code: 'animated-image',
        message: '暂不支持编辑动图',
      },
    )
    expect(createImageBitmapMock).not.toHaveBeenCalled()
  })

  it('releases cached canvases on explicit release and dispose', async () => {
    installCanvasMocks()
    installImageMocks()
    const cache = new ImageSourceCache({ maxEntries: 2 })

    const released = await cache.getFullSizeSource('https://example.com/full.png')
    const disposed = await cache.getFullSizeSource('https://example.com/disposed.png')

    expect(released?.canvas.width).toBe(4)
    cache.releaseSource(released)
    expect(cache.size).toBe(1)
    expect(released?.canvas.width).toBe(0)
    expect(released?.canvas.height).toBe(0)

    cache.dispose()
    expect(cache.size).toBe(0)
    expect(disposed?.canvas.width).toBe(0)
    expect(disposed?.canvas.height).toBe(0)
  })
})
