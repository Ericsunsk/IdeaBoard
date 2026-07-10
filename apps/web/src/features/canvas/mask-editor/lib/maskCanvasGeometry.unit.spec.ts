import { describe, expect, it } from 'vitest'
import { createMaskCanvasPointerProjection } from './maskCanvasGeometry'

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
    const project = createMaskCanvasPointerProjection(canvas)

    expect(project(60, 45)).toEqual({ x: 100, y: 50 })
  })

  it('clamps points to canvas bounds and handles empty layout', () => {
    const canvas = createCanvas({ left: 10, top: 20, width: 100, height: 50 })
    const project = createMaskCanvasPointerProjection(canvas)
    expect(project(-10, 200)).toEqual({ x: 0, y: 100 })

    const collapsed = createCanvas({ left: 0, top: 0, width: 0, height: 0 })
    const collapsedProject = createMaskCanvasPointerProjection(collapsed)
    expect(collapsedProject(10, 10)).toEqual({ x: 0, y: 0 })
  })

  it('reuses a captured layout projection for subsequent pointer moves', () => {
    const canvas = createCanvas({ left: 10, top: 20, width: 100, height: 50 })
    const project = createMaskCanvasPointerProjection(canvas)
    canvas.getBoundingClientRect = () => ({ left: 999, top: 999, width: 1, height: 1 }) as DOMRect

    expect(project(60, 45)).toEqual({ x: 100, y: 50 })
  })
})
