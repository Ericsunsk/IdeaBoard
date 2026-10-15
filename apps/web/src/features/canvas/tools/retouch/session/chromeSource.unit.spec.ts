import { createApp, defineComponent, nextTick, ref, shallowRef } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRetouchChromeSource } from './chromeSource.ts'
import { useRetouchSession } from './useRetouchSession'

const rendererMocks = vi.hoisted(() => ({ updateSourceImage: vi.fn() }))

vi.mock('./stage/runtime/webglEngine.ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./stage/runtime/webglEngine.ts')>()),
  isWebGLRetouchSupported: () => true,
  createWebGLRetouchRenderer: () => ({
    updateSourceImage: rendererMocks.updateSourceImage,
    updateUniforms: vi.fn(),
    render: vi.fn(),
    dispose: vi.fn(),
    isContextLost: () => false,
  }),
}))

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
    complete: { value: true, configurable: true },
    naturalWidth: { value: width, configurable: true },
    naturalHeight: { value: height, configurable: true },
    currentSrc: { value: src, configurable: true },
    src: { value: src, configurable: true },
  })
  return image
}

function installCanvas2dMock() {
  const drawImage = vi.fn()
  vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
    if (tagName.toLowerCase() !== 'canvas') {
      return document.createElementNS('http://www.w3.org/1999/xhtml', tagName) as HTMLElement
    }
    const canvas = document.createElementNS(
      'http://www.w3.org/1999/xhtml',
      'canvas',
    ) as HTMLCanvasElement
    Object.defineProperty(canvas, 'getContext', {
      value: () => ({
        drawImage,
        getImageData: (x: number, y: number, width: number, height: number) =>
          new TestImageData(new Uint8ClampedArray(width * height * 4).fill(7), width, height),
      }),
    })
    return canvas
  })
  return { drawImage }
}

afterEach(() => {
  vi.clearAllMocks()
  vi.restoreAllMocks()
})

function mountChromeSource() {
  const { drawImage } = installCanvas2dMock()
  const isAdjustMode = ref(true)
  const sessionKey = ref('node-1')
  const stageElement = document.createElement('div')
  stageElement.appendChild(readyImage('https://example.com/node.png'))
  const currentStageElement = shallowRef(stageElement)
  let source: ReturnType<typeof createRetouchChromeSource> | undefined
  let sessionApi: ReturnType<typeof useRetouchSession> | undefined
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        const retouchSession = useRetouchSession()
        sessionApi = retouchSession
        source = createRetouchChromeSource({
          isAdjustMode,
          layers: retouchSession.retouchLayers,
          selectedEffectLayer: retouchSession.selectedRetouchEffectLayer,
          getStageElement: () => currentStageElement.value,
          sessionKey,
        })
        return () => null
      },
    }),
  )
  app.mount(host)
  if (!source || !sessionApi) throw new Error('chrome source was not created')
  return {
    source,
    sessionApi,
    stageElement,
    currentStageElement,
    isAdjustMode,
    sessionKey,
    drawImage,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

describe('createRetouchChromeSource histogram base', () => {
  it('exposes the zero-copy stage without a GPU histogram sample', () => {
    const { source, unmount } = mountChromeSource()
    try {
      expect(source.retouchStage.canvasRef).toBeDefined()
      expect(source.retouchStage.setCanvasRef).toEqual(expect.any(Function))
      expect(source.retouchStage.isGpuActive).toBeDefined()
      expect(source.retouchStage.stageStatus).toBeDefined()
      expect('histogramSample' in source.retouchStage).toBe(false)
    } finally {
      unmount()
    }
  })

  it('keeps the histogram source stable while the selected layer draft changes', () => {
    const { source, sessionApi, unmount } = mountChromeSource()
    try {
      sessionApi.selectImageEffectLayer('threshold')
      const first = source.retouchHistogramSource.value
      expect(first).not.toBeNull()

      // Slider drags rewrite the selected layer params: the mountain (layer
      // input) must freeze, only the markers follow the hand.
      sessionApi.updateSelectedRetouchEffectParams({ threshold: 200, invert: false })
      expect(source.retouchHistogramSource.value).toBe(first)

      sessionApi.updateSelectedRetouchEffectParams({ threshold: 40, invert: true })
      expect(source.retouchHistogramSource.value).toBe(first)
    } finally {
      unmount()
    }
  })

  it('updates the histogram source when the selected layer changes', () => {
    const { source, sessionApi, unmount } = mountChromeSource()
    try {
      sessionApi.selectImageEffectLayer('threshold')
      const first = source.retouchHistogramSource.value
      expect(first).not.toBeNull()

      sessionApi.selectImageEffectLayer('hsl')
      const second = source.retouchHistogramSource.value
      expect(second).not.toBeNull()
      expect(second).not.toBe(first)
    } finally {
      unmount()
    }
  })
})

describe('createRetouchChromeSource decoded image quality', () => {
  it('rebinds the original image after entering Retouch from a low-resolution Canvas tier', async () => {
    const { source, sessionApi, stageElement, drawImage, unmount } = mountChromeSource()
    try {
      const thumbnail = readyImage('https://example.com/thumb.webp', 256, 192)
      stageElement.replaceChildren(thumbnail)
      const canvas = document.createElement('canvas')
      stageElement.appendChild(canvas)
      source.retouchStage.setCanvasRef(canvas)
      await nextTick()
      expect(rendererMocks.updateSourceImage).toHaveBeenLastCalledWith(thumbnail, 256, 192)

      sessionApi.selectImageEffectLayer('threshold')
      const compactHistogram = source.retouchHistogramSource.value
      expect(compactHistogram).not.toBeNull()

      const original = thumbnail
      Object.defineProperties(original, {
        currentSrc: { value: 'https://example.com/original.webp' },
        src: { value: 'https://example.com/original.webp' },
        naturalWidth: { value: 4000 },
        naturalHeight: { value: 3000 },
      })
      original.dispatchEvent(new Event('load'))
      await nextTick()

      expect(rendererMocks.updateSourceImage).toHaveBeenLastCalledWith(original, 4000, 3000)
      expect(source.retouchStage.stageStatus.value).toBe('ready')
      const fullHistogram = source.retouchHistogramSource.value
      expect(fullHistogram).not.toBeNull()
      expect(fullHistogram).not.toBe(compactHistogram)
      expect(drawImage).toHaveBeenLastCalledWith(original, 0, 0, 128, 96)
      sessionApi.updateSelectedRetouchLayerParam('threshold', 200)
      expect(source.retouchHistogramSource.value).toBe(fullHistogram)
    } finally {
      unmount()
    }
  })
})

describe('createRetouchChromeSource source readiness lifecycle', () => {
  it('binds a source that decodes after entry without a slider interaction', async () => {
    const { source, stageElement, unmount } = mountChromeSource()
    try {
      stageElement.replaceChildren()
      const canvas = document.createElement('canvas')
      stageElement.appendChild(canvas)
      source.retouchStage.setCanvasRef(canvas)
      await nextTick()
      expect(source.retouchStage.stageStatus.value).toBe('idle')
      const original = readyImage('https://example.com/original.webp', 4000, 3000)
      stageElement.appendChild(original)
      original.dispatchEvent(new Event('load'))
      await nextTick()
      expect(rendererMocks.updateSourceImage).toHaveBeenLastCalledWith(original, 4000, 3000)
      expect(source.retouchStage.stageStatus.value).toBe('ready')
    } finally {
      unmount()
    }
  })

  it('ignores late image loads after exit and unmount', async () => {
    const { source, stageElement, isAdjustMode, unmount } = mountChromeSource()
    const canvas = document.createElement('canvas')
    stageElement.appendChild(canvas)
    source.retouchStage.setCanvasRef(canvas)
    await nextTick()
    rendererMocks.updateSourceImage.mockClear()
    isAdjustMode.value = false
    const original = readyImage('https://example.com/original.webp', 4000, 3000)
    stageElement.replaceChildren(original, canvas)
    original.dispatchEvent(new Event('load'))
    await nextTick()
    expect(rendererMocks.updateSourceImage).not.toHaveBeenCalled()
    expect(source.retouchStage.stageStatus.value).toBe('idle')
    unmount()
    original.dispatchEvent(new Event('load'))
    await nextTick()
    expect(rendererMocks.updateSourceImage).not.toHaveBeenCalled()
  })

  it('ignores loads from the old node after the active node and canvas change', async () => {
    const { source, stageElement, currentStageElement, sessionKey, unmount } = mountChromeSource()
    try {
      const oldCanvas = document.createElement('canvas')
      stageElement.appendChild(oldCanvas)
      source.retouchStage.setCanvasRef(oldCanvas)
      await nextTick()
      const nextStage = document.createElement('div')
      const currentImage = readyImage('https://example.com/new-original.webp', 3000, 2000)
      const currentCanvas = document.createElement('canvas')
      nextStage.append(currentImage, currentCanvas)
      currentStageElement.value = nextStage
      sessionKey.value = 'node-2'
      source.retouchStage.setCanvasRef(currentCanvas)
      const staleImage = readyImage('https://example.com/old-original.webp', 4000, 3000)
      stageElement.appendChild(staleImage)
      staleImage.dispatchEvent(new Event('load'))
      await nextTick()
      expect(rendererMocks.updateSourceImage).toHaveBeenLastCalledWith(currentImage, 3000, 2000)
      rendererMocks.updateSourceImage.mockClear()
      staleImage.dispatchEvent(new Event('load'))
      await nextTick()
      expect(rendererMocks.updateSourceImage).not.toHaveBeenCalled()
    } finally {
      unmount()
    }
  })
})
