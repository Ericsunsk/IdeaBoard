import { describe, expect, it, vi } from 'vitest'
import { drawComposingOrb } from './composingRenderer'

function createCanvasContext() {
  const arcs: Array<[number, number, number]> = []
  const context = {
    fillStyle: '',
    beginPath: vi.fn(),
    arc: vi.fn((x: number, y: number, radius: number) => arcs.push([x, y, radius])),
    fill: vi.fn(),
  } as unknown as CanvasRenderingContext2D

  return { arcs, context }
}

describe('composing orb renderer', () => {
  it('renders the tuned 171px composing preset with finite geometry', () => {
    const { arcs, context } = createCanvasContext()

    drawComposingOrb(context, 171, 1.25)

    expect(arcs).toHaveLength(566)
    expect(arcs.every((point) => point.every(Number.isFinite))).toBe(true)
    expect(arcs.every(([, , radius]) => radius >= 0.3)).toBe(true)
  })
})
