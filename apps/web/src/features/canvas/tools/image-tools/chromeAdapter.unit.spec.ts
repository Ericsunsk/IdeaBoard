import { createApp, computed, defineComponent, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createImageToolChromeAdapter } from './chromeAdapter.ts'
import { useRetouchSession } from '../retouch'
import type { ImageToolChromeCommands } from './bindings.ts'
import type { ImageToolSession } from './session.ts'

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

function mountAdapter() {
  installCanvas2dMock()
  const stageElement = document.createElement('div')
  stageElement.appendChild(readyImage('https://example.com/node.png'))
  const retouchMode = ref<'adjust' | 'inpaint'>('adjust')
  const retouchNodeId = ref<string | null>('node-1')
  let adapter: ReturnType<typeof createImageToolChromeAdapter> | undefined
  let sessionApi: ReturnType<typeof useRetouchSession> | undefined
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        const retouchSession = useRetouchSession()
        sessionApi = retouchSession
        const session = {
          capabilities: {
            board: {
              getImageStageElement: (nodeId: string) => (nodeId === 'node-1' ? stageElement : null),
            },
            execution: {
              isNodeExecutionActive: () => false,
            },
          },
          retouchMode,
          retouchNodeId,
          retouchSession,
          isImageRetouchMode: computed(() => retouchMode.value === 'adjust'),
          applyScope: { isApplying: ref(false) },
          inpaint: {
            drawMode: ref('paint'),
            brushSize: ref(18),
            canUndoMask: ref(false),
            canRedoMask: ref(false),
          },
          maskChromeState: { inpaintLayerOpacity: ref(80) },
          coordinates: {
            retouchChromeAnchorRect: ref(null),
            isRetouchChromeReady: ref(true),
          },
        } as unknown as ImageToolSession
        adapter = createImageToolChromeAdapter(session, {} as unknown as ImageToolChromeCommands)
        return () => null
      },
    }),
  )
  app.mount(host)
  if (!adapter || !sessionApi) throw new Error('adapter was not created')
  return {
    adapter,
    sessionApi,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

describe('createImageToolChromeAdapter histogram base', () => {
  it('exposes the zero-copy stage without a GPU histogram sample', () => {
    const { adapter, unmount } = mountAdapter()
    try {
      expect(adapter.retouchStage.canvasRef).toBeDefined()
      expect(adapter.retouchStage.setCanvasRef).toEqual(expect.any(Function))
      expect(adapter.retouchStage.isGpuActive).toBeDefined()
      expect(adapter.retouchStage.stageStatus).toBeDefined()
      expect('histogramSample' in adapter.retouchStage).toBe(false)
    } finally {
      unmount()
    }
  })

  it('keeps the histogram source stable while the selected layer draft changes', () => {
    const { adapter, sessionApi, unmount } = mountAdapter()
    try {
      sessionApi.selectImageEffectLayer('threshold')
      const first = adapter.chromeModel.value.editor.histogramSource
      expect(first).not.toBeNull()

      // Slider drags rewrite the selected layer params: the mountain (layer
      // input) must freeze, only the markers follow the hand.
      sessionApi.updateSelectedRetouchEffectParams({ threshold: 200, invert: false })
      expect(adapter.chromeModel.value.editor.histogramSource).toBe(first)

      sessionApi.updateSelectedRetouchEffectParams({ threshold: 40, invert: true })
      expect(adapter.chromeModel.value.editor.histogramSource).toBe(first)
    } finally {
      unmount()
    }
  })

  it('updates the histogram source when the selected layer changes', () => {
    const { adapter, sessionApi, unmount } = mountAdapter()
    try {
      sessionApi.selectImageEffectLayer('threshold')
      const first = adapter.chromeModel.value.editor.histogramSource
      expect(first).not.toBeNull()

      sessionApi.selectImageEffectLayer('hsl')
      const second = adapter.chromeModel.value.editor.histogramSource
      expect(second).not.toBeNull()
      expect(second).not.toBe(first)
    } finally {
      unmount()
    }
  })
})
