import { createApp, defineComponent, h, nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import ImageMaskEditorOverlay from './ImageMaskEditorOverlay.vue'

function mountOverlay() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const setCanvasRef = vi.fn()
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageMaskEditorOverlay, {
            brushShape: 'round',
            brushSize: 40,
            canvasHeight: 100,
            canvasWidth: 200,
            imageLayerVisible: true,
            maskLayerOpacity: 72,
            setCanvasRef,
            onCanvasPointerdown: vi.fn(),
          })
      },
    }),
  )
  app.mount(host)
  const canvas = host.querySelector<HTMLCanvasElement>('canvas')
  const cursor = host.querySelector<HTMLElement>('.image-mask-editor-brush-cursor')
  if (!canvas || !cursor) throw new Error('expected mask editor overlay elements')
  Object.defineProperties(canvas, {
    offsetWidth: { value: 200 },
    offsetHeight: { value: 100 },
  })

  return {
    app,
    canvas,
    cursor,
    host,
    setCanvasRef,
    unmount() {
      app.unmount()
      host.remove()
    },
  }
}

describe('ImageMaskEditorOverlay', () => {
  it('positions the brush cursor from pointer offset coordinates', async () => {
    const harness = mountOverlay()
    await nextTick()

    try {
      const event = new PointerEvent('pointermove', {
        bubbles: true,
        pointerType: 'mouse',
        clientX: 0,
        clientY: 0,
      })
      Object.defineProperties(event, {
        offsetX: { value: 37 },
        offsetY: { value: 42 },
      })
      harness.canvas.dispatchEvent(event)
      await nextTick()

      expect(harness.cursor.style.transform).toContain('translate3d(37px, 42px, 0)')
      expect(harness.cursor.classList.contains('is-visible')).toBe(true)
    } finally {
      harness.unmount()
    }
  })
})
