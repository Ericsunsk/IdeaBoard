import { describe, expect, it, vi } from 'vitest'
import { MASK_PAINTING_COLOR } from '../config/constants'
import { maskStrokeEngine } from './maskBrushEngine'

function createCanvasFixture() {
  const canvas = document.createElement('canvas')
  const lineTo = vi.fn()
  const context = {
    beginPath: vi.fn(),
    closePath: vi.fn(),
    fill: vi.fn(),
    lineTo,
    moveTo: vi.fn(),
    restore: vi.fn(),
    save: vi.fn(),
    fillStyle: '',
    globalCompositeOperation: 'source-over',
    canvas,
  } as unknown as CanvasRenderingContext2D
  vi.spyOn(canvas, 'getContext').mockReturnValue(context)
  return { canvas, context, lineTo }
}

const points = [
  { x: 10, y: 10, pressure: 0.5 },
  { x: 28, y: 18, pressure: 0.5 },
  { x: 46, y: 14, pressure: 0.5 },
]

describe('maskStrokeEngine', () => {
  it('renders a closed smoothed outline for a paint stroke', () => {
    const { canvas, context, lineTo } = createCanvasFixture()

    maskStrokeEngine.draw(canvas, points, {
      brushSize: 18,
      drawMode: 'paint',
      last: true,
    })

    expect(context.globalCompositeOperation).toBe('source-over')
    expect(context.fillStyle).toBe(MASK_PAINTING_COLOR)
    expect(context.moveTo).toHaveBeenCalledOnce()
    expect(lineTo.mock.calls.length).toBeGreaterThan(1)
    expect(context.closePath).toHaveBeenCalledOnce()
    expect(context.fill).toHaveBeenCalledOnce()
  })

  it('uses destination-out for erase strokes', () => {
    const { canvas, context } = createCanvasFixture()

    maskStrokeEngine.draw(canvas, points, {
      brushSize: 18,
      drawMode: 'erase',
      last: false,
    })

    expect(context.globalCompositeOperation).toBe('destination-out')
    expect(context.fill).toHaveBeenCalledOnce()
  })
})
