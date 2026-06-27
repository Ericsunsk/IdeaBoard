import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageEditorAdjustmentLayer } from '../types'
import { renderImageAdjustmentFile } from './imageAdjustmentRenderer'

const fetchMock = vi.fn()
const createImageBitmapMock = vi.fn()

const invertLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:invert',
  type: 'adjustment',
  kind: 'invert',
  name: '反相 1',
  visible: true,
  params: { enabled: true },
}

afterEach(() => {
  vi.restoreAllMocks()
  fetchMock.mockReset()
  createImageBitmapMock.mockReset()
})

function installCanvasMocks() {
  class TestImageData {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }

  const sourceImageData = new TestImageData(new Uint8ClampedArray([10, 20, 30, 255]), 1, 1)
  const contexts: Array<{
    drawImage: ReturnType<typeof vi.fn>
    getImageData: ReturnType<typeof vi.fn>
    putImageData: ReturnType<typeof vi.fn>
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
      getImageData: vi.fn(() => sourceImageData),
      putImageData: vi.fn(),
    }
    Object.defineProperty(canvas, 'getContext', {
      value: () => context,
    })
    Object.defineProperty(canvas, 'toBlob', {
      value: vi.fn((callback: BlobCallback) => {
        callback(new Blob(['rendered'], { type: 'image/webp' }))
      }),
    })
    contexts.push(context)
    return canvas
  })

  return { contexts }
}

describe('renderImageAdjustmentFile', () => {
  it('renders visible adjustment layers to an uploadable file', async () => {
    const { contexts } = installCanvasMocks()
    fetchMock.mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['source'], { type: 'image/png' }),
    })
    createImageBitmapMock.mockResolvedValue({
      width: 2,
      height: 1,
      close: vi.fn(),
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('createImageBitmap', createImageBitmapMock)

    const result = await renderImageAdjustmentFile(
      { url: 'https://example.com/source.png', title: 'Sample.png' },
      [invertLayer],
    )

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/user/download?url=https%3A%2F%2Fexample.com%2Fsource.png',
      expect.objectContaining({ credentials: 'include' }),
    )
    expect(result.name).toBe('Sample-adjusted.webp')
    expect(result.type).toBe('image/webp')
    expect(contexts.at(-1)?.putImageData).toHaveBeenCalled()
  })

  it('rejects empty visible adjustment stacks', async () => {
    await expect(
      renderImageAdjustmentFile({ url: 'https://example.com/source.png' }, [
        { ...invertLayer, visible: false },
      ]),
    ).rejects.toThrow('没有可应用的调整图层')
  })
})
