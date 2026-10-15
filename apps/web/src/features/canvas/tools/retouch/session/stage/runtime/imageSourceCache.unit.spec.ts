import { afterEach, describe, expect, it, test, vi } from 'vitest'
import { createDeferred } from '../../../../../../../test/deferred'
import { ImageSourceCache } from './imageSourceCache'

const fetchMock = vi.fn()
const createImageBitmapMock = vi.fn()

afterEach(() => {
  delete window.__canvasInteractionBenchmarkCounters
  vi.restoreAllMocks()
  fetchMock.mockReset()
  createImageBitmapMock.mockReset()
})

function installCanvasMocks() {
  const canvases: HTMLCanvasElement[] = []
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
    canvases.push(canvas)
    contexts.push(context)
    return canvas
  })

  return { canvases, contexts }
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
    window.__canvasInteractionBenchmarkCounters = {}
    const { contexts } = installCanvasMocks()
    installImageMocks()
    const cache = new ImageSourceCache({ maxEntries: 2 })

    const first = await cache.getFullSizeSource('https://example.com/source.png')
    const second = await cache.getFullSizeSource('https://example.com/source.png')

    expect(second).toBe(first)
    expect(window.__canvasInteractionBenchmarkCounters.retouchSourceCacheMiss).toBe(1)
    expect(window.__canvasInteractionBenchmarkCounters.retouchSourceCacheHit).toBe(1)
    expect(
      window.__canvasInteractionBenchmarkCounters.runtimeTimingMs?.retouchSourceLoad?.count,
    ).toBe(1)
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

  test.each(['replace', 'delete-reinstall', 'enable-later'])(
    'isolates deferred source-load timing after %s',
    async (reset) => {
      if (reset !== 'enable-later') window.__canvasInteractionBenchmarkCounters = {}
      installCanvasMocks()
      installImageMocks()
      const decoding = createDeferred<{ width: number; height: number; close: () => void }>()
      createImageBitmapMock.mockReturnValueOnce(decoding.promise)
      const cache = new ImageSourceCache({ maxEntries: 2 })
      const loading = cache.getFullSizeSource('https://example.com/deferred.png')
      await waitForImageBitmapRequest()
      if (reset === 'delete-reinstall') delete window.__canvasInteractionBenchmarkCounters
      const replacement = (window.__canvasInteractionBenchmarkCounters = {})
      const bitmap = { width: 4, height: 2, close: vi.fn() }
      decoding.resolve(bitmap)
      expect(await loading).toMatchObject({ width: 4, height: 2 })
      expect(bitmap.close).toHaveBeenCalledTimes(1)
      expect(replacement).toEqual({})

      await cache.getFullSizeSource('https://example.com/new-collection.png')
      expect(
        window.__canvasInteractionBenchmarkCounters.runtimeTimingMs?.retouchSourceLoad?.count,
      ).toBe(1)
      cache.dispose()
    },
  )

  it('resizes maxSide sources and caches them separately from full-size sources', async () => {
    const { contexts } = installCanvasMocks()
    installImageMocks(4000, 2000)
    const cache = new ImageSourceCache({ maxEntries: 3 })

    const preview = await cache.getFullSizeSource('https://example.com/source.png', {
      maxSide: 1000,
    })
    const samePreview = await cache.getFullSizeSource('https://example.com/source.png', {
      maxSide: 1000,
    })
    const fullSize = await cache.getFullSizeSource('https://example.com/source.png')

    expect(samePreview).toBe(preview)
    expect(preview).toMatchObject({ width: 1000, height: 500 })
    expect(fullSize).toMatchObject({ width: 4000, height: 2000 })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(createImageBitmapMock).toHaveBeenCalledTimes(2)
    expect(contexts[0]?.drawImage).toHaveBeenCalledWith(
      expect.objectContaining({ width: 4000, height: 2000 }),
      0,
      0,
      1000,
      500,
    )
  })

  it('allows maxSide previews for images whose original pixels exceed the full-size limit', async () => {
    installCanvasMocks()
    installImageMocks(8000, 4000)
    const cache = new ImageSourceCache({ maxEntries: 2 })

    const preview = await cache.getFullSizeSource('https://example.com/large.png', {
      maxSide: 1000,
    })

    expect(preview).toMatchObject({ width: 1000, height: 500 })
  })

  it('rejects full-size sources whose original pixels exceed the default limit', async () => {
    installCanvasMocks()
    installImageMocks(8000, 4000)
    const cache = new ImageSourceCache({ maxEntries: 2 })

    await expect(cache.getFullSizeSource('https://example.com/large.png')).rejects.toMatchObject({
      code: 'too-large',
      message: 'too-large',
    })
  })

  it('fits oversized final sources within the pixel limit when requested', async () => {
    const { contexts } = installCanvasMocks()
    installImageMocks(8000, 4000)
    const cache = new ImageSourceCache({ maxEntries: 2 })

    const source = await cache.getFullSizeSource('https://example.com/large.png', {
      fitWithinMaxPixels: true,
    })

    expect(source).not.toBeNull()
    expect((source?.width ?? 0) * (source?.height ?? 0)).toBeLessThanOrEqual(24_000_000)
    expect(source?.width).toBe(6928)
    expect(source?.height).toBe(3464)
    expect(contexts[0]?.drawImage).toHaveBeenCalledWith(
      expect.objectContaining({ width: 8000, height: 4000 }),
      0,
      0,
      6928,
      3464,
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
        message: 'animated-image',
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

  it('aborts pending fetches when disposed without starting bitmap decode', async () => {
    installCanvasMocks()
    installImageMocks()
    fetchMock.mockImplementationOnce(
      (_url: string, { signal }: RequestInit) =>
        new Promise((_resolve, reject) => {
          signal?.addEventListener('abort', () => reject(signal.reason), { once: true })
        }),
    )
    const cache = new ImageSourceCache()
    const source = cache.getFullSizeSource('https://example.com/pending.png')
    const signal = fetchMock.mock.calls[0]?.[1]?.signal as AbortSignal
    expect(signal.aborted).toBe(false)
    cache.dispose()
    expect(signal.aborted).toBe(true)
    await expect(source).rejects.toMatchObject({ code: 'disposed' })
    expect(createImageBitmapMock).not.toHaveBeenCalled()
  })

  it('skips decoding bytes that finish reading after disposal', async () => {
    installCanvasMocks()
    installImageMocks()
    let finishBlob!: (blob: Blob) => void
    const blob = vi.fn(
      () =>
        new Promise<Blob>((resolve) => {
          finishBlob = resolve
        }),
    )
    const response = new Response()
    vi.spyOn(response, 'blob').mockImplementation(blob)
    fetchMock.mockResolvedValueOnce(response)
    const cache = new ImageSourceCache()
    const source = cache.getFullSizeSource('https://example.com/pending-body.png')
    await vi.waitFor(() => expect(blob).toHaveBeenCalledTimes(1))
    cache.dispose()
    finishBlob(new Blob(['source'], { type: 'image/png' }))
    await expect(source).rejects.toMatchObject({ code: 'disposed' })
    expect(createImageBitmapMock).not.toHaveBeenCalled()
  })

  it('releases a full-size canvas if the cache is disposed while decode is in flight', async () => {
    const { canvases } = installCanvasMocks()
    fetchMock.mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['source'], { type: 'image/png' }),
    })
    let resolveBitmap!: (bitmap: {
      width: number
      height: number
      close: ReturnType<typeof vi.fn>
    }) => void
    createImageBitmapMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveBitmap = resolve
        }),
    )
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('createImageBitmap', createImageBitmapMock)
    const bitmap = { width: 4, height: 2, close: vi.fn() }
    const cache = new ImageSourceCache({ maxEntries: 2 })

    const source = cache.getFullSizeSource('https://example.com/in-flight.png')
    await waitForImageBitmapRequest()
    cache.dispose()
    resolveBitmap(bitmap)

    await expect(source).rejects.toMatchObject({
      code: 'disposed',
      message: 'disposed',
    })
    expect(bitmap.close).toHaveBeenCalledTimes(1)
    expect(canvases).toHaveLength(0)
    await expect(cache.getFullSizeSource('https://example.com/next.png')).rejects.toMatchObject({
      code: 'disposed',
    })
  })
})

async function waitForImageBitmapRequest() {
  for (
    let attempt = 0;
    attempt < 10 && createImageBitmapMock.mock.calls.length === 0;
    attempt += 1
  ) {
    await new Promise((resolve) => window.setTimeout(resolve, 0))
  }
  expect(createImageBitmapMock).toHaveBeenCalledTimes(1)
}
