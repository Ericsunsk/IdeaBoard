import { describe, expect, it, vi } from 'vitest'
import { drawComposingOrb } from './composingRenderer'

function createCanvasContext() {
  const arcs: Array<[number, number, number]> = []
  const colors: string[] = []
  const context = {
    fillStyle: '',
    beginPath: vi.fn(),
    arc: vi.fn((x: number, y: number, radius: number) => {
      arcs.push([x, y, radius])
      colors.push(String(context.fillStyle))
    }),
    fill: vi.fn(),
  } as unknown as CanvasRenderingContext2D

  return { arcs, colors, context }
}

describe('composing orb renderer', () => {
  it('keeps geometry and paint order stable after other frames, sizes, and contexts', () => {
    const first = createCanvasContext()
    const second = createCanvasContext()
    drawComposingOrb(first.context, 171, 1.25, false)
    const expectedArcs = [...first.arcs]
    const expectedColors = [...first.colors]

    drawComposingOrb(first.context, 300, 8.5, false)
    drawComposingOrb(second.context, 100, 0, false)
    first.arcs.length = 0
    first.colors.length = 0
    drawComposingOrb(first.context, 171, 1.25, false)

    expect(first.arcs).toEqual(expectedArcs)
    expect(first.colors).toEqual(expectedColors)
    expect(second.arcs).toHaveLength(566)
  })

  it('changes the animated geometry and its depth-dependent colors between frames', () => {
    const frame = createCanvasContext()
    drawComposingOrb(frame.context, 171, 0, false)
    const initialArcs = [...frame.arcs]
    const initialColors = [...frame.colors]

    frame.arcs.length = 0
    frame.colors.length = 0
    drawComposingOrb(frame.context, 171, 1.25, false)

    expect(frame.arcs).toHaveLength(566)
    expect(frame.arcs).not.toEqual(initialArcs)
    expect(frame.colors).not.toEqual(initialColors)
  })

  it('renders the tuned 171px composing preset with finite geometry', () => {
    const { arcs, context } = createCanvasContext()

    drawComposingOrb(context, 171, 1.25, false)

    expect(arcs).toHaveLength(566)
    expect(arcs.every((point) => point.every(Number.isFinite))).toBe(true)
    expect(arcs.every(([, , radius]) => radius >= 0.3)).toBe(true)
  })

  it('inverts ink on dark backgrounds to keep contrast', () => {
    const light = createCanvasContext()
    const dark = createCanvasContext()

    drawComposingOrb(light.context, 171, 1.25, false)
    drawComposingOrb(dark.context, 171, 1.25, true)

    expect(dark.colors).toHaveLength(light.colors.length)
    expect(dark.colors).not.toEqual(light.colors)

    const grays = (colors: string[]) =>
      colors.map((color) => Number(/rgba\((\d+),/.exec(color)?.[1] ?? NaN))
    const lightGrays = grays(light.colors)
    const darkGrays = grays(dark.colors)
    expect(lightGrays.every(Number.isFinite)).toBe(true)
    expect(darkGrays.every(Number.isFinite)).toBe(true)
    // Upstream inversion: gray(dark) === 255 - gray(light), up to rounding.
    for (let index = 0; index < lightGrays.length; index += 1) {
      expect(Math.abs(darkGrays[index]! + lightGrays[index]! - 255)).toBeLessThanOrEqual(1)
    }
    const average = (values: number[]) => values.reduce((sum, value) => sum + value, 0) / values.length
    expect(Math.abs(average(darkGrays) - average(lightGrays))).toBeGreaterThan(10)
  })
})
