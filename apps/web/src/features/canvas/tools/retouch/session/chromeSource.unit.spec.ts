import { createApp, computed, defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRetouchChromeSource } from './chromeSource.ts'
import { useRetouchSession } from './useRetouchSession'

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
  vi.restoreAllMocks()
})

function mountChromeSource() {
  installCanvas2dMock()
  const stageElement = document.createElement('div')
  stageElement.appendChild(readyImage('https://example.com/node.png'))
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
          isAdjustMode: computed(() => true),
          layers: retouchSession.retouchLayers,
          selectedEffectLayer: retouchSession.selectedRetouchEffectLayer,
          getStageElement: () => stageElement,
          sessionKey: computed(() => 'node-1'),
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
