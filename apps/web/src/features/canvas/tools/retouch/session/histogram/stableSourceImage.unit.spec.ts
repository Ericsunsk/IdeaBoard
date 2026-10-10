import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import {
  decodeStableSourceImage,
  resolveStableSourceKey,
  STABLE_HISTOGRAM_MAX_SIDE,
  useStableSourceImage,
} from './stableSourceImage.ts'

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

function readyImage(src: string, width = 800, height = 600): HTMLImageElement {
  const image = document.createElement('img')
  Object.defineProperties(image, {
    complete: { value: true },
    naturalWidth: { value: width },
    naturalHeight: { value: height },
    currentSrc: { value: src },
    src: { value: src },
  })
  return image
}

function installCanvas2dMock(failImageData = false) {
  const drawImage = vi.fn()
  const getImageData = vi.fn(
    (x: number, y: number, width: number, height: number) =>
      new TestImageData(new Uint8ClampedArray(width * height * 4).fill(7), width, height),
  )
  if (failImageData) {
    getImageData.mockImplementation(() => {
      throw new DOMException('tainted', 'SecurityError')
    })
  }
  vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') {
      return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
    }
    const canvas = document.createElementNS(
      'http://www.w3.org/1999/xhtml',
      'canvas',
    ) as HTMLCanvasElement
    Object.defineProperty(canvas, 'getContext', {
      value: () => ({ drawImage, getImageData }),
    })
    return canvas
  })
  return { drawImage, getImageData }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('resolveStableSourceKey', () => {
  it('identifies the source by URL and decoded size', () => {
    expect(resolveStableSourceKey(readyImage('https://example.com/a.png', 800, 600))).toBe(
      'https://example.com/a.png 800x600',
    )
  })
})

describe('decodeStableSourceImage', () => {
  it('downsamples to the max side while keeping the aspect ratio', () => {
    const { drawImage, getImageData } = installCanvas2dMock()
    const decoded = decodeStableSourceImage(readyImage('https://example.com/a.png', 800, 600))

    expect(STABLE_HISTOGRAM_MAX_SIDE).toBe(128)
    expect(drawImage).toHaveBeenCalledTimes(1)
    expect(drawImage.mock.calls[0]?.slice(1)).toEqual([0, 0, 128, 96])
    expect(getImageData).toHaveBeenCalledWith(0, 0, 128, 96)
    expect(decoded?.width).toBe(128)
    expect(decoded?.height).toBe(96)
  })

  it('keeps small sources at full size', () => {
    const { drawImage } = installCanvas2dMock()
    const decoded = decodeStableSourceImage(readyImage('https://example.com/a.png', 64, 32))

    expect(drawImage.mock.calls[0]?.slice(1)).toEqual([0, 0, 64, 32])
    expect(decoded?.width).toBe(64)
    expect(decoded?.height).toBe(32)
  })

  it('returns null for missing or tainted sources without throwing', () => {
    installCanvas2dMock()
    expect(decodeStableSourceImage(null)).toBeNull()
    expect(decodeStableSourceImage(document.createElement('img'))).toBeNull()
  })

  it('returns null when the canvas read is blocked (CORS taint)', () => {
    installCanvas2dMock(true)
    expect(decodeStableSourceImage(readyImage('https://example.com/a.png'))).toBeNull()
  })
})

describe('useStableSourceImage', () => {
  it('reuses the same preview reference while the source is unchanged', () => {
    const { drawImage } = installCanvas2dMock()
    const isAdjustMode = ref(true)
    const refreshSource = ref<unknown>(0)
    const image = readyImage('https://example.com/a.png')
    const { stablePreviewSource } = useStableSourceImage({
      isAdjustMode,
      resolveDomImage: () => image,
      refreshSource,
    })

    const first = stablePreviewSource.value
    // A slider drag bumps the layer identity: the base must not move.
    refreshSource.value = 1
    const second = stablePreviewSource.value

    expect(first).not.toBeNull()
    expect(second).toBe(first)
    expect(drawImage).toHaveBeenCalledTimes(1)
  })

  it('resamples when the source image changes', () => {
    installCanvas2dMock()
    const isAdjustMode = ref(true)
    const refreshSource = ref<unknown>(0)
    let image = readyImage('https://example.com/a.png')
    const { stablePreviewSource } = useStableSourceImage({
      isAdjustMode,
      resolveDomImage: () => image,
      refreshSource,
    })

    const first = stablePreviewSource.value
    image = readyImage('https://example.com/b.png')
    refreshSource.value = 1

    expect(stablePreviewSource.value).not.toBeNull()
    expect(stablePreviewSource.value).not.toBe(first)
  })

  it('stays null outside adjust mode and samples on entry', () => {
    const { drawImage } = installCanvas2dMock()
    const isAdjustMode = ref(false)
    const refreshSource = ref<unknown>(0)
    const { stablePreviewSource } = useStableSourceImage({
      isAdjustMode,
      resolveDomImage: () => readyImage('https://example.com/a.png'),
      refreshSource,
    })

    expect(stablePreviewSource.value).toBeNull()
    expect(drawImage).not.toHaveBeenCalled()

    isAdjustMode.value = true
    expect(stablePreviewSource.value).not.toBeNull()
  })

  it('picks up a late-decoding image on the next refresh', () => {
    installCanvas2dMock()
    const isAdjustMode = ref(true)
    const refreshSource = ref<unknown>(0)
    let image: HTMLImageElement | null = null
    const { stablePreviewSource } = useStableSourceImage({
      isAdjustMode,
      resolveDomImage: () => image,
      refreshSource,
    })

    expect(stablePreviewSource.value).toBeNull()

    image = readyImage('https://example.com/a.png')
    refreshSource.value = 1
    expect(stablePreviewSource.value).not.toBeNull()
  })

  it('freezes the first sample within a session even when the DOM pick changes', () => {
    const { drawImage } = installCanvas2dMock()
    const isAdjustMode = ref(true)
    const refreshSource = ref<unknown>(0)
    const sessionKey = ref('node-1')
    let image = readyImage('https://example.com/a.png')
    const { stablePreviewSource } = useStableSourceImage({
      isAdjustMode,
      resolveDomImage: () => image,
      refreshSource,
      sessionKey,
    })

    const first = stablePreviewSource.value
    expect(first).not.toBeNull()

    // First drag: the DOM resolver now picks a different img (thumbnail vs
    // full image, late currentSrc) — the mountain must not move.
    image = readyImage('https://example.com/b.png')
    refreshSource.value = 1

    expect(stablePreviewSource.value).toBe(first)
    expect(drawImage).toHaveBeenCalledTimes(1)
  })

  it('resamples after the session key changes', () => {
    installCanvas2dMock()
    const isAdjustMode = ref(true)
    const refreshSource = ref<unknown>(0)
    const sessionKey = ref('node-1')
    const { stablePreviewSource } = useStableSourceImage({
      isAdjustMode,
      resolveDomImage: () => readyImage('https://example.com/a.png'),
      refreshSource,
      sessionKey,
    })

    const first = stablePreviewSource.value
    expect(first).not.toBeNull()

    sessionKey.value = 'node-2'
    const second = stablePreviewSource.value
    expect(second).not.toBeNull()
    expect(second).not.toBe(first)
  })

  it('recovers after a tainted read instead of caching the failure', () => {
    installCanvas2dMock(true)
    const isAdjustMode = ref(true)
    const refreshSource = ref<unknown>(0)
    const { stablePreviewSource } = useStableSourceImage({
      isAdjustMode,
      resolveDomImage: () => readyImage('https://example.com/a.png'),
      refreshSource,
    })

    expect(stablePreviewSource.value).toBeNull()

    vi.restoreAllMocks()
    installCanvas2dMock()
    refreshSource.value = 1
    expect(stablePreviewSource.value).not.toBeNull()
  })
})
