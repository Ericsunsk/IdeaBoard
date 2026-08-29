import { describe, expect, it, vi } from 'vitest'
import ImageInpaintOverlay from './ImageInpaintOverlay.vue'
import { render, waitFor } from '../../../../test/render'

function renderOverlay() {
  const setCanvasRef = vi.fn()
  const result = render(ImageInpaintOverlay, {
    props: {
      brushSize: 40,
      canvasHeight: 100,
      canvasWidth: 200,
      imageLayerVisible: true,
      maskLayerOpacity: 72,
      setCanvasRef,
      'onCanvas-pointerdown': vi.fn(),
    },
  })
  const canvas = result.container.querySelector<HTMLCanvasElement>('canvas')
  const cursor = result.container.querySelector<HTMLElement>('.inpaint-brush-cursor')
  if (!canvas || !cursor) throw new Error('expected mask editor overlay elements')
  Object.defineProperties(canvas, {
    offsetWidth: { value: 200 },
    offsetHeight: { value: 100 },
  })

  return {
    canvas,
    cursor,
    setCanvasRef,
    ...result,
  }
}

describe('ImageInpaintOverlay', () => {
  it('positions the brush cursor from pointer offset coordinates', async () => {
    const { canvas, cursor } = renderOverlay()

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
    canvas.dispatchEvent(event)

    await waitFor(() => {
      expect(cursor.style.transform).toContain('translate3d(37px, 42px, 0)')
      expect(cursor).toHaveClass('is-visible')
    })
  })
})
