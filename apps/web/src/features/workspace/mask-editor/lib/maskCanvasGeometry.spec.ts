import { describe, expect, it } from 'vitest'
import { getMaskCanvasPoint } from './maskCanvasGeometry'

function createCanvas(rect: Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>) {
  const canvas = document.createElement('canvas')
  canvas.width = 200
  canvas.height = 100
  canvas.getBoundingClientRect = () => ({ ...rect }) as DOMRect
  return canvas
}

describe('maskCanvasGeometry', () => {
  it('maps viewport coordinates into intrinsic canvas space', () => {
    const canvas = createCanvas({ left: 10, top: 20, width: 100, height: 50 })

    expect(getMaskCanvasPoint(60, 45, canvas)).toEqual({ x: 100, y: 50 })
  })

  it('clamps points to canvas bounds and handles empty layout', () => {
    const canvas = createCanvas({ left: 10, top: 20, width: 100, height: 50 })
    expect(getMaskCanvasPoint(-10, 200, canvas)).toEqual({ x: 0, y: 100 })

    const collapsed = createCanvas({ left: 0, top: 0, width: 0, height: 0 })
    expect(getMaskCanvasPoint(10, 10, collapsed)).toEqual({ x: 0, y: 0 })
  })
})
